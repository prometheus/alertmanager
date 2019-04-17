// Copyright 2018 Prometheus Team
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
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

func TestAggrGroup(t *testing.T) {
	lset := model.LabelSet{
		"a": "v1",
		"b": "v2",
	}
	opts := &RouteOpts{
		Receiver:       "n1",
		GroupBy:        map[model.LabelName]struct{}{},
		GroupWait:      1 * time.Second,
		GroupInterval:  300 * time.Millisecond,
		RepeatInterval: 1 * time.Hour,
	}
	route := &Route{
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
	ag := newAggrGroup(context.Background(), lset, route, nil, log.NewNopLogger())
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
	ag = newAggrGroup(context.Background(), lset, route, nil, log.NewNopLogger())
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

	// Resolve all alerts, they should be removed after the next batch was sent.
	a1r, a2r, a3r := *a1, *a2, *a3
	resolved := types.AlertSlice{&a1r, &a2r, &a3r}
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
	var a = &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"a": "v1",
				"b": "v2",
				"c": "v3",
			},
		},
	}

	route := &Route{
		RouteOpts: RouteOpts{
			GroupBy: map[model.LabelName]struct{}{
				"a": struct{}{},
				"b": struct{}{},
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
	var a = &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"a": "v1",
				"b": "v2",
				"c": "v3",
			},
		},
	}

	route := &Route{
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

func TestGroups(t *testing.T) {
	logger := log.NewNopLogger()
	conf, _, err := config.LoadFile("testdata/conf.yml")
	if err != nil {
		t.Fatal(err)
	}
	route := NewRoute(conf.Route, nil)
	marker := types.NewMarker(prometheus.DefaultRegisterer)
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()
	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	dispatcher := NewDispatcher(alerts, route, &noopStage{}, marker, timeout, logger)
	go dispatcher.Run()

	// create alerts. the dispatcher will automatically create the groups
	alerts.Put(
		newAlert(model.LabelSet{"env": "testing", "alertname": "TestingAlert", "service": "api"}),
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighErrorRate", "cluster": "aa", "service": "api"}),
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighErrorRate", "cluster": "bb", "service": "api"}),
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighLatency", "cluster": "bb", "service": "db", "kafka": "yes"}),
	)
	// Let alerts get processed
	time.Sleep(time.Second)

	var routeFilter, alertFilter bool
	alertGroups, receivers := dispatcher.Groups(func(_ *Route) bool {
		routeFilter = true
		return true
	}, func(_ *types.Alert, _ time.Time) bool {
		alertFilter = true
		return true
	})

	// Verify filter functions were called
	require.True(t, routeFilter)
	require.True(t, alertFilter)

	// Verify grouping works
	require.Equal(t, 5, len(alertGroups))
	for _, ag := range alertGroups {
		if len(ag.Labels) == 2 {
			// testing receiver
			require.Equal(t, 2, len(ag.Labels))
			require.Equal(t, model.LabelSet{"alertname": "TestingAlert", "service": "api"}, ag.Labels)
			for _, alert := range ag.Alerts {
				alertsReceivers, ok := receivers[alert.Fingerprint()]
				require.True(t, ok)
				require.Equal(t, 1, len(alertsReceivers))
				require.Equal(t, "testing", alertsReceivers[0])
			}
			continue
		}
		require.Equal(t, 3, len(ag.Labels))
		for _, alert := range ag.Alerts {
			alertsReceivers, ok := receivers[alert.Fingerprint()]
			require.True(t, ok)
			if labelValue := ag.Labels["alertname"]; string(labelValue) == "HighLatency" {
				// Matches both prod and kafka receivers
				require.Equal(t, []string{"prod", "kafka"}, alertsReceivers)
				continue
			}
			require.Equal(t, 1, len(alertsReceivers))
			require.Equal(t, "prod", alertsReceivers[0])
		}
	}
}

type noopStage struct{}

func (n *noopStage) Exec(ctx context.Context, l log.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	return ctx, nil, nil
}

var (
	t0 = time.Now()
	t1 = t0.Add(100 * time.Millisecond)
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
