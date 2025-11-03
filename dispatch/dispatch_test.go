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
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

const testMaintenanceInterval = 30 * time.Second

func TestAggrGroup(t *testing.T) {
	lset := model.LabelSet{
		"a": "v1",
		"b": "v2",
	}
	opts := &RouteOpts{
		Receiver: "n1",
		GroupBy: map[model.LabelName]struct{}{
			"a": {},
			"b": {},
		},
		GroupWait:      1 * time.Second,
		GroupInterval:  300 * time.Millisecond,
		RepeatInterval: 1 * time.Hour,
	}
	route := &route{
		RouteOpts: *opts,
	}

	var (
		a1 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v3",
				},
				StartsAt: time.Now().Add(time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
			UpdatedAt: time.Now(),
		}
		a2 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v4",
				},
				StartsAt: time.Now().Add(-time.Hour),
				EndsAt:   time.Now().Add(2 * time.Hour),
			},
			UpdatedAt: time.Now(),
		}
		a3 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v5",
				},
				StartsAt: time.Now().Add(time.Minute),
				EndsAt:   time.Now().Add(5 * time.Minute),
			},
			UpdatedAt: time.Now(),
		}
	)

	var (
		last       = time.Now()
		current    = time.Now()
		lastCurMtx = &sync.Mutex{}
		alertsCh   = make(chan types.AlertSlice)
	)

	ntfy := func(ctx context.Context, alerts ...*types.Alert) bool {
		// Validate that the context is properly populated.
		if _, ok := notify.Now(ctx); !ok {
			t.Errorf("now missing")
		}
		if _, ok := notify.GroupKey(ctx); !ok {
			t.Errorf("group key missing")
		}
		if lbls, ok := notify.GroupLabels(ctx); !ok || !reflect.DeepEqual(lbls, lset) {
			t.Errorf("wrong group labels: %q", lbls)
		}
		if rcv, ok := notify.ReceiverName(ctx); !ok || rcv != opts.Receiver {
			t.Errorf("wrong receiver: %q", rcv)
		}
		if ri, ok := notify.RepeatInterval(ctx); !ok || ri != opts.RepeatInterval {
			t.Errorf("wrong repeat interval: %q", ri)
		}

		lastCurMtx.Lock()
		last = current
		// Subtract a millisecond to allow for races.
		current = time.Now().Add(-time.Millisecond)
		lastCurMtx.Unlock()

		alertsCh <- types.AlertSlice(alerts)

		return true
	}

	removeEndsAt := func(as types.AlertSlice) types.AlertSlice {
		for i, a := range as {
			ac := *a
			ac.EndsAt = time.Time{}
			as[i] = &ac
		}
		return as
	}

	// Test regular situation where we wait for group_wait to send out alerts.
	ag := newAggrGroup(context.Background(), lset, route, nil, types.NewMarker(prometheus.NewRegistry()), promslog.NewNopLogger())
	go ag.run(ntfy)

	ag.insert(a1)

	select {
	case <-time.After(2 * opts.GroupWait):
		t.Fatalf("expected initial batch after group_wait")

	case batch := <-alertsCh:
		lastCurMtx.Lock()
		s := time.Since(last)
		lastCurMtx.Unlock()
		if s < opts.GroupWait {
			t.Fatalf("received batch too early after %v", s)
		}
		exp := removeEndsAt(types.AlertSlice{a1})
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}
	}

	for i := 0; i < 3; i++ {
		// New alert should come in after group interval.
		ag.insert(a3)

		select {
		case <-time.After(2 * opts.GroupInterval):
			t.Fatalf("expected new batch after group interval but received none")

		case batch := <-alertsCh:
			lastCurMtx.Lock()
			s := time.Since(last)
			lastCurMtx.Unlock()
			if s < opts.GroupInterval {
				t.Fatalf("received batch too early after %v", s)
			}
			exp := removeEndsAt(types.AlertSlice{a1, a3})
			sort.Sort(batch)

			if !reflect.DeepEqual(batch, exp) {
				t.Fatalf("expected alerts %v but got %v", exp, batch)
			}
		}
	}

	ag.stop()

	// Add an alert that started more than group_interval in the past. We expect
	// immediate flushing.
	// Finally, set all alerts to be resolved. After successful notify the aggregation group
	// should empty itself.
	ag = newAggrGroup(context.Background(), lset, route, nil, types.NewMarker(prometheus.NewRegistry()), promslog.NewNopLogger())
	go ag.run(ntfy)

	ag.insert(a1)
	ag.insert(a2)

	// a2 lies way in the past so the initial group_wait should be skipped.
	select {
	case <-time.After(opts.GroupWait / 2):
		t.Fatalf("expected immediate alert but received none")

	case batch := <-alertsCh:
		exp := removeEndsAt(types.AlertSlice{a1, a2})
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}
	}

	for i := 0; i < 3; i++ {
		// New alert should come in after group interval.
		ag.insert(a3)

		select {
		case <-time.After(2 * opts.GroupInterval):
			t.Fatalf("expected new batch after group interval but received none")

		case batch := <-alertsCh:
			lastCurMtx.Lock()
			s := time.Since(last)
			lastCurMtx.Unlock()
			if s < opts.GroupInterval {
				t.Fatalf("received batch too early after %v", s)
			}
			exp := removeEndsAt(types.AlertSlice{a1, a2, a3})
			sort.Sort(batch)

			if !reflect.DeepEqual(batch, exp) {
				t.Fatalf("expected alerts %v but got %v", exp, batch)
			}
		}
	}

	// Resolve an alert, and it should be removed after the next batch was sent.
	a1r := *a1
	a1r.EndsAt = time.Now()
	ag.insert(&a1r)
	exp := append(types.AlertSlice{&a1r}, removeEndsAt(types.AlertSlice{a2, a3})...)

	select {
	case <-time.After(2 * opts.GroupInterval):
		t.Fatalf("expected new batch after group interval but received none")
	case batch := <-alertsCh:
		lastCurMtx.Lock()
		s := time.Since(last)
		lastCurMtx.Unlock()
		if s < opts.GroupInterval {
			t.Fatalf("received batch too early after %v", s)
		}
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}
	}

	// Resolve all remaining alerts, they should be removed after the next batch was sent.
	// Do not add a1r as it should have been deleted following the previous batch.
	a2r, a3r := *a2, *a3
	resolved := types.AlertSlice{&a2r, &a3r}
	for _, a := range resolved {
		a.EndsAt = time.Now()
		ag.insert(a)
	}

	select {
	case <-time.After(2 * opts.GroupInterval):
		t.Fatalf("expected new batch after group interval but received none")

	case batch := <-alertsCh:
		lastCurMtx.Lock()
		s := time.Since(last)
		lastCurMtx.Unlock()
		if s < opts.GroupInterval {
			t.Fatalf("received batch too early after %v", s)
		}
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, resolved) {
			t.Fatalf("expected alerts %v but got %v", resolved, batch)
		}

		if !ag.empty() {
			t.Fatalf("Expected aggregation group to be empty after resolving alerts: %v", ag)
		}
	}

	ag.stop()
}

