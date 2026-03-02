// Copyright The Prometheus Authors
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
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

const (
	DispatcherStateUnknown = iota
	DispatcherStateWaitingToStart
	DispatcherStateRunning
	DispatcherStateStopped
)

var tracer = otel.Tracer("github.com/prometheus/alertmanager/dispatch")

// DispatcherMetrics represents metrics associated to a dispatcher.
type DispatcherMetrics struct {
	aggrGroups            prometheus.Gauge
	processingDuration    prometheus.Summary
	aggrGroupLimitReached prometheus.Counter
}

// NewDispatcherMetrics returns a new registered DispatchMetrics.
func NewDispatcherMetrics(registerLimitMetrics bool, r prometheus.Registerer) *DispatcherMetrics {
	if r == nil {
		return nil
	}
	m := DispatcherMetrics{
		aggrGroups: promauto.With(r).NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_dispatcher_aggregation_groups",
				Help: "Number of active aggregation groups",
			},
		),
		processingDuration: promauto.With(r).NewSummary(
			prometheus.SummaryOpts{
				Name: "alertmanager_dispatcher_alert_processing_duration_seconds",
				Help: "Summary of latencies for the processing of alerts.",
			},
		),
		aggrGroupLimitReached: promauto.With(r).NewCounter(
			prometheus.CounterOpts{
				Name: "alertmanager_dispatcher_aggregation_group_limit_reached_total",
				Help: "Number of times when dispatcher failed to create new aggregation group due to limit.",
			},
		),
	}

	return &m
}

// Dispatcher sorts incoming alerts into aggregation groups and
// assigns the correct notifiers to each.
type Dispatcher struct {
	route      *Route
	alerts     provider.Alerts
	stage      notify.Stage
	marker     types.GroupMarker
	metrics    *DispatcherMetrics
	limits     Limits
	propagator propagation.TextMapPropagator

	timeout func(time.Duration) time.Duration

	loaded   chan struct{}
	finished sync.WaitGroup
	ctx      context.Context
	cancel   func()

	routeGroupsSlice []routeAggrGroups
	aggrGroupsNum    atomic.Int32

	maintenanceInterval time.Duration
	concurrency         int // Number of goroutines for alert ingestion

	logger *slog.Logger

	startTimer *time.Timer
	state      atomic.Int32
}

// Limits describes limits used by Dispatcher.
type Limits interface {
	// MaxNumberOfAggregationGroups returns max number of aggregation groups that dispatcher can have.
	// 0 or negative value = unlimited.
	// If dispatcher hits this limit, it will not create additional groups, but will log an error instead.
	MaxNumberOfAggregationGroups() int
}

type routeAggrGroups struct {
	route     *Route
	groups    sync.Map // map[string]*aggrGroup
	groupsLen atomic.Int64
}

// NewDispatcher returns a new Dispatcher.
func NewDispatcher(
	alerts provider.Alerts,
	route *Route,
	stage notify.Stage,
	marker types.GroupMarker,
	timeout func(time.Duration) time.Duration,
	maintenanceInterval time.Duration,
	limits Limits,
	logger *slog.Logger,
	metrics *DispatcherMetrics,
) *Dispatcher {
	if limits == nil {
		limits = nilLimits{}
	}

	// Calculate concurrency for ingestion.
	concurrency := min(max(runtime.GOMAXPROCS(0)/2, 2), 8)

	disp := &Dispatcher{
		alerts:              alerts,
		stage:               stage,
		route:               route,
		marker:              marker,
		timeout:             timeout,
		maintenanceInterval: maintenanceInterval,
		concurrency:         concurrency,
		logger:              logger.With("component", "dispatcher"),
		metrics:             metrics,
		limits:              limits,
		propagator:          otel.GetTextMapPropagator(),
	}
	disp.state.Store(DispatcherStateUnknown)
	disp.loaded = make(chan struct{})
	disp.ctx, disp.cancel = context.WithCancel(context.Background())
	return disp
}

