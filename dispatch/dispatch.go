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
	aggrGroupsPerRoute map[*Route]map[model.Fingerprint]*aggrGroup
	aggrGroupsNum      int

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
	d.aggrGroupsPerRoute = map[*Route]map[model.Fingerprint]*aggrGroup{}
	d.aggrGroupsNum = 0
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
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, groups := range d.aggrGroupsPerRoute {
		for _, ag := range groups {
			if ag.empty() {
				ag.stop()
				d.marker.DeleteByGroupKey(ag.routeID, ag.GroupKey())
				delete(groups, ag.fingerprint())
				d.aggrGroupsNum--
				d.metrics.aggrGroups.Dec()
			}
		}
	}
}

// Groups returns a slice of AlertGroups from the dispatcher's internal state.
func (d *Dispatcher) Groups(routeFilter func(*Route) bool, alertFilter func(*types.Alert, time.Time) bool) (AlertGroups, map[model.Fingerprint][]string) {
	groups := AlertGroups{}

	d.mtx.RLock()
	defer d.mtx.RUnlock()

	// Keep a list of receivers for an alert to prevent checking each alert
	// again against all routes. The alert has already matched against this
	// route on ingestion.
	receivers := map[model.Fingerprint][]string{}

	now := time.Now()
	for route, ags := range d.aggrGroupsPerRoute {
		if !routeFilter(route) {
			continue
		}

		for _, ag := range ags {
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
				continue
			}
			alertGroup.Alerts = filteredAlerts

			groups = append(groups, alertGroup)
		}
	}
	sort.Sort(groups)
	for i := range groups {
		sort.Sort(groups[i].Alerts)
	}
	for i := range receivers {
		sort.Strings(receivers[i])
	}

	return groups, receivers
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

	d.mtx.Lock()
	defer d.mtx.Unlock()

	routeGroups, ok := d.aggrGroupsPerRoute[route]
	if !ok {
		routeGroups = map[model.Fingerprint]*aggrGroup{}
		d.aggrGroupsPerRoute[route] = routeGroups
	}

	ag, ok := routeGroups[fp]
	if ok {
		ag.insert(alert)
		return
	}

	// If the group does not exist, create it. But check the limit first.
	if limit := d.limits.MaxNumberOfAggregationGroups(); limit > 0 && d.aggrGroupsNum >= limit {
		d.metrics.aggrGroupLimitReached.Inc()
		d.logger.Error("Too many aggregation groups, cannot create new group for alert", "groups", d.aggrGroupsNum, "limit", limit, "alert", alert.Name())
		return
	}

	ag = newAggrGroup(d.ctx, groupLabels, route, d.timeout, d.logger)
	routeGroups[fp] = ag
	d.aggrGroupsNum++
	d.metrics.aggrGroups.Inc()

	// Insert the 1st alert in the group before starting the group's run()
	// function, to make sure that when the run() will be executed the 1st
	// alert is already there.
	ag.insert(alert)

	go ag.run(func(ctx context.Context, alerts ...*types.Alert) bool {
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