func TestGroupLabels(t *testing.T) {
	a := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"a": "v1",
				"b": "v2",
				"c": "v3",
			},
		},
	}

	route := &route{
		RouteOpts: RouteOpts{
			GroupBy: map[model.LabelName]struct{}{
				"a": {},
				"b": {},
			},
			GroupByAll: false,
		},
	}

	expLs := model.LabelSet{
		"a": "v1",
		"b": "v2",
	}

	ls := getGroupLabels(a, route)

	if !reflect.DeepEqual(ls, expLs) {
		t.Fatalf("expected labels are %v, but got %v", expLs, ls)
	}
}

func TestGroupByAllLabels(t *testing.T) {
	a := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"a": "v1",
				"b": "v2",
				"c": "v3",
			},
		},
	}

	route := &route{
		RouteOpts: RouteOpts{
			GroupBy:    map[model.LabelName]struct{}{},
			GroupByAll: true,
		},
	}

	expLs := model.LabelSet{
		"a": "v1",
		"b": "v2",
		"c": "v3",
	}

	ls := getGroupLabels(a, route)

	if !reflect.DeepEqual(ls, expLs) {
		t.Fatalf("expected labels are %v, but got %v", expLs, ls)
	}
}

type recordStage struct {
	mtx    sync.RWMutex
	alerts map[string]map[model.Fingerprint]*types.Alert
}

