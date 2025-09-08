// Copyright Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

// Dispatcher sorts incoming alerts into aggregation groups and
// assigns the correct notifiers to each.
type Dispatcher struct {
	route   *Route
	alerts  provider.Alerts
	stage   notify.Stage
	marker  types.GroupMarker
	metrics *DispatcherMetrics
	limits  Limits

	timeout func(time.Duration) time.Duration

	mtx                sync.RWMutex
	aggrGroupsPerRoute routeGroups

	done   chan struct{}
	ctx    context.Context
	cancel func()

	logger *slog.Logger
}

// NewDispatcher returns a new Dispatcher.
func NewDispatcher(
	ap provider.Alerts,
	r *Route,
	s notify.Stage,
	mk types.GroupMarker,
	to func(time.Duration) time.Duration,
	lim Limits,
	l *slog.Logger,
	m *DispatcherMetrics,
) *Dispatcher {
	if lim == nil {
		lim = nilLimits{}
	}

	disp := &Dispatcher{
		alerts:  ap,
		stage:   s,
		route:   r,
		marker:  mk,
		timeout: to,
		logger:  l.With("component", "dispatcher"),
		metrics: m,
		limits:  lim,
	}
	return disp
}

// Run starts dispatching alerts incoming via the updates channel.
func (d *Dispatcher) Run() {
	d.done = make(chan struct{})

	d.mtx.Lock()
	d.aggrGroupsPerRoute = routeGroups{
		groupsNum: &atomic.Int64{},
		limits:    d.limits,
	}
	d.metrics.aggrGroups.Set(0)
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.mtx.Unlock()

	d.run(d.alerts.Subscribe())
	close(d.done)
}

func (d *Dispatcher) run(it provider.AlertIterator) {
	maintenance := time.NewTicker(30 * time.Second)
	defer maintenance.Stop()

	defer it.Close()

	for {
		select {
		case alert, ok := <-it.Next():
			if !ok {
				// Iterator exhausted for some reason.
				if err := it.Err(); err != nil {
					d.logger.Error("Error on alert update", "err", err)
				}
				return
			}

			d.logger.Debug("Received alert", "alert", alert)

			// Log errors but keep trying.
			if err := it.Err(); err != nil {
				d.logger.Error("Error on alert update", "err", err)
				continue
			}

			now := time.Now()
			for _, r := range d.route.Match(alert.Labels) {
				d.processAlert(alert, r)
			}
			d.metrics.processingDuration.Observe(time.Since(now).Seconds())

		case <-maintenance.C:
			d.doMaintenance()
		case <-d.ctx.Done():
			return
		}
	}
}

func (d *Dispatcher) doMaintenance() {
	type groupToRemove struct {
		route *Route
		fp    model.Fingerprint
		ag    *aggrGroup
	}

	var toRemove []groupToRemove

	// First pass: collect groups to remove
	d.aggrGroupsPerRoute.Range(func(route *Route, groups *fingerprintGroups) bool {
		groups.Range(func(fp model.Fingerprint, ag *aggrGroup) bool {
			if ag.empty() {
				toRemove = append(toRemove, groupToRemove{route, fp, ag})
			}
			return true
		})
		return true
	})

	// Second pass: remove collected groups
	for _, item := range toRemove {
		item.ag.stop()
		d.marker.DeleteByGroupKey(item.ag.routeID, item.ag.GroupKey())
		groupsMap := d.aggrGroupsPerRoute.GetRoute(item.route)
		if groupsMap != nil {
			groupsMap.RemoveGroup(item.fp)
			d.metrics.aggrGroups.Dec()
		}
	}
}

