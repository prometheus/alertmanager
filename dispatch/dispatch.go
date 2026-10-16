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
	"maps"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/tracing"
)

const (
	DispatcherStateUnknown = iota
	DispatcherStateWaitingToStart
	DispatcherStateRunning
	DispatcherStateStopped
)

var tracer = tracing.NewTracer("github.com/prometheus/alertmanager/dispatch")

// Dispatcher sorts incoming alerts into aggregation groups and
// assigns the correct notifiers to each.
type Dispatcher struct {
	route      *Route
	alerts     provider.Alerts
	stage      notify.Stage
	marker     marker.GroupMarker
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

	logger   *slog.Logger
	recorder eventrecorder.Recorder
	tmpl     *template.Template

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
	// labels is the route's label classification, computed once per route and
	// shared by all of its aggregation groups.
	labels routeLabelSplit
}

// NewDispatcher returns a new Dispatcher.
func NewDispatcher(
	alerts provider.Alerts,
	route *Route,
	stage notify.Stage,
	marker marker.GroupMarker,
	timeout func(time.Duration) time.Duration,
	maintenanceInterval time.Duration,
	limits Limits,
	logger *slog.Logger,
	recorder eventrecorder.Recorder,
	metrics *DispatcherMetrics,
	tmpl *template.Template,
) *Dispatcher {
	if limits == nil {
		limits = nilLimits{}
	}
	if metrics == nil {
		metrics = newNoopDispatcherMetrics()
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
		recorder:            recorder,
		metrics:             metrics,
		limits:              limits,
		tmpl:                tmpl,
		propagator:          otel.GetTextMapPropagator(),
	}
	disp.state.Store(DispatcherStateUnknown)
	disp.loaded = make(chan struct{})
	disp.ctx, disp.cancel = context.WithCancel(eventrecorder.WithEventRecording(context.Background()))

	if metrics != nil && metrics.alertsCollector != nil {
		metrics.alertsCollector.dispatcher.Store(disp)
	}

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
			route:  r,
			labels: splitRouteLabels(d.tmpl, r.RouteOpts.Labels),
		}
	})

	d.aggrGroupsNum.Store(0)
	d.metrics.aggrGroups.Set(0)

	initalAlerts, it := d.alerts.SlurpAndSubscribe("dispatcher")
	for _, alrt := range initalAlerts {
		d.routeAlert(d.ctx, alrt)
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
		select {
		case <-d.startTimer.C:
		case <-d.ctx.Done():
			// Stopped before the start time: Stop must not wait for it.
			d.startTimer.Stop()
			return
		}

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
				case alrt, ok := <-alertCh:
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
					if alrt.Header != nil {
						ctx = d.propagator.Extract(ctx, propagation.MapCarrier(alrt.Header))
					}

					d.routeAlert(ctx, alrt.Data)

				case <-d.ctx.Done():
					return
				}
			}
		}(i)
	}
	<-d.ctx.Done()
}