func (r *recordStage) Alerts() []*types.Alert {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	alerts := make([]*types.Alert, 0)
	for k := range r.alerts {
		for _, a := range r.alerts[k] {
			alerts = append(alerts, a)
		}
	}
	return alerts
}

func (r *recordStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	gk, ok := notify.GroupKey(ctx)
	if !ok {
		panic("GroupKey not present!")
	}
	if _, ok := r.alerts[gk]; !ok {
		r.alerts[gk] = make(map[model.Fingerprint]*types.Alert)
	}
	for _, a := range alerts {
		r.alerts[gk][a.Fingerprint()] = a
	}
	return ctx, nil, nil
}

var (
	// Set the start time in the past to trigger a flush immediately.
	t0 = time.Now().Add(-time.Minute)
	// Set the end time in the future to avoid deleting the alert.
	t1 = t0.Add(2 * time.Minute)
)

func newAlert(labels model.LabelSet) *types.Alert {
	return &types.Alert{
		Alert: model.Alert{
			Labels:       labels,
			Annotations:  model.LabelSet{"foo": "bar"},
			StartsAt:     t0,
			EndsAt:       t1,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: t0,
		Timeout:   false,
	}
}

func TestDispatcherRace(t *testing.T) {
	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	marker := types.NewMarker(reg)
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	dispatcher := NewDispatcher(alerts, nil, nil, marker, timeout, testMaintenanceInterval, nil, logger, NewDispatcherMetrics(false, reg))
	go dispatcher.Run()
	dispatcher.Stop()
}

func TestDispatcherRaceOnFirstAlertNotDeliveredWhenGroupWaitIsZero(t *testing.T) {
	const numAlerts = 5000

	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	marker := types.NewMarker(reg)
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	route := &route{
		RouteOpts: RouteOpts{
			Receiver:       "default",
			GroupBy:        map[model.LabelName]struct{}{"alertname": {}},
			GroupWait:      0,
			GroupInterval:  1 * time.Hour, // Should never hit in this test.
			RepeatInterval: 1 * time.Hour, // Should never hit in this test.
		},
	}

	timeout := func(d time.Duration) time.Duration { return d }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, testMaintenanceInterval, nil, logger, NewDispatcherMetrics(false, reg))
	go dispatcher.Run()
	defer dispatcher.Stop()

	// Push all alerts.
	for i := 0; i < numAlerts; i++ {
		alert := newAlert(model.LabelSet{"alertname": model.LabelValue(fmt.Sprintf("Alert_%d", i))})
		require.NoError(t, alerts.Put(alert))
	}

	// Wait until the alerts have been notified or the waiting timeout expires.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if len(recorder.Alerts()) >= numAlerts {
			break
		}

		// Throttle.
		time.Sleep(10 * time.Millisecond)
	}

	// We expect all alerts to be notified immediately, since they all belong to different groups.
	require.Len(t, recorder.Alerts(), numAlerts)
}

