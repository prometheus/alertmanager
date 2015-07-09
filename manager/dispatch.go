package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
)

const ResolveTimeout = 35 * time.Second

// Dispatcher dispatches alerts. It is absed on the alert's data
// rather than the time they arrive. Thus it can recover it's state
// without persistence.
type Dispatcher struct {
	state State

	aggrGroups map[model.Fingerprint]*aggrGroup
	notifiers  map[string]Notifier

	mtx sync.RWMutex
}

func NewDispatcher(state State, notifiers []Notifier) *Dispatcher {
	disp := &Dispatcher{
		state:      state,
		aggrGroups: map[model.Fingerprint]*aggrGroup{},
		notifiers:  map[string]Notifier{},
	}

	for _, n := range notifiers {
		disp.notifiers[n.Name()] = n
	}

	return disp
}

func (d *Dispatcher) filter(alerts ...*Alert) ([]*Alert, error) {

	silences, err := d.state.Silence().List()
	if err != nil {
		return nil, err
	}

	var sentAlerts []*Alert

	for _, alert := range alerts {
		add := true
		// None of the existing silences must match the alert.
		for _, sil := range silences {
			if sil.Match(alert.Labels) {
				add = false
				break
			}
		}
		if !add {
			continue
		}

		// Filter out alerts that have already been sent.
		ni, err := d.state.Notify().Get(alert.Fingerprint())
		// Always try to send on error as the safest option.
		if err == nil && ni.LastSent.Before(alert.ResolvedAt) && ni.LastResolved == alert.Resolved() {
			continue
		}

		sentAlerts = append(sentAlerts, alert)
	}

	return sentAlerts, nil
}

func (d *Dispatcher) notify(name string, alerts ...*Alert) error {
	alerts, err := d.filter(alerts...)
	if err != nil {
		return err
	}

	if len(alerts) == 0 {
		return nil
	}

	d.mtx.RLock()
	notifier, ok := d.notifiers[name]
	d.mtx.RUnlock()

	if !ok {
		return fmt.Errorf("notifier %q does not exist", name)
	}

	if err = notifier.Send(alerts...); err != nil {
		return err
	}

	for _, alert := range alerts {
		_ = d.state.Notify().Set(alert.Fingerprint(), &NotifyInfo{
			LastSent:     time.Now(),
			LastResolved: alert.Resolved(),
		})
	}

	return nil
}

func (d *Dispatcher) Run() {
	var (
		updates = d.state.Alert().Iter()
		cleanup = time.Tick(30 * time.Second)
	)

	for {
		select {
		case <-cleanup:
			// Cleanup routine.
			for _, ag := range d.aggrGroups {
				if ag.empty() {
					ag.stop()
					delete(d.aggrGroups, ag.fingerprint())
				}
			}

		case alert := <-updates:

			conf, err := d.state.Config().Get()
			if err != nil {
				log.Error(err)
				continue
			}

			for _, m := range conf.Routes.Match(alert.Labels) {
				d.processAlert(alert, m)
			}

			if alert.ResolvedAt.IsZero() {
				alert.ResolvedAt = alert.CreatedAt.Add(ResolveTimeout)
			}

		}
	}
}

func (d *Dispatcher) processAlert(alert *Alert, opts *RouteOpts) {
	group := model.LabelSet{}

	for ln, lv := range alert.Labels {
		if _, ok := opts.GroupBy[ln]; ok {
			group[ln] = lv
		}
	}

	fp := group.Fingerprint()

	ag, ok := d.aggrGroups[fp]
	if !ok {
		ag = newAggrGroup(d, group, opts)
		d.aggrGroups[fp] = ag
	}

	ag.insert(alert)
}

type Silence struct {
	Matchers Matchers

	// The numeric ID of the silence.
	ID string

	// Name/email of the silence creator.
	CreatedBy string
	// When the silence was first created (Unix timestamp).
	CreatedAt, EndsAt time.Time

	// Additional comment about the silence.
	Comment string
}