func (d *Dispatcher) routeAlert(ctx context.Context, alrt *alert.Alert) {
	d.logger.Debug("Received alert", "alert", alrt)

	ctx, span := tracer.Start(ctx, "dispatch.Dispatcher.routeAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alrt.Name()),
			attribute.String("alerting.alert.fingerprint", alrt.Fingerprint().String()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	for _, r := range d.route.Match(alrt.Labels) {
		span.AddEvent("dispatching alert to route",
			trace.WithAttributes(
				attribute.String("alerting.route.receiver.name", r.RouteOpts.Receiver),
			),
		)
		d.groupAlert(ctx, alrt, r)
	}
	d.metrics.processingDuration.Observe(time.Since(now).Seconds())
}

func (d *Dispatcher) doMaintenance() {
	for i := range d.routeGroupsSlice {
		d.routeGroupsSlice[i].groups.Range(func(key, el any) bool {
			ag := el.(*aggrGroup)
			if ag.destroyed() {
				ag.stop()
				deleted := d.routeGroupsSlice[i].groups.CompareAndDelete(key, ag)
				if deleted {
					// TODO(ultrotter, siavash):
					// Deletion from the marker should only happen if we really deleted the group.
					// Fully fixing the case where a new group with the same fingerprint is created between
					// CompareAndDelete and DeleteByGroupKey would require changes to the marker interface,
					// so we leave it as a fix for after landing the pending marker changes.
					d.marker.DeleteByGroupKey(ag.routeID, ag.GroupKey())
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

// AlertFilter decides whether an alert belongs in a Groups result and returns
// the status to report for it. Groups calls the filter once per alert per
// aggregation group and records the returned status in
// AlertGroup.AlertStatuses, so the same alert may legitimately be reported
// with a different status in different groups.
type AlertFilter func(a *alert.Alert, now time.Time) (alert.AlertStatus, bool)

// AlertGroup represents how alerts exist within an aggrGroup.
type AlertGroup struct {
	Alerts      alert.AlertSlice
	Labels      model.LabelSet
	RouteLabels model.LabelSet
	Receiver    string
	GroupKey    string
	RouteID     string
	// AlertStatuses holds the status returned by the AlertFilter for each
	// alert in Alerts, keyed by fingerprint.
	AlertStatuses map[model.Fingerprint]alert.AlertStatus
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
func (d *Dispatcher) Groups(ctx context.Context, routeFilter func(*Route) bool, alertFilter AlertFilter) (AlertGroups, map[model.Fingerprint][]string, error) {
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
			alerts := ag.alerts.List()
			filteredAlerts := make([]*alert.Alert, 0, len(alerts))
			statuses := make(map[model.Fingerprint]alert.AlertStatus, len(alerts))
			for _, a := range alerts {
				status, ok := alertFilter(a, now)
				if !ok {
					continue
				}

				fp := a.Fingerprint()
				statuses[fp] = status
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

			// Compute RouteLabels() only now that the group will be returned:
			// it may render templates on a cache miss, which is wasted work for
			// groups filtered out above.
			alertGroup := &AlertGroup{
				Labels:      ag.labels,
				RouteLabels: ag.RouteLabels(now),
				Receiver:    receiver,
				GroupKey:    ag.GroupKey(),
				RouteID:     ag.routeID,
			}
			alertGroup.Alerts = filteredAlerts
			alertGroup.AlertStatuses = statuses

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
type notifyFunc func(context.Context, ...*alert.Alert) bool

// groupAlert determines in which aggregation group the alert falls
// and inserts it.
func (d *Dispatcher) groupAlert(ctx context.Context, alrt *alert.Alert, route *Route) {
	_, span := tracer.Start(ctx, "dispatch.Dispatcher.groupAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alrt.Name()),
			attribute.String("alerting.alert.fingerprint", alrt.Fingerprint().String()),
			attribute.String("alerting.route.receiver.name", route.RouteOpts.Receiver),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := time.Now()
	groupLabels := getGroupLabels(alrt, route)

	fp := groupLabels.Fingerprint()

	el, loaded := d.routeGroupsSlice[route.Idx].groups.Load(fp)
	if loaded {
		ag := el.(*aggrGroup)
		// Try to insert into the aggrgroup.
		// If it's destroyed insert will return false.
		if ag.insert(ctx, alrt) {
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
		d.logger.Error(message, "err", err.Error(), "groups", current, "limit", limit, "alert", alrt.Name())
		span.SetStatus(codes.Error, message)
		span.RecordError(err,
			trace.WithAttributes(
				attribute.Int("alerting.aggregation_group.count", current),
				attribute.Int("alerting.aggregation_group.limit", limit),
			),
		)
		return
	}

	ag := newAggrGroup(d.ctx, groupLabels, route, d.timeout, d.recorder, d.logger, d.tmpl, &d.routeGroupsSlice[route.Idx].labels)
	// Insert the 1st alert in the group before starting the group's run()
	// function, to make sure that when the run() will be executed the 1st
	// alert is already there.
	ag.insert(ctx, alrt)

	retries := 0
	for {
		if loaded {
			// Try to store the new group in the map. If another goroutine has already created the same group, use the existing one.
			swapped := d.routeGroupsSlice[route.Idx].groups.CompareAndSwap(fp, el, ag)
			if swapped {
				// Since we swapped the new group in, we need to cancel the old one,
				// as doMaintenance will not be able to find it in the map anymore.
				el.(*aggrGroup).cancel()
				// We swapped the new group in, we can break and start it.
				break
			}
			loaded = false
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
			if agExisting.insert(ctx, alrt) {
				return // if we inserted we return to avoid incrementing the aggrgroup count and starting the group.
			}
		}

		// If we failed to swap, it means another goroutine has created/modified the group
		retries++
		d.metrics.aggrGroupCreationRetries.Inc()
		if retries > 100 {
			// This shouldn't happen - indicates a bug or extreme contention
			d.metrics.aggrGroupCreationGivenUp.Inc()
			d.logger.Error("excessive retries creating aggregation group",
				"fingerprint", fp,
				"route", route.Key(),
				"alert", alrt.Name(),
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

	if alrt.StartsAt.Add(ag.opts.GroupWait).Before(now) {
		message := "Alert is old enough for immediate flush, resetting timer to zero"
		ag.logger.Debug(message, "alert", alrt.Name(), "fingerprint", alrt.Fingerprint(), "startsAt", alrt.StartsAt)
		span.AddEvent(message,
			trace.WithAttributes(
				attribute.String("alerting.alert.StartsAt", alrt.StartsAt.Format(time.RFC3339)),
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
	go ag.run(func(ctx context.Context, alerts ...*alert.Alert) bool {
		_, _, err := d.stage.Exec(ctx, d.logger, alerts...)
		if err != nil {
			logger := d.logger.With("aggrGroup", ag.GroupKey(), "num_alerts", len(alerts), "receiver", ag.opts.Receiver, "err", err)
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

func getGroupLabels(alrt *alert.Alert, route *Route) model.LabelSet {
	capacity := len(route.RouteOpts.GroupBy)
	if route.RouteOpts.GroupByAll {
		capacity = len(alrt.Labels)
	}
	groupLabels := make(model.LabelSet, capacity)
	for ln, lv := range alrt.Labels {
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
	groupKey string
	matchers labels.Matchers

	alerts   *store.Alerts
	marker   marker.AlertMarker
	recorder eventrecorder.Recorder
	tmpl     *template.Template
	ctx      context.Context
	cancel   func()
	done     chan struct{}
	next     *time.Timer
	timeout  func(time.Duration) time.Duration
	running  atomic.Bool
	flushIdx uint64

	// labelSplit partitions the route's labels into those whose rendered value
	// is fixed for the group's lifetime (static) and those that can depend on
	// the group's alerts (dynamic); see template.SplitRouteLabels.
	// staticRouteLabels is the static part, rendered once at construction
	// without any alert data. It is never invalidated and must not be mutated.
	labelSplit        *routeLabelSplit
	staticRouteLabels model.LabelSet

	// routeLabels caches the full route label set (static plus dynamic)
	// rendered against the group's current alerts, for the
	// /api/v2/alerts/groups read path. It is only used when the route has
	// dynamic labels. It is rendered lazily and kept up to date with a
	// generation counter rather than a lock, so that invalidation on the hot
	// alert-ingestion path is a single atomic add.
	//
	// routeLabelsGen is bumped (by invalidateRouteLabels) whenever the alert
	// set changes in a way templates can observe. Each cached render is tagged
	// with the generation it was computed at; RouteLabels() treats the cache
	// as valid only if its tag still equals the current generation. A render
	// that races an invalidation is therefore tagged with a now-stale
	// generation: it may be stored, but the next reader sees the mismatch and
	// re-renders, so a stale value is never served.
	//
	// Time is the other input: an alert's status flips from firing to resolved
	// when its EndsAt passes, with no insert or delete, and templates observe
	// that through .Status and .Alerts. Each render therefore also records the
	// earliest such moment among the alerts it saw (validUntil) and is a miss
	// from then on.
	routeLabels    atomic.Pointer[renderedRouteLabels]
	routeLabelsGen atomic.Uint64
}

// renderedRouteLabels is a cached route-label render tagged with the alert-set
// generation it was computed at and the time until which it is valid (zero
// means indefinitely). See aggrGroup.routeLabels.
type renderedRouteLabels struct {
	gen        uint64
	validUntil time.Time
	labels     model.LabelSet
}

// routeLabelSplit is a route's labels partitioned by whether their rendered
// value can depend on the alerts of a group. See template.SplitRouteLabels.
type routeLabelSplit struct {
	static  model.LabelSet
	dynamic model.LabelSet
}

// splitRouteLabels classifies labels with tmpl. Without a template nothing is
// rendered at all, so every label counts as static (and renders to nothing).
func splitRouteLabels(tmpl *template.Template, labels model.LabelSet) routeLabelSplit {
	if tmpl == nil || len(labels) == 0 {
		return routeLabelSplit{static: labels, dynamic: model.LabelSet{}}
	}
	static, dynamic := tmpl.SplitRouteLabels(labels)
	return routeLabelSplit{static: static, dynamic: dynamic}
}

// newAggrGroup returns a new aggregation group.
func newAggrGroup(
	ctx context.Context,
	labels model.LabelSet,
	r *Route,
	to func(time.Duration) time.Duration,
	recorder eventrecorder.Recorder,
	logger *slog.Logger,
	tmpl *template.Template,
	split *routeLabelSplit,
) *aggrGroup {
	if to == nil {
		to = func(d time.Duration) time.Duration { return d }
	}
	if split == nil {
		s := splitRouteLabels(tmpl, r.RouteOpts.Labels)
		split = &s
	}
	ag := &aggrGroup{
		labels:     labels,
		routeID:    r.ID(),
		groupKey:   fmt.Sprintf("%s:%s", r.Key(), labels),
		matchers:   r.Matchers,
		opts:       &r.RouteOpts,
		timeout:    to,
		alerts:     store.NewAlerts(),
		marker:     marker.NewAlertMarker(),
		recorder:   recorder,
		tmpl:       tmpl,
		labelSplit: split,
		done:       make(chan struct{}),
		flushIdx:   1,
	}
	ag.ctx, ag.cancel = context.WithCancel(ctx)

	if id, err := uuid.NewRandom(); err == nil {
		ag.ctx = notify.WithAggrGroupID(ag.ctx, id.String())
	}

	ag.logger = logger.With("aggrGroup", ag.GroupKey())

	// Static labels do not depend on alerts: render them once, now, with none.
	ag.staticRouteLabels = ag.renderLabels(split.static)

	// Set an initial one-time wait before flushing
	// the first batch of notifications.
	ag.next = time.NewTimer(ag.opts.GroupWait)

	return ag
}

func (ag *aggrGroup) fingerprint() model.Fingerprint {
	return ag.labels.Fingerprint()
}

// GroupKey returns the key built from the route and immutable group labels.
// Note that multiple aggregation groups can have the same key.
func (ag *aggrGroup) GroupKey() string {
	return ag.groupKey
}

func (ag *aggrGroup) String() string {
	return ag.GroupKey()
}

// renderLabels renders the given subset of the route's labels as templates
// against the given alerts and the group's data. A route label value may be a
// template, so this allows it to reference group labels, other route labels,
// etc. Labels that fail to render are logged and omitted.
func (ag *aggrGroup) renderLabels(labels model.LabelSet, alerts ...*alert.Alert) model.LabelSet {
	rendered := make(model.LabelSet, len(labels))
	// Nothing to render when there are no labels (the common case) or there
	// is no template engine. Skip building template data, which is O(n_alerts).
	if len(labels) == 0 || ag.tmpl == nil {
		return rendered
	}

	// The notification reason is not available to route labels and is passed as ReasonUnknown.
	// The data carries every raw label so routeLabels cross-references resolve.
	data := ag.tmpl.Data(ag.opts.Receiver, ag.labels, ag.opts.Labels, notify.ReasonUnknown.String(), alerts...)

	logger := ag.logger.With("data", data)

	// Render every label through a single shared resolver so a label that
	// cross-references another (via {{ routeLabels "x" }}) reuses the rendered
	// value instead of re-rendering it per loop iteration.
	render := ag.tmpl.RouteLabelRenderer(data)
	for label, value := range labels {
		if v, err := render(string(label)); err == nil {
			rendered[label] = model.LabelValue(v)
			logger.Debug("rendered route label", "label", label, "value", v)
		} else {
			logger.Error("failed to render route label", "label", label, "value", string(value), "err", err)
		}
	}

	return rendered
}

// hasDynamicRouteLabels reports whether any of the route's labels can depend
// on the group's alerts. When none does, the route labels are fully described
// by staticRouteLabels and no alert-dependent rendering or invalidation is
// ever needed.
func (ag *aggrGroup) hasDynamicRouteLabels() bool {
	return len(ag.labelSplit.dynamic) > 0
}

// renderRouteLabels renders the route's labels against the given alerts: the
// static labels come from the render done at construction, only the dynamic
// ones are rendered now. It is used on the notification path with the batch
// of alerts being flushed. The returned set must not be mutated.
func (ag *aggrGroup) renderRouteLabels(alerts ...*alert.Alert) model.LabelSet {
	if !ag.hasDynamicRouteLabels() {
		return ag.staticRouteLabels
	}
	rendered := ag.renderLabels(ag.labelSplit.dynamic, alerts...)
	maps.Copy(rendered, ag.staticRouteLabels)
	return rendered
}

// RouteLabels returns the route labels rendered against the group's current
// alerts as of now, caching the result. Both the read and the cache update are
// lock-free. The cache is invalidated by invalidateRouteLabels (a generation
// bump) after the alert set changes, and expires on its own when an alert it
// was rendered against resolves (see aggrGroup.routeLabels).
//
// Routes without dynamic labels never touch the cache or the alert store: the
// static render is the answer.
//
// Concurrent callers that all miss may render redundantly; that is acceptable
// because the only caller is the (cold) /alerts/groups API path. What must not
// happen — serving a render that predates an invalidation — is prevented by the
// generation tag. The returned set must not be mutated.
func (ag *aggrGroup) RouteLabels(now time.Time) model.LabelSet {
	if !ag.hasDynamicRouteLabels() {
		return ag.staticRouteLabels
	}

	// Load the cached value before the generation: if an invalidation happens
	// between these two loads, we read the newer generation and treat the cache
	// as a miss, which is safe. (Loading gen first could let us pair an old gen
	// with a value cached for that gen and wrongly accept a stale render.)
	cached := ag.routeLabels.Load()
	gen := ag.routeLabelsGen.Load()
	if cached != nil && cached.gen == gen && (cached.validUntil.IsZero() || now.Before(cached.validUntil)) {
		return cached.labels
	}

	alerts := ag.alerts.List()

	var labels model.LabelSet
	if len(alerts) == 0 {
		// Nothing to render dynamic labels against: e.g. all alerts resolved
		// and deleted and the group is awaiting GC. Rendering templates that
		// reference .Alerts against an empty set would only log errors, and
		// empty groups are excluded from the API response anyway.
		labels = ag.staticRouteLabels
	} else {
		labels = ag.renderRouteLabels(alerts...)
	}

	// Tag the render with the generation we observed before reading the alerts.
	// If an invalidation raced this render, gen is already stale, so the next
	// reader will see the mismatch and re-render rather than serve stale labels.
	ag.routeLabels.Store(&renderedRouteLabels{
		gen:        gen,
		validUntil: nextResolution(alerts, now),
		labels:     labels,
	})
	return labels
}

// nextResolution returns the earliest time after now at which one of alerts
// resolves, or the zero time if no alert has a pending EndsAt. Until then the
// alerts' statuses, and so any render that depends on them, cannot change
// without an insert or a delete.
func nextResolution(alerts []*alert.Alert, now time.Time) time.Time {
	var next time.Time
	for _, a := range alerts {
		if a.EndsAt.IsZero() || !a.EndsAt.After(now) {
			continue
		}
		if next.IsZero() || a.EndsAt.Before(next) {
			next = a.EndsAt
		}
	}
	return next
}

// sameTemplateInputs reports whether two versions of the same alert (same
// fingerprint, hence same labels) are indistinguishable to route label
// templates. Besides the labels, template.Alert exposes the annotations, the
// timestamps (from which the status is derived) and the generator URL.
func sameTemplateInputs(a, b *alert.Alert) bool {
	return a.StartsAt.Equal(b.StartsAt) &&
		a.EndsAt.Equal(b.EndsAt) &&
		a.GeneratorURL == b.GeneratorURL &&
		a.Annotations.Equal(b.Annotations)
}

// invalidateRouteLabels marks the cached route labels stale. It must be called
// after the group's alert set has changed in a way templates can observe and
// the change is visible via ag.alerts, so that any render still in flight is
// tagged with a generation older than this one and will not be served. It is
// only needed for routes with dynamic labels.
func (ag *aggrGroup) invalidateRouteLabels() {
	ag.routeLabelsGen.Add(1)
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
			ctx = notify.WithRouteLabels(ctx, ag.opts.Labels)
			ctx = notify.WithReceiverName(ctx, ag.opts.Receiver)
			ctx = notify.WithRepeatInterval(ctx, ag.opts.RepeatInterval)
			ctx = notify.WithMuteTimeIntervals(ctx, ag.opts.MuteTimeIntervals)
			ctx = notify.WithActiveTimeIntervals(ctx, ag.opts.ActiveTimeIntervals)
			ctx = notify.WithRouteID(ctx, ag.routeID)
			ctx = notify.WithFlushID(ctx, ag.flushIdx)
			ctx = notify.WithGroupMatchers(ctx, ag.matchers)
			ctx = marker.WithContext(ctx, ag.marker)

			ag.flushIdx++

			// Wait the configured interval before calling flush again.
			ag.resetTimer(ag.opts.GroupInterval)

			ag.flush(func(alerts ...*alert.Alert) bool {
				ctx, span := tracer.Start(ctx, "dispatch.AggregationGroup.flush",
					trace.WithAttributes(
						attribute.String("alerting.aggregation_group.key", ag.GroupKey()),
						attribute.Int("alerting.alerts.count", len(alerts)),
					),
					trace.WithSpanKind(trace.SpanKindInternal),
				)
				defer span.End()

				// Render route labels against the batch of alerts that we are
				// about to notify about.
				ctx = notify.WithRouteLabels(ctx, ag.renderRouteLabels(alerts...))

				success := nf(ctx, alerts...)
				if !success {
					span.SetStatus(codes.Error, "notification failed")
				}
				return success
			})

			cancel()

			// If destroyed, exit: this particular alert group won't be used anymore.
			if ag.destroyed() {
				return
			}

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
func (ag *aggrGroup) insert(ctx context.Context, alrt *alert.Alert) bool {
	_, span := tracer.Start(ctx, "dispatch.AggregationGroup.insert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", alrt.Name()),
			attribute.String("alerting.alert.fingerprint", alrt.Fingerprint().String()),
			attribute.String("alerting.aggregation_group.key", ag.GroupKey()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	prev, err := ag.alerts.Swap(alrt)
	if err != nil {
		if errors.Is(err, store.ErrDestroyed) {
			return false
		}
		message := "error on set alert"
		span.SetStatus(codes.Error, message)
		span.RecordError(err)
		ag.logger.Error(message, "err", err)
	} else {
		ag.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
			return notify.NewAlertGroupedEvent(ag.alertGroupInfo(), alrt)
		})
		// The alert is already visible via ag.alerts. Only routes with dynamic
		// labels cache anything alert-dependent, and a re-sent alert that is
		// identical in everything templates can see (the common case while an
		// alert keeps firing) leaves the cached render correct.
		if ag.hasDynamicRouteLabels() && (prev == nil || !sameTemplateInputs(prev, alrt)) {
			ag.invalidateRouteLabels()
		}
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
func (ag *aggrGroup) flush(notify func(...*alert.Alert) bool) {
	if ag.empty() {
		return
	}

	var (
		alerts        = ag.alerts.List()
		alertsSlice   = make(alert.AlertSlice, 0, len(alerts))
		resolvedSlice = make(alert.AlertSlice, 0, len(alerts))
		now           = time.Now()
	)
	for _, alrt := range alerts {
		a := *alrt
		// Ensure that alerts don't resolve as time move forwards.
		if a.ResolvedAt(now) {
			resolvedSlice = append(resolvedSlice, &a)
		} else {
			a.EndsAt = time.Time{}
		}
		alertsSlice = append(alertsSlice, &a)
	}
	sort.Stable(alertsSlice)

	ag.logger.Debug("flushing", "numAlerts", len(alertsSlice), "alerts", alertsSlice)

	if notify(alertsSlice...) {
		ag.recordResolvedEvents(resolvedSlice)

		// Delete all resolved alerts as we just sent a notification for them,
		// and we don't want to send another one. However, we need to make sure
		// that each resolved alert has not fired again during the flush as then
		// we would delete an active alert thinking it was resolved.
		// Since we are passing DestroyIfEmpty=true the group will be marked as
		// destroyed if there are no more alerts after the deletion.
		if err := ag.alerts.DeleteIfNotModified(resolvedSlice, true); err != nil {
			ag.logger.Error("error on delete alerts", "err", err)
		} else {
			if len(resolvedSlice) > 0 && ag.hasDynamicRouteLabels() {
				// The alert set changed; the deletion is already visible via
				// ag.alerts.
				ag.invalidateRouteLabels()
			}
			// Delete markers for resolved alerts that are not in the store.
			for _, alrt := range resolvedSlice {
				_, err := ag.alerts.Get(alrt.Fingerprint())
				if errors.Is(err, store.ErrNotFound) {
					ag.marker.Delete(alrt.Fingerprint())
				}
			}
		}
	}
}

func (ag *aggrGroup) recordResolvedEvents(resolved alert.AlertSlice) {
	if len(resolved) == 0 {
		return
	}
	for _, a := range resolved {
		ag.recorder.RecordEvent(ag.ctx, func() eventrecorder.EventData {
			return notify.NewAlertResolvedEvent(ag.alertGroupInfo(), a)
		})
	}
}

func (ag *aggrGroup) alertGroupInfo() eventrecorder.AlertGroup {
	return eventrecorder.NewAlertGroup(
		ag.GroupKey(), ag.labels, notify.Key(ag.GroupKey()).Hash(), ag.opts.Receiver, nil, "",
	)
}

type nilLimits struct{}

func (n nilLimits) MaxNumberOfAggregationGroups() int { return 0 }