func TestDispatcher_DoMaintenance(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := types.NewMarker(r)

	alerts, err := mem.NewAlerts(context.Background(), marker, time.Minute, nil, promslog.NewNopLogger(), r)
	if err != nil {
		t.Fatal(err)
	}

	route := &route{
		RouteOpts: RouteOpts{
			GroupBy:       map[model.LabelName]struct{}{"alertname": {}},
			GroupWait:     0,
			GroupInterval: 5 * time.Minute, // Should never hit in this test.
		},
	}
	timeout := func(d time.Duration) time.Duration { return d }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}

	ctx := context.Background()
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, testMaintenanceInterval, nil, promslog.NewNopLogger(), NewDispatcherMetrics(false, r))
	aggrGroups := make(map[Route]map[model.Fingerprint]*aggrGroup)
	aggrGroups[route] = make(map[model.Fingerprint]*aggrGroup)

	// Insert an aggregation group with no alerts.
	labels := model.LabelSet{"alertname": "1"}
	aggrGroup1 := newAggrGroup(ctx, labels, route, timeout, types.NewMarker(prometheus.NewRegistry()), promslog.NewNopLogger())
	aggrGroups[route][aggrGroup1.fingerprint()] = aggrGroup1
	dispatcher.aggrGroupsPerRoute = aggrGroups
	// Must run otherwise doMaintenance blocks on aggrGroup1.stop().
	go aggrGroup1.run(func(context.Context, ...*types.Alert) bool { return true })

	// Insert a marker for the aggregation group's group key.
	marker.SetMuted(route.ID(), aggrGroup1.GroupKey(), []string{"weekends"})
	mutedBy, isMuted := marker.Muted(route.ID(), aggrGroup1.GroupKey())
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, mutedBy)

	// Run the maintenance and the marker should be removed.
	dispatcher.doMaintenance()
	mutedBy, isMuted = marker.Muted(route.ID(), aggrGroup1.GroupKey())
	require.False(t, isMuted)
	require.Empty(t, mutedBy)
}