// Run starts dispatching alerts incoming via the updates channel.
func (d *Dispatcher) Run(dispatchStartTime time.Time) {
	if !d.state.CompareAndSwap(DispatcherStateUnknown, DispatcherStateWaitingToStart) {
		return
	}
	d.finished.Add(1)
	defer d.finished.Done()

	d.logger.Debug("preparing to start", "startTime", dispatchStartTime)
	d.startTimer = time.NewTimer(time.Until(dispatchStartTime))
	d.logger.Debug("setting state", "state", "waiting_to_start")
	d.routeGroupsSlice = make([]routeAggrGroups, d.route.Idx+1)
	d.route.Walk(func(r *Route) {
		d.routeGroupsSlice[r.Idx] = routeAggrGroups{
			route: r,
		}
	})

	d.aggrGroupsNum.Store(0)
	d.metrics.aggrGroups.Set(0)

	initalAlerts, it := d.alerts.SlurpAndSubscribe("dispatcher")
	for _, alert := range initalAlerts {
		d.routeAlert(d.ctx, alert)
	}
	close(d.loaded)

	d.run(it)
}

func (d *Dispatcher) run(it provider.AlertIterator) {
	defer it.Close()

	// Start maintenance goroutine
	d.finished.Go(func() {
		ticker := time.NewTicker(d.maintenanceInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				d.doMaintenance()
			case <-d.ctx.Done():
				return
			}
		}
	})

	// Start timer goroutine
	d.finished.Go(func() {
		<-d.startTimer.C

		if d.state.CompareAndSwap(DispatcherStateWaitingToStart, DispatcherStateRunning) {
			d.logger.Debug("started", "state", "running")
			d.logger.Debug("Starting all existing aggregation groups")
			for rg := range d.routeGroupsSlice {
				d.routeGroupsSlice[rg].groups.Range(func(_, ag any) bool {
					d.runAG(ag.(*aggrGroup))
					return true
				})
			}
		}
	})

	// Start multiple alert ingestion goroutines
	alertCh := it.Next()
	for i := 0; i < d.concurrency; i++ {
		d.finished.Add(1)
		go func(workerID int) {
			defer d.finished.Done()
			d.logger.Debug("starting alert ingestion worker", "workerID", workerID)

			for {
				select {
				case alert, ok := <-alertCh:
					if !ok {
						// Iterator exhausted for some reason.
						if err := it.Err(); err != nil {
							d.logger.Error("Error on alert update", "err", err, "workerID", workerID)
						}
						return
					}

					// Log errors but keep trying.
					if err := it.Err(); err != nil {
						d.logger.Error("Error on alert update", "err", err, "workerID", workerID)
						continue
					}

					ctx := d.ctx
					if alert.Header != nil {
						ctx = d.propagator.Extract(ctx, propagation.MapCarrier(alert.Header))
					}

					d.routeAlert(ctx, alert.Data)

				case <-d.ctx.Done():
					return
				}
			}
		}(i)
	}
	<-d.ctx.Done()
}

func (d *Dispatcher) routeAlert(ctx context.Context, alert *types.Alert) {
	d.logger.Debug("Received alert", "alert", alert)

	ctx, span := tracer.Start(ctx, "dispatch.Dispatcher.routeAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alert.Name()),
			attribute.String("alerting.alert.fingerprint", alert.Fingerprint().String()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	for _, r := range d.route.Match(alert.Labels) {
		span.AddEvent("dispatching alert to route",
			trace.WithAttributes(
				attribute.String("alerting.route.receiver.name", r.RouteOpts.Receiver),
			),
		)
		d.groupAlert(ctx, alert, r)
	}
	d.metrics.processingDuration.Observe(time.Since(now).Seconds())
}

func (d *Dispatcher) doMaintenance() {
	for i := range d.routeGroupsSlice {
		d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
			ag := el.(*aggrGroup)
			if ag.destroyed() {
				ag.stop()
				d.marker.DeleteByGroupKey(ag.routeID, ag.GroupKey())
				deleted := d.routeGroupsSlice[i].groups.CompareAndDelete(ag.fingerprint(), ag)
				if deleted {
					d.routeGroupsSlice[i].groupsLen.Add(-1)
					d.aggrGroupsNum.Add(-1)
					d.metrics.aggrGroups.Set(float64(d.aggrGroupsNum.Load()))
				}
			}
			return true
		})
	}
}

