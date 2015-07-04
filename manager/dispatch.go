package manager

import (
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
)

const ResolveTimeout = 15 * time.Second

// Dispatcher dispatches alerts. It is absed on the alert's data
// rather than the time they arrive. Thus it can recover it's state
// without persistence.
type Dispatcher struct {
	state State

	aggrGroups map[model.Fingerprint]*aggrGroup
}

func NewDispatcher(state State) *Dispatcher {
	return &Dispatcher{
		state:      state,
		aggrGroups: map[model.Fingerprint]*aggrGroup{},
	}
}

func (d *Dispatcher) notify(name string, alerts ...*Alert) {
	n := &LogNotifier{}
	i := []interface{}{name, "::"}
	for _, a := range alerts {
		i = append(i, a)
	}
	n.Send(i...)
}

func (d *Dispatcher) Run() {
	for {
		alert := d.state.Alert().Next()

		conf, err := d.state.Config().Get()
		if err != nil {
			log.Error(err)
			continue
		}

		for _, m := range conf.Routes.Match(alert.Labels) {
			d.processAlert(alert, m)
		}

		if !alert.Resolved {
			// After the constant timeout update the alert to be resolved.
			go func(alert *Alert) {
				for {
					// TODO: get most recent version first.
					time.Sleep(ResolveTimeout)

					a := *alert
					a.Resolved = true

					if err := d.state.Alert().Add(&a); err != nil {
						log.Error(err)
						continue
					}
					return
				}
			}(alert)
		}

		// Cleanup routine.
		for _, ag := range d.aggrGroups {
			if ag.empty() {
				ag.stop()
				delete(d.aggrGroups, ag.fingerprint())
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

// Alert models an action triggered by Prometheus.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload map[string]string `json:"payload"`

	// Short summary of alert.
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Runbook     string `json:"runbook"`

	// When the alert was reported.
	Timestamp time.Time `json:"timestamp"`

	// Whether the alert with the given label set is resolved.
	Resolved bool `json:"resolved"`
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
	}

	go ag.run()

	return ag
}

func (ag *aggrGroup) run() {
	// Set an initial one-time wait before flushing
	// the first batch of notifications.
	ag.next = time.NewTimer(ag.opts.GroupWait)

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
		if a.Resolved {
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