// Groups returns a slice of AlertGroups from the dispatcher's internal state.
func (d *Dispatcher) Groups(routeFilter func(*Route) bool, alertFilter func(*types.Alert, time.Time) bool) (AlertGroups, map[model.Fingerprint][]string) {
	// Snapshot the outer map in routeGroups to
	//  avoid holding the read lock when dispatcher has 1000s of aggregation groups.
	routeGroups := routeGroups{}
	d.aggrGroupsPerRoute.Range(func(route *Route, groups *fingerprintGroups) bool {
		routeGroups.AddRoute(route)
		return true
	})

	// TODO: move this processing out of Dispatcher, it does not belong here.
	alertGroups := AlertGroups{}
	receivers := map[model.Fingerprint][]string{}
	now := time.Now()
	routeGroups.Range(func(route *Route, _ *fingerprintGroups) bool {
		if !routeFilter(route) {
			return true
		}

		// Read inner fingerprintGroups if necessary.
		d.aggrGroupsPerRoute.GetRoute(route).Range(func(fp model.Fingerprint, ag *aggrGroup) bool {
			receiver := route.RouteOpts.Receiver
			alertGroup := &AlertGroup{
				Labels:   ag.labels,
				Receiver: receiver,
				GroupKey: ag.GroupKey(),
				RouteID:  ag.routeID,
			}

			alerts := ag.alerts.List()
			filteredAlerts := make([]*types.Alert, 0, len(alerts))
			for _, a := range alerts {
				if !alertFilter(a, now) {
					continue
				}

				fp := a.Fingerprint()
				if r, ok := receivers[fp]; ok {
					// Receivers slice already exists. Add
					// the current receiver to the slice.
					receivers[fp] = append(r, receiver)
				} else {
					// First time we've seen this alert fingerprint.
					// Initialize a new receivers slice.
					receivers[fp] = []string{receiver}
				}

				filteredAlerts = append(filteredAlerts, a)
			}
			if len(filteredAlerts) == 0 {
				return true
			}
			alertGroup.Alerts = filteredAlerts

			alertGroups = append(alertGroups, alertGroup)
			return true
		})
		return true
	})
	sort.Sort(alertGroups)
	for i := range alertGroups {
		sort.Sort(alertGroups[i].Alerts)
	}
	for i := range receivers {
		sort.Strings(receivers[i])
	}

	return alertGroups, receivers
}

// Stop the dispatcher.
func (d *Dispatcher) Stop() {
	if d == nil {
		return
	}
	d.mtx.Lock()
	if d.cancel == nil {
		d.mtx.Unlock()
		return
	}
	d.cancel()
	d.cancel = nil
	d.mtx.Unlock()

	<-d.done
}

// processAlert determines in which aggregation group the alert falls
// and inserts it.
func (d *Dispatcher) processAlert(alert *types.Alert, route *Route) {
	groupLabels := getGroupLabels(alert, route)
	fp := groupLabels.Fingerprint()

	routeGroups := d.aggrGroupsPerRoute.AddRoute(route)
	group := routeGroups.GetGroup(fp)
	if group != nil {
		group.insert(alert)
		return
	}

	// If the group does not exist, create it.
	group, count, limit := routeGroups.AddGroup(fp, newAggrGroup(d.ctx, groupLabels, route, d.timeout, d.logger))
	if group == nil {
		// Rate limited.
		d.metrics.aggrGroupLimitReached.Inc()
		d.logger.Error("Too many aggregation groups, cannot create new group for alert", "groups", count, "limit", limit)
		return
	}
	d.metrics.aggrGroups.Inc()

	// Insert the 1st alert in the group before starting the group's run()
	// function, to make sure that when the run() will be executed the 1st
	// alert is already there.
	group.insert(alert)

	go group.run(func(ctx context.Context, alerts ...*types.Alert) bool {
		_, _, err := d.stage.Exec(ctx, d.logger, alerts...)
		if err != nil {
			logger := d.logger.With("num_alerts", len(alerts), "err", err)
			if errors.Is(ctx.Err(), context.Canceled) {
				// It is expected for the context to be canceled on
				// configuration reload or shutdown. In this case, the
				// message should only be logged at the debug level.
				logger.Debug("Notify for alerts failed")
			} else {
				logger.Error("Notify for alerts failed")
			}
		}
		return err == nil
	})
}
