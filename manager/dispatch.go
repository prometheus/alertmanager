package manager

import (
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
)

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
		ag = newAggrGroup(group, opts)
		d.aggrGroups[fp] = ag
	}

	ag.insert(alert)

	if ag.empty() {
		ag.stop()
		delete(d.aggrGroups, fp)
	}
}

// aggrGroup aggregates alerts into groups based on
// common values for a set of labels.
type aggrGroup struct {
	dispatcher *Dispatcher

	labels    model.LabelSet
	alertsOld alertTimeline
	alertsNew alertTimeline
	notify    string

	wait      time.Duration
	waitTimer *time.Timer

	done chan bool
	mtx  sync.RWMutex
}

// newAggrGroup returns a new aggregation group and starts background processing
// that sends notifications about the contained alerts.
func newAggrGroup(labels model.LabelSet, opts *RouteOpts) *aggrGroup {
	ag := &aggrGroup{
		dispatcher: d,

		labels:    group,
		wait:      opts.GroupWait(),
		waitTimer: time.NewTimer(opts.GroupWait()),
		notify:    opts.SendTo,
		done:      make(chan bool),
	}
	if ag.wait == 0 {
		ag.waitTimer.Stop()
	}

	go ag.run()

	return ag
}

func (ag *aggrGroup) run() {
	for {
		select {
		case <-ag.waitTimer.C:
			ag.flush()
		case <-ag.done:
			return
		}
	}
}

func (ag *aggrGroup) stop() {
	ag.waitTimer.Stop()
	close(ag.done)
}

func (ag *aggrGroup) fingerprint() model.Fingerprint {
	return ag.labels.Fingerprint()
}

// insert the alert into the aggregation group. If the aggregation group
// is empty afterwards, true is returned.
func (ag *aggrGroup) insert(alert *Alert) {
	ag.mtx.Lock()

	ag.alertsNew = append(ag.alertsNew, alert)
	sort.Sort(ag.alertsNew)

	ag.mtx.Unlock()

	// Immediately trigger a flush if the wait duration for this
	// alert is already over.
	if alert.Timestamp.Add(ag.wait).Before(time.Now()) {
		ag.flush()
	}

	if ag.wait > 0 {
		ag.waitTimer.Reset(ag.wait)
	}
}

func (ag *aggrGroup) empty() bool {
	ag.mtx.RLock()
	defer ag.mtx.RUnlock()

	return len(ag.alertsNew)+len(ag.alertsOld) == 0
}

// flush sends notifications for all new alerts.
func (ag *aggrGroup) flush() {
	ag.mtx.Lock()
	defer ag.mtx.Unlock()

	ag.dispatcher.notify(ag.notify, ag.alertsNew...)

	ag.alertsOld = append(ag.alertsOld, ag.alertsNew...)
	ag.alertsNew = ag.alertsNew[:0]
}

// alertTimeline is a list of alerts sorted by their timestamp.
type alertTimeline []*Alert

func (at alertTimeline) Len() int           { return len(at) }
func (at alertTimeline) Less(i, j int) bool { return at[i].Timestamp.Before(at[j].Timestamp) }
func (at alertTimeline) Swap(i, j int)      { at[i], at[j] = at[j], at[i] }