func TestDispatcher_DeleteResolvedAlertsFromMarker(t *testing.T) {
	t.Run("successful flush deletes markers for resolved alerts", func(t *testing.T) {
		ctx := context.Background()
		marker := types.NewMarker(prometheus.NewRegistry())
		labels := model.LabelSet{"alertname": "TestAlert"}
		route := &route{
			RouteOpts: RouteOpts{
				Receiver:       "test",
				GroupBy:        map[model.LabelName]struct{}{"alertname": {}},
				GroupWait:      0,
				GroupInterval:  time.Minute,
				RepeatInterval: time.Hour,
			},
		}
		timeout := func(d time.Duration) time.Duration { return d }
		logger := promslog.NewNopLogger()

		// Create an aggregation group
		ag := newAggrGroup(ctx, labels, route, timeout, marker, logger)

		// Create test alerts: one active and one resolved
		now := time.Now()
		activeAlert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
					"instance":  "1",
				},
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(time.Hour), // Active alert
			},
			UpdatedAt: now,
		}
		resolvedAlert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
					"instance":  "2",
				},
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(-time.Minute), // Resolved alert
			},
			UpdatedAt: now,
		}

		// Insert alerts into the aggregation group
		ag.insert(activeAlert)
		ag.insert(resolvedAlert)

		// Set markers for both alerts
		marker.SetActiveOrSilenced(activeAlert.Fingerprint(), 0, nil, nil)
		marker.SetActiveOrSilenced(resolvedAlert.Fingerprint(), 0, nil, nil)

		// Verify markers exist before flush
		require.True(t, marker.Active(activeAlert.Fingerprint()))
		require.True(t, marker.Active(resolvedAlert.Fingerprint()))

		// Create a notify function that succeeds
		notifyFunc := func(alerts ...*types.Alert) bool {
			return true
		}

		// Flush the alerts
		ag.flush(notifyFunc)

		// Verify that the resolved alert's marker was deleted
		require.True(t, marker.Active(activeAlert.Fingerprint()), "active alert marker should still exist")
		require.False(t, marker.Active(resolvedAlert.Fingerprint()), "resolved alert marker should be deleted")
	})

	t.Run("failed flush does not delete markers", func(t *testing.T) {
		ctx := context.Background()
		marker := types.NewMarker(prometheus.NewRegistry())
		labels := model.LabelSet{"alertname": "TestAlert"}
		route := &route{
			RouteOpts: RouteOpts{
				Receiver:       "test",
				GroupBy:        map[model.LabelName]struct{}{"alertname": {}},
				GroupWait:      0,
				GroupInterval:  time.Minute,
				RepeatInterval: time.Hour,
			},
		}
		timeout := func(d time.Duration) time.Duration { return d }
		logger := promslog.NewNopLogger()

		// Create an aggregation group
		ag := newAggrGroup(ctx, labels, route, timeout, marker, logger)

		// Create a resolved alert
		now := time.Now()
		resolvedAlert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
					"instance":  "1",
				},
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(-time.Minute), // Resolved alert
			},
			UpdatedAt: now,
		}

		// Insert alert into the aggregation group
		ag.insert(resolvedAlert)

		// Set marker for the alert
		marker.SetActiveOrSilenced(resolvedAlert.Fingerprint(), 0, nil, nil)

		// Verify marker exists before flush
		require.True(t, marker.Active(resolvedAlert.Fingerprint()))

		// Create a notify function that fails
		notifyFunc := func(alerts ...*types.Alert) bool {
			return false
		}

		// Flush the alerts (notify will fail)
		ag.flush(notifyFunc)

		// Verify that the marker was NOT deleted due to failed notification
		require.True(t, marker.Active(resolvedAlert.Fingerprint()), "marker should not be deleted when notify fails")
	})

	t.Run("markers not deleted when alert is modified during flush", func(t *testing.T) {
		ctx := context.Background()
		marker := types.NewMarker(prometheus.NewRegistry())
		labels := model.LabelSet{"alertname": "TestAlert"}
		route := &route{
			RouteOpts: RouteOpts{
				Receiver:       "test",
				GroupBy:        map[model.LabelName]struct{}{"alertname": {}},
				GroupWait:      0,
				GroupInterval:  time.Minute,
				RepeatInterval: time.Hour,
			},
		}
		timeout := func(d time.Duration) time.Duration { return d }
		logger := promslog.NewNopLogger()

		// Create an aggregation group
		ag := newAggrGroup(ctx, labels, route, timeout, marker, logger)

		// Create a resolved alert
		now := time.Now()
		resolvedAlert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
					"instance":  "1",
				},
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(-time.Minute), // Resolved alert
			},
			UpdatedAt: now,
		}

		// Insert alert into the aggregation group
		ag.insert(resolvedAlert)

		// Set marker for the alert
		marker.SetActiveOrSilenced(resolvedAlert.Fingerprint(), 0, nil, nil)

		// Verify marker exists before flush
		require.True(t, marker.Active(resolvedAlert.Fingerprint()))

		// Create a notify function that modifies the alert before returning
		notifyFunc := func(alerts ...*types.Alert) bool {
			// Simulate the alert being modified (e.g., firing again) during flush
			modifiedAlert := &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "TestAlert",
						"instance":  "1",
					},
					StartsAt: now.Add(-time.Hour),
					EndsAt:   now.Add(time.Hour), // Active again
				},
				UpdatedAt: now.Add(time.Second), // More recent update
			}
			// Update the alert in the store
			ag.alerts.Set(modifiedAlert)
			return true
		}

		// Flush the alerts
		ag.flush(notifyFunc)

		// Verify that the marker was NOT deleted because the alert was modified
		// during the flush (DeleteIfNotModified should have failed)
		require.True(t, marker.Active(resolvedAlert.Fingerprint()), "marker should not be deleted when alert is modified during flush")
	})
}