func (d *Dispatcher) WaitForLoading() {
	<-d.loaded
}

func (d *Dispatcher) LoadingDone() <-chan struct{} {
	return d.loaded
}

// AlertGroup represents how alerts exist within an aggrGroup.
type AlertGroup struct {
	Alerts   types.AlertSlice
	Labels   model.LabelSet
	Receiver string
	GroupKey string
	RouteID  string
}

type AlertGroups []*AlertGroup

func (ag AlertGroups) Swap(i, j int) { ag[i], ag[j] = ag[j], ag[i] }
func (ag AlertGroups) Less(i, j int) bool {
	if ag[i].Labels.Equal(ag[j].Labels) {
		return ag[i].Receiver < ag[j].Receiver
	}
	return ag[i].Labels.Before(ag[j].Labels)
}
func (ag AlertGroups) Len() int { return len(ag) }

// Groups returns a slice of AlertGroups from the dispatcher's internal state.
func (d *Dispatcher) Groups(ctx context.Context, routeFilter func(*Route) bool, alertFilter func(*types.Alert, time.Time) bool) (AlertGroups, map[model.Fingerprint][]string, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-d.LoadingDone():
	}
	groups := AlertGroups{}

	// Keep a list of receivers for an alert to prevent checking each alert
	// again against all routes. The alert has already matched against this
	// route on ingestion.
	receivers := map[model.Fingerprint][]string{}

	now := time.Now()
	for i := range d.routeGroupsSlice {
		if !routeFilter(d.routeGroupsSlice[i].route) {
			continue
		}
		receiver := d.routeGroupsSlice[i].route.RouteOpts.Receiver

		// Make a snapshot of the aggregation groups in each route to avoid holding
		// sync.Map locks while we process alerts or acquiring leaf locks in the alert
		// store.

		// Estimate capacity based on total groups and number of routes.
		// We overallocate a bit to avoid copying in most cases.
		snapshot := make([]*aggrGroup, 0, d.routeGroupsSlice[i].groupsLen.Load()+32)
		d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
			snapshot = append(snapshot, el.(*aggrGroup))
			return true
		})

		// Process the snapshot without holding sync.Map locks
		for _, ag := range snapshot {
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

	return groups, receivers, nil
}

// Stop the dispatcher.
func (d *Dispatcher) Stop() {
	if d == nil {
		return
	}
	d.state.Store(DispatcherStateStopped)
	d.cancel()
	d.finished.Wait()
}

// notifyFunc is a function that performs notification for the alert
// with the given fingerprint. It aborts on context cancelation.
// Returns false if notifying failed.
type notifyFunc func(context.Context, ...*types.Alert) bool

