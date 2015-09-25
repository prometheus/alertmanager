package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/provider"
)

const ResolveTimeout = 30 * time.Second

// Dispatcher sorts incoming alerts into aggregation groups and
// assigns the correct notifiers to each.
type Dispatcher struct {
	routes        Routes
	alertProvider provider.Alerts

	aggrGroups map[model.Fingerprint]*aggrGroup
	notifiers  map[string]Notifier

	mtx    sync.RWMutex
	done   chan struct{}
	ctx    context.Context
	cancel func()
}

// NewDispatcher returns a new Dispatcher.
func NewDispatcher(ctx context.Context, state State, notifier Notifier) *Dispatcher {
	d := &Dispatcher{
		aggrGroups: map[model.Fingerprint]*aggrGroup{},
	}
	d.ctx, d.cancel = context.WithCancel(ctx)

	return d
}

// ApplyConfig updates the dispatcher to match the new configuration.
func (d *Dispatcher) ApplyConfig(conf *Config) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	d.routes = conf.Routes
	d.notifiers = map[string]Notifier{}

	// TODO(fabxc): build correct notifiers from new conf.NotificationConfigs.
	for _, ncfg := range conf.NotificationConfigs {
		d.notifiers[ncfg.Name] = &LogNotifier{ncfg.Name}
	}
}

// Run starts dispatching alerts incoming via the updates channel.
func (d *Dispatcher) Run(updates <-chan *Alert) {
	d.done = make(chan struct{})

	defer close(d.done)
	defer close(updates)

	cleanup := time.Tick(30 * time.Second)

	for {
		select {
		case alert := <-updates:
			d.mtx.RLock()
			routes := d.routes.Match(alert.Labels)
			d.mtx.RUnlock()

			for _, r := range routes {
				d.processAlert(alert, r)
			}

		case <-cleanup:
			// Cleanup routine.
			for _, ag := range d.aggrGroups {
				if ag.empty() {
					ag.stop()
					delete(d.aggrGroups, ag.fingerprint())
				}
			}

		case <-d.ctx.Done():
			return
		}
	}
}

// Stop the dispatcher.
func (d *Dispatcher) Stop() {
	d.cancel()
	<-d.done
}

// notifyFunc is a function that performs notifcation for the alert
// with the given fingerprint. It aborts on context cancelation.
// It returns whether the alert has successfully been communiated as
// resolved.
type notifyFunc func(context.Context, model.Fingerprint) bool

// notifyFunc returns a function which performs a notification
// as required by the routing options.
func (d *Dispatcher) notifyFunc(dest string) notifyFunc {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	notifier := d.notifiers[dest]

	return func(ctx context.Context, fp model.Fingerprint) bool {
		alert := d.alertProvider.Get(fp)

		if err := notifier.Notify(ctx, alert); err != nil {
			log.Errorf("Notify for %v failed: %s", alert, err)
			return false
		}
		return alert.Resolved()
	}
}

// processAlert determins in which aggregation group the alert falls
// and insert it.
func (d *Dispatcher) processAlert(alert *Alert, opts *RouteOpts) {
	group := model.LabelSet{}

	for ln, lv := range alert.Labels {
		if _, ok := opts.GroupBy[ln]; ok {
			group[ln] = lv
		}
	}

	fp := group.Fingerprint()

	// If the group does not exist, create it.
	ag, ok := d.aggrGroups[fp]
	if !ok {
		ag = newAggrGroup(d, group, opts)
		ag.run(ag.notifyFunc(opts.SendTo))

		d.aggrGroups[fp] = ag
	}

	ag.insert(alert)
}

type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload map[string]string `json:"payload,omitempty"`

	CreatedAt  time.Time `json:"created_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`

	// The authoritative timestamp.
	Timestamp time.Time `json:"timestamp"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
}

// func (a *Alert) Merge(o *Alert) bool {

// }

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

// alertTimeline is a list of alerts sorted by their timestamp.
type alertTimeline []*Alert

func (at alertTimeline) Len() int           { return len(at) }
func (at alertTimeline) Less(i, j int) bool { return at[i].Timestamp.Before(at[j].Timestamp) }
func (at alertTimeline) Swap(i, j int)      { at[i], at[j] = at[j], at[i] }

// aggrGroup aggregates alert fingerprints into groups to which a
// common set of routing options applies.
// It emits notifications in the specified intervals.
type aggrGroup struct {
	labels model.LabelSet
	opts   *RouteOpts

	ctx    context.Context
	cancel func()
	done   chan struct{}

	mtx     sync.RWMutex
	alerts  map[model.Fingerprint]struct{}
	hasSent bool
	curRev  int
}

// newAggrGroup returns a new aggregation group.
func newAggrGroup(ctx context.Context, labels model.LabelSet, opts *RouteOpts) *aggrGroup {
	ag := &aggrGroup{
		labels: labels,
		opts:   opts,
		alerts: map[model.Fingerprint]struct{}{},
	}
	ag.ctx, ag.cancel = context.WithCancel(ctx)

	return ag
}

func (ag *aggrGroup) run(notify notifyFunc) {
	ag.done = make(chan struct{})

	// Set an initial one-time wait before flushing
	// the first batch of notifications.
	next := time.NewTimer(opts.GroupWait)

	defer close(ag.done)
	defer ag.next.Stop()

	for {
		select {
		case <-ag.next.C:
			// Give the notifcations 2/3 the time of the repeat interval
			// to finish before terminating them.
			ctx, _ := context.WithTimeout(ag.ctx, ag.opts.RepeatInterval*2/3)

			// Wait the configured interval before calling flush again.
			next.Reset(ag.opts.RepeatInterval)

			ag.flush(func(fp model.Fingerprint) bool {
				notify(ctx, fp)
			})

		case <-ag.ctx.Done():
			return
		}
	}
}

func (ag *aggrGroup) stop() {
	// Calling cancel will terminate all in-process notifications
	// and the run() loop.
	ag.cancel()
	<-ag.done
}

func (ag *aggrGroup) fingerprint() model.Fingerprint {
	return ag.labels.Fingerprint()
}

// insert the alert into the aggregation group. If the aggregation group
// is empty afterwards, true is returned.
func (ag *aggrGroup) insert(fp model.Fingerprint) {
	ag.mtx.Lock()
	defer ag.mtx.Unlock()

	ag.curRev++
	ag.alerts[fp] = ag.curRev

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
func (ag *aggrGroup) flush(notify func(model.Fingerprint) bool) {
	ag.mtx.Lock()

	alerts := make(map[model.Fingerprint]int, len(ag.alerts))
	for fp, rev := range ag.alerts {
		alerts[fp] = rev
	}

	ag.mtx.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(alerts))

	for fp, rev := range alerts {
		go func(fp model.Fingerprint) {
			// notify returns whether the alert can be deleted
			// afterwards.
			if notify(fp) {
				ag.mtx.Lock()
				// Only delete if the fingerprint has not been inserted
				// again since we notified about it.
				if ag.alerts[fp] == rev {
					delete(alerts, fp)
				}
				ag.mtx.Unlock()
			}
			wg.Done()
		}(fp)
	}

	wg.Wait()

	ag.hasSent = true
}
