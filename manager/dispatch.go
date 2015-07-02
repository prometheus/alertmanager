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
		log.Infoln("waiting")
		alert := d.state.Alert().Next()

		log.Infoln("received:", alert)

		conf, err := d.state.Config().Get()
		if err != nil {
			log.Error(err)
			continue
		}
		log.Infoln("retrieved config")

		for _, m := range conf.Routes.Match(alert.Labels) {
			d.processAlert(alert, m)
		}
		log.Infoln("processing done")
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
		ag = &aggrGroup{
			dispatcher: d,

			labels:    group,
			wait:      opts.GroupWait(),
			waitTimer: time.NewTimer(time.Hour),
			notify:    opts.SendTo,
		}
		d.aggrGroups[fp] = ag

		go ag.run()
	}

	ag.insert(alert)
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

func (ag *aggrGroup) run() {

	ag.waitTimer.Stop()

	if ag.wait > 0 {
		ag.waitTimer.Reset(ag.wait)
	}

	for {
		select {
		case <-ag.waitTimer.C:
			log.Infoln("timer flush")
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

func (ag *aggrGroup) insert(alert *Alert) {
	log.Infoln("insert:", alert)

	ag.alertsNew = append(ag.alertsNew, alert)
	sort.Sort(ag.alertsNew)

	if alert.Timestamp.Add(ag.wait).Before(time.Now()) {
		ag.flush()
	}

	if ag.wait > 0 {
		ag.waitTimer.Reset(ag.wait)
	}
}

func (ag *aggrGroup) flush() {
	log.Infoln("flush")

	ag.mtx.Lock()
	defer ag.mtx.Unlock()

	ag.dispatcher.notify(ag.notify, ag.alertsNew...)

	ag.alertsOld = append(ag.alertsOld, ag.alertsNew...)
	ag.alertsNew = ag.alertsNew[:0]
}

type alertTimeline []*Alert

func (at alertTimeline) Len() int {
	return len(at)
}

func (at alertTimeline) Less(i, j int) bool {
	return at[i].Timestamp.Before(at[j].Timestamp)
}

func (at alertTimeline) Swap(i, j int) {
	at[i], at[j] = at[j], at[i]
}