// groupAlert determines in which aggregation group the alert falls
// and inserts it.
func (d *Dispatcher) groupAlert(ctx context.Context, alert *types.Alert, route *Route) {
	_, span := tracer.Start(ctx, "dispatch.Dispatcher.groupAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alert.Name()),
			attribute.String("alerting.alert.fingerprint", alert.Fingerprint().String()),
			attribute.String("alerting.route.receiver.name", route.RouteOpts.Receiver),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	groupLabels := getGroupLabels(alert, route)

	fp := groupLabels.Fingerprint()

	el, loaded := d.routeGroupsSlice[route.Idx].groups.Load(fp)
	if loaded {
		ag := el.(*aggrGroup)
		// Try to insert into the aggrgroup.
		// If it's destroyed insert will return false.
		if ag.insert(ctx, alert) {
			return
		}
	}

	// If we couldn't insert, we need to create a new aggregation group.
	// Since multiple goroutines might be trying to create the same group concurrently
	// we will use the sync map swap to ensure only one of them creates it.

	// If the group does not exist, create it. But check the limit first.
	limit := d.limits.MaxNumberOfAggregationGroups()
	current := int(d.aggrGroupsNum.Load())
	if limit > 0 && current >= limit {
		d.metrics.aggrGroupLimitReached.Inc()
		err := errors.New("too many aggregation groups, cannot create new group for alert")
		message := "Failed to create aggregation group"
		d.logger.Error(message, "err", err.Error(), "groups", current, "limit", limit, "alert", alert.Name())
		span.SetStatus(codes.Error, message)
		span.RecordError(err,
			trace.WithAttributes(
				attribute.Int("alerting.aggregation_group.count", current),
				attribute.Int("alerting.aggregation_group.limit", limit),
			),
		)
		return
	}

	ag := newAggrGroup(d.ctx, groupLabels, route, d.timeout, d.marker.(types.AlertMarker), d.logger)
	// Insert the 1st alert in the group before starting the group's run()
	// function, to make sure that when the run() will be executed the 1st
	// alert is already there.
	ag.insert(ctx, alert)

	retries := 0
	for {
		if loaded {
			// Try to store the new group in the map. If another goroutine has already created the same group, use the existing one.
			swapped := d.routeGroupsSlice[route.Idx].groups.CompareAndSwap(fp, el, ag)
			if swapped {
				// We swapped the new group in, we can break and start it.
				break
			}
		} else {
			el, loaded = d.routeGroupsSlice[route.Idx].groups.LoadOrStore(fp, ag)
			if !loaded {
				d.routeGroupsSlice[route.Idx].groupsLen.Add(1)
				d.aggrGroupsNum.Add(1)
				d.metrics.aggrGroups.Set(float64(d.aggrGroupsNum.Load()))
				// We stored the new group, we can break and start it.
				break
			}
			if el == nil {
				continue
			}
			// we found an existing group, try to insert the alert into it. If it's destroyed, we will retry the whole process with the updated el.
			agExisting := el.(*aggrGroup)
			if agExisting.insert(ctx, alert) {
				return // if we inserted we return to avoid incrementing the aggrgroup count and starting the group.
			}
		}

		// If we failed to swap, it means another goroutine has created/modified the group
		retries++
		if retries > 100 {
			// This shouldn't happen - indicates a bug or extreme contention
			d.logger.Error("excessive retries creating aggregation group",
				"fingerprint", fp,
				"route", route.Key(),
				"alert", alert.Name(),
				"retries", retries,
			)
			// Give up and accept potential alert loss rather than infinite loop
			return
		}
	}

	span.AddEvent("new AggregationGroup created",
		trace.WithAttributes(
			attribute.String("alerting.aggregation_group.key", ag.GroupKey()),
			attribute.Int("alerting.aggregation_group.count", int(d.aggrGroupsNum.Load())),
		),
	)

	if alert.StartsAt.Add(ag.opts.GroupWait).Before(now) {
		message := "Alert is old enough for immediate flush, resetting timer to zero"
		ag.logger.Debug(message, "alert", alert.Name(), "fingerprint", alert.Fingerprint(), "startsAt", alert.StartsAt)
		span.AddEvent(message,
			trace.WithAttributes(
				attribute.String("alerting.alert.StartsAt", alert.StartsAt.Format(time.RFC3339)),
			),
		)
		ag.resetTimer(0)
	}
	// Check dispatcher and alert state to determine if we should run the AG now.
	switch d.state.Load() {
	case DispatcherStateWaitingToStart:
		span.AddEvent("Not starting Aggregation Group, dispatcher is not running")
		d.logger.Debug("Dispatcher still waiting to start")
	case DispatcherStateRunning:
		span.AddEvent("Starting Aggregation Group")
		d.runAG(ag)
	default:
		d.logger.Warn("unknown state detected", "state", "unknown")
	}
}

func (d *Dispatcher) runAG(ag *aggrGroup) {
	if !ag.running.CompareAndSwap(false, true) {
		return // already running
	}
	go ag.run(func(ctx context.Context, alerts ...*types.Alert) bool {
		_, _, err := d.stage.Exec(ctx, d.logger, alerts...)
		if err != nil {
			logger := d.logger.With("aggrGroup", ag.GroupKey(), "num_alerts", len(alerts), "err", err)
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

func getGroupLabels(alert *types.Alert, route *Route) model.LabelSet {
	groupLabels := model.LabelSet{}
	for ln, lv := range alert.Labels {
		if _, ok := route.RouteOpts.GroupBy[ln]; ok || route.RouteOpts.GroupByAll {
			groupLabels[ln] = lv
		}
	}

	return groupLabels
}

// aggrGroup aggregates alert fingerprints into groups to which a
// common set of routing options applies.
// It emits notifications in the specified intervals.
type aggrGroup struct {
	labels   model.LabelSet
	opts     *RouteOpts
	logger   *slog.Logger
	routeID  string
	routeKey string

	alerts  *store.Alerts
	marker  types.AlertMarker
	ctx     context.Context
	cancel  func()
	done    chan struct{}
	next    *time.Timer
	timeout func(time.Duration) time.Duration
	running atomic.Bool
}

// newAggrGroup returns a new aggregation group.
func newAggrGroup(
	ctx context.Context,
	labels model.LabelSet,
	r *Route,
	to func(time.Duration) time.Duration,
	marker types.AlertMarker,
	logger *slog.Logger,
) *aggrGroup {
	if to == nil {
		to = func(d time.Duration) time.Duration { return d }
	}
	ag := &aggrGroup{
		labels:   labels,
		routeID:  r.ID(),
		routeKey: r.Key(),
		opts:     &r.RouteOpts,
		timeout:  to,
		alerts:   store.NewAlerts(),
		marker:   marker,
		done:     make(chan struct{}),
	}
	ag.ctx, ag.cancel = context.WithCancel(ctx)

	ag.logger = logger.With("aggrGroup", ag)

	// Set an initial one-time wait before flushing
	// the first batch of notifications.
	ag.next = time.NewTimer(ag.opts.GroupWait)

	return ag
}

func (ag *aggrGroup) fingerprint() model.Fingerprint {
	return ag.labels.Fingerprint()
}

func (ag *aggrGroup) GroupKey() string {
	return fmt.Sprintf("%s:%s", ag.routeKey, ag.labels)
}

func (ag *aggrGroup) String() string {
	return ag.GroupKey()
}

func (ag *aggrGroup) run(nf notifyFunc) {
	defer close(ag.done)
	defer ag.next.Stop()

	for {
		select {
		case now := <-ag.next.C:
			// Give the notifications time until the next flush to
			// finish before terminating them.
			ctx, cancel := context.WithTimeout(ag.ctx, ag.timeout(ag.opts.GroupInterval))

			// The now time we retrieve from the ticker is the only reliable
			// point of time reference for the subsequent notification pipeline.
			// Calculating the current time directly is prone to flaky behavior,
			// which usually only becomes apparent in tests.
			ctx = notify.WithNow(ctx, now)

			// Populate context with information needed along the pipeline.
			ctx = notify.WithGroupKey(ctx, ag.GroupKey())
			ctx = notify.WithGroupLabels(ctx, ag.labels)
			ctx = notify.WithReceiverName(ctx, ag.opts.Receiver)
			ctx = notify.WithRepeatInterval(ctx, ag.opts.RepeatInterval)
			ctx = notify.WithMuteTimeIntervals(ctx, ag.opts.MuteTimeIntervals)
			ctx = notify.WithActiveTimeIntervals(ctx, ag.opts.ActiveTimeIntervals)
			ctx = notify.WithRouteID(ctx, ag.routeID)

			// Wait the configured interval before calling flush again.
			ag.resetTimer(ag.opts.GroupInterval)

			ag.flush(func(alerts ...*types.Alert) bool {
				ctx, span := tracer.Start(ctx, "dispatch.AggregationGroup.flush",
					trace.WithAttributes(
						attribute.String("alerting.aggregation_group.key", ag.GroupKey()),
						attribute.Int("alerting.alerts.count", len(alerts)),
					),
					trace.WithSpanKind(trace.SpanKindInternal),
				)
				defer span.End()

				success := nf(ctx, alerts...)
				if !success {
					span.SetStatus(codes.Error, "notification failed")
				}
				return success
			})

			cancel()

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

// resetTimer resets the timer for the AG.
func (ag *aggrGroup) resetTimer(t time.Duration) {
	ag.next.Reset(t)
}

// insert inserts the alert into the aggregation group.
// Returns false if the aggregation group has been destroyed.
func (ag *aggrGroup) insert(ctx context.Context, alert *types.Alert) bool {
	_, span := tracer.Start(ctx, "dispatch.AggregationGroup.insert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alert.Name()),
			attribute.String("alerting.alert.fingerprint", alert.Fingerprint().String()),
			attribute.String("alerting.aggregation_group.key", ag.GroupKey()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	if err := ag.alerts.Set(alert); err != nil {
		if errors.Is(err, store.ErrDestroyed) {
			return false
		}
		message := "error on set alert"
		span.SetStatus(codes.Error, message)
		span.RecordError(err)
		ag.logger.Error(message, "err", err)
	}
	return true
}

func (ag *aggrGroup) empty() bool {
	return ag.alerts.Empty()
}

func (ag *aggrGroup) destroyed() bool {
	return ag.alerts.Destroyed()
}

// flush sends notifications for all new alerts.
func (ag *aggrGroup) flush(notify func(...*types.Alert) bool) {
	if ag.empty() {
		return
	}

	var (
		alerts        = ag.alerts.List()
		alertsSlice   = make(types.AlertSlice, 0, len(alerts))
		resolvedSlice = make(types.AlertSlice, 0, len(alerts))
		now           = time.Now()
	)
	for _, alert := range alerts {
		a := *alert
		// Ensure that alerts don't resolve as time move forwards.
		if a.ResolvedAt(now) {
			resolvedSlice = append(resolvedSlice, &a)
		} else {
			a.EndsAt = time.Time{}
		}
		alertsSlice = append(alertsSlice, &a)
	}
	sort.Stable(alertsSlice)

	ag.logger.Debug("flushing", "alerts", fmt.Sprintf("%v", alertsSlice))

	if notify(alertsSlice...) {
		// Delete all resolved alerts as we just sent a notification for them,
		// and we don't want to send another one. However, we need to make sure
		// that each resolved alert has not fired again during the flush as then
		// we would delete an active alert thinking it was resolved.
		// Since we are passing DestroyIfEmpty=true the group will be marked as
		// destroyed if there are no more alerts after the deletion.
		if err := ag.alerts.DeleteIfNotModified(resolvedSlice, true); err != nil {
			ag.logger.Error("error on delete alerts", "err", err)
		} else {
			// Delete markers for resolved alerts that are not in the store.
			for _, alert := range resolvedSlice {
				_, err := ag.alerts.Get(alert.Fingerprint())
				if errors.Is(err, store.ErrNotFound) {
					ag.marker.Delete(alert.Fingerprint())
				}
			}
		}
	}
}

type nilLimits struct{}

func (n nilLimits) MaxNumberOfAggregationGroups() int { return 0 }
