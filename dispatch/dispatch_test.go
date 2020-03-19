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
		Receiver: "n1",
		GroupBy: map[model.LabelName]struct{}{
			"a": struct{}{},
			"b": struct{}{},
		},
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
	confData := `receivers:
- name: 'kafka'
- name: 'prod'
- name: 'testing'

route:
  group_by: ['alertname']
  group_wait: 10ms
  group_interval: 10ms
  receiver: 'prod'
  routes:
  - match:
      env: 'testing'
    receiver: 'testing'
    group_by: ['alertname', 'service']
  - match:
      env: 'prod'
    receiver: 'prod'
    group_by: ['alertname', 'service', 'cluster']
    continue: true
  - match:
      kafka: 'yes'
    receiver: 'kafka'
    group_by: ['alertname', 'service', 'cluster']`
	conf, err := config.Load(confData)
	if err != nil {
		t.Fatal(err)
	}

	logger := log.NewNopLogger()
	route := NewRoute(conf.Route, nil)
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, logger, NewDispatcherMetrics(prometheus.NewRegistry()))
	go dispatcher.Run()
	defer dispatcher.Stop()

	// Create alerts. the dispatcher will automatically create the groups.
	inputAlerts := []*types.Alert{
		// Matches the parent route.
		newAlert(model.LabelSet{"alertname": "OtherAlert", "cluster": "cc", "service": "dd"}),
		// Matches the first sub-route.
		newAlert(model.LabelSet{"env": "testing", "alertname": "TestingAlert", "service": "api", "instance": "inst1"}),
		// Matches the second sub-route.
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighErrorRate", "cluster": "aa", "service": "api", "instance": "inst1"}),
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighErrorRate", "cluster": "aa", "service": "api", "instance": "inst2"}),
		// Matches the second sub-route.
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighErrorRate", "cluster": "bb", "service": "api", "instance": "inst1"}),
		// Matches the second and third sub-route.
		newAlert(model.LabelSet{"env": "prod", "alertname": "HighLatency", "cluster": "bb", "service": "db", "kafka": "yes", "instance": "inst3"}),
	}
	alerts.Put(inputAlerts...)

	// Let alerts get processed.
	for i := 0; len(recorder.Alerts()) != 7 && i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
	}
	require.Equal(t, 7, len(recorder.Alerts()))

	alertGroups, receivers := dispatcher.Groups(
		func(*Route) bool {
			return true
		}, func(*types.Alert, time.Time) bool {
			return true
		},
	)

	require.Equal(t, AlertGroups{
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[0]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("OtherAlert"),
			},
			Receiver: "prod",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[1]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("TestingAlert"),
				model.LabelName("service"):   model.LabelValue("api"),
			},
			Receiver: "testing",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[2], inputAlerts[3]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("HighErrorRate"),
				model.LabelName("service"):   model.LabelValue("api"),
				model.LabelName("cluster"):   model.LabelValue("aa"),
			},
			Receiver: "prod",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[4]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("HighErrorRate"),
				model.LabelName("service"):   model.LabelValue("api"),
				model.LabelName("cluster"):   model.LabelValue("bb"),
			},
			Receiver: "prod",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[5]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("HighLatency"),
				model.LabelName("service"):   model.LabelValue("db"),
				model.LabelName("cluster"):   model.LabelValue("bb"),
			},
			Receiver: "kafka",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[5]},
			Labels: model.LabelSet{
				model.LabelName("alertname"): model.LabelValue("HighLatency"),
				model.LabelName("service"):   model.LabelValue("db"),
				model.LabelName("cluster"):   model.LabelValue("bb"),
			},
			Receiver: "prod",
		},
	}, alertGroups)
	require.Equal(t, map[model.Fingerprint][]string{
		inputAlerts[0].Fingerprint(): []string{"prod"},
		inputAlerts[1].Fingerprint(): []string{"testing"},
		inputAlerts[2].Fingerprint(): []string{"prod"},
		inputAlerts[3].Fingerprint(): []string{"prod"},
		inputAlerts[4].Fingerprint(): []string{"prod"},
		inputAlerts[5].Fingerprint(): []string{"kafka", "prod"},
	}, receivers)
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

func (r *recordStage) Exec(ctx context.Context, l log.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
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
	logger := log.NewNopLogger()
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	dispatcher := NewDispatcher(alerts, nil, nil, marker, timeout, logger, NewDispatcherMetrics(prometheus.NewRegistry()))
	go dispatcher.Run()
	dispatcher.Stop()
}