func (sil *Silence) Match(lset model.LabelSet) bool {
	now := time.Now()

	if now.Before(sil.CreatedAt) || now.After(sil.EndsAt) {
		return false
	}

	return sil.Matchers.Match(lset)
}

// Alert models an action triggered by Prometheus.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload     map[string]string `json:"payload,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Description string            `json:"description,omitempty"`
	Runbook     string            `json:"runbook,omitempty"`

	CreatedAt  time.Time `json:"created_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`

	// The authoritative timestamp.
	Timestamp time.Time `json:"timestamp"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
}

// Fingerprint returns a unique hash for the alert. It is equivalent to
// the fingerprint of the alert's label set.
func (a *Alert) Fingerprint() model.Fingerprint {
	return a.Labels.Fingerprint()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), a.Fingerprint())
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

func (a *Alert) Resolved() bool {
	if a.ResolvedAt.IsZero() {
		return false
	}
	return !a.ResolvedAt.After(time.Now())
}

// aggrGroup aggregates alerts into groups based on
// common values for a set of labels.
type aggrGroup struct {
	dispatcher *Dispatcher

	labels model.LabelSet
	opts   *RouteOpts

	next *time.Timer
	done chan struct{}

	mtx     sync.RWMutex
	alerts  map[model.Fingerprint]struct{}
	hasSent bool
}

// newAggrGroup returns a new aggregation group and starts background processing
// that sends notifications about the contained alerts.
func newAggrGroup(d *Dispatcher, labels model.LabelSet, opts *RouteOpts) *aggrGroup {
	ag := &aggrGroup{
		dispatcher: d,

		labels: labels,
		opts:   opts,

		alerts: map[model.Fingerprint]struct{}{},
		done:   make(chan struct{}),

		// Set an initial one-time wait before flushing
		// the first batch of notifications.
		next: time.NewTimer(opts.GroupWait),
	}

	go ag.run()

	return ag
}

func (ag *aggrGroup) run() {

	defer ag.next.Stop()

	for {
		select {
		case <-ag.next.C:
			ag.flush()
			// Wait the configured interval before calling flush again.
			ag.next.Reset(ag.opts.GroupInterval)

		case <-ag.done:
			return
		}
	}
}

func (ag *aggrGroup) stop() {
	close(ag.done)
}

func (ag *aggrGroup) fingerprint() model.Fingerprint {
	return ag.labels.Fingerprint()
}

// insert the alert into the aggregation group. If the aggregation group
// is empty afterwards, true is returned.
func (ag *aggrGroup) insert(alert *Alert) {
	fp := alert.Fingerprint()

	ag.mtx.Lock()
	ag.alerts[fp] = struct{}{}
	ag.mtx.Unlock()

	// Immediately trigger a flush if the wait duration for this
	// alert is already over.
	if !ag.hasSent && alert.Timestamp.Add(ag.opts.GroupWait).Before(time.Now()) {
		ag.next.Reset(0)
	}
}

func (ag *aggrGroup) empty() bool {
	ag.mtx.RLock()
	defer ag.mtx.RUnlock()

	return len(ag.alerts) == 0
}

// flush sends notifications for all new alerts.
func (ag *aggrGroup) flush() {
	ag.mtx.Lock()
	defer ag.mtx.Unlock()

	var alerts []*Alert
	for fp := range ag.alerts {
		a, err := ag.dispatcher.state.Alert().Get(fp)
		if err != nil {
			log.Error(err)
			continue
		}
		// TODO(fabxc): only delete if notify successful.
		if a.Resolved() {
			delete(ag.alerts, fp)
		}
		alerts = append(alerts, a)
	}

	ag.dispatcher.notify(ag.opts.SendTo, alerts...)
	ag.hasSent = true
}

// alertTimeline is a list of alerts sorted by their timestamp.
type alertTimeline []*Alert

func (at alertTimeline) Len() int           { return len(at) }
func (at alertTimeline) Less(i, j int) bool { return at[i].Timestamp.Before(at[j].Timestamp) }
func (at alertTimeline) Swap(i, j int)      { at[i], at[j] = at[j], at[i] }
