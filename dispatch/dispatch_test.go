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
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

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

	logger := promslog.NewNopLogger()
	route := NewRoute(conf.Route, nil)
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, nil, logger, NewDispatcherMetrics(false, prometheus.NewRegistry()))
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
	require.Len(t, recorder.Alerts(), 7)

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
				"alertname": "OtherAlert",
			},
			Receiver: "prod",
			GroupKey: "{}:{alertname=\"OtherAlert\"}",
			RouteID:  "{}",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[1]},
			Labels: model.LabelSet{
				"alertname": "TestingAlert",
				"service":   "api",
			},
			Receiver: "testing",
			GroupKey: "{}/{env=\"testing\"}:{alertname=\"TestingAlert\", service=\"api\"}",
			RouteID:  "{}/{env=\"testing\"}/0",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[2], inputAlerts[3]},
			Labels: model.LabelSet{
				"alertname": "HighErrorRate",
				"service":   "api",
				"cluster":   "aa",
			},
			Receiver: "prod",
			GroupKey: "{}/{env=\"prod\"}:{alertname=\"HighErrorRate\", cluster=\"aa\", service=\"api\"}",
			RouteID:  "{}/{env=\"prod\"}/1",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[4]},
			Labels: model.LabelSet{
				"alertname": "HighErrorRate",
				"service":   "api",
				"cluster":   "bb",
			},
			Receiver: "prod",
			GroupKey: "{}/{env=\"prod\"}:{alertname=\"HighErrorRate\", cluster=\"bb\", service=\"api\"}",
			RouteID:  "{}/{env=\"prod\"}/1",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[5]},
			Labels: model.LabelSet{
				"alertname": "HighLatency",
				"service":   "db",
				"cluster":   "bb",
			},
			Receiver: "kafka",
			GroupKey: "{}/{kafka=\"yes\"}:{alertname=\"HighLatency\", cluster=\"bb\", service=\"db\"}",
			RouteID:  "{}/{kafka=\"yes\"}/2",
		},
		&AlertGroup{
			Alerts: []*types.Alert{inputAlerts[5]},
			Labels: model.LabelSet{
				"alertname": "HighLatency",
				"service":   "db",
				"cluster":   "bb",
			},
			Receiver: "prod",
			GroupKey: "{}/{env=\"prod\"}:{alertname=\"HighLatency\", cluster=\"bb\", service=\"db\"}",
			RouteID:  "{}/{env=\"prod\"}/1",
		},
	}, alertGroups)
	require.Equal(t, map[model.Fingerprint][]string{
		inputAlerts[0].Fingerprint(): {"prod"},
		inputAlerts[1].Fingerprint(): {"testing"},
		inputAlerts[2].Fingerprint(): {"prod"},
		inputAlerts[3].Fingerprint(): {"prod"},
		inputAlerts[4].Fingerprint(): {"prod"},
		inputAlerts[5].Fingerprint(): {"kafka", "prod"},
	}, receivers)
}

func TestGroupsWithLimits(t *testing.T) {
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

	logger := promslog.NewNopLogger()
	route := NewRoute(conf.Route, nil)
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	lim := limits{groups: 6}
	m := NewDispatcherMetrics(true, prometheus.NewRegistry())
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, lim, logger, m)
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
	err = alerts.Put(inputAlerts...)
	if err != nil {
		t.Fatal(err)
	}

	// Let alerts get processed.
	for i := 0; len(recorder.Alerts()) != 7 && i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
	}
	require.Len(t, recorder.Alerts(), 7)

	routeFilter := func(*Route) bool { return true }
	alertFilter := func(*types.Alert, time.Time) bool { return true }

	alertGroups, _ := dispatcher.Groups(routeFilter, alertFilter)
	require.Len(t, alertGroups, 6)

	require.Equal(t, 0.0, testutil.ToFloat64(m.aggrGroupLimitReached))

	// Try to store new alert. This time, we will hit limit for number of groups.
	err = alerts.Put(newAlert(model.LabelSet{"env": "prod", "alertname": "NewAlert", "cluster": "new-cluster", "service": "db"}))
	if err != nil {
		t.Fatal(err)
	}

	// Let alert get processed.
	for i := 0; testutil.ToFloat64(m.aggrGroupLimitReached) == 0 && i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
	}
	require.Equal(t, 1.0, testutil.ToFloat64(m.aggrGroupLimitReached))

	// Verify there are still only 6 groups.
	alertGroups, _ = dispatcher.Groups(routeFilter, alertFilter)
	require.Len(t, alertGroups, 6)
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
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	dispatcher := NewDispatcher(alerts, nil, nil, marker, timeout, nil, logger, NewDispatcherMetrics(false, prometheus.NewRegistry()))
	go dispatcher.Run()
	dispatcher.Stop()
}

func TestDispatcherRaceOnFirstAlertNotDeliveredWhenGroupWaitIsZero(t *testing.T) {
	const numAlerts = 5000

	logger := promslog.NewNopLogger()
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer alerts.Close()

	route := &Route{
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
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, nil, logger, NewDispatcherMetrics(false, prometheus.NewRegistry()))
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

type limits struct {
	groups int
}

func (l limits) MaxNumberOfAggregationGroups() int {
	return l.groups
}

func TestDispatcher_DoMaintenance(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := types.NewMarker(r)

	alerts, err := mem.NewAlerts(context.Background(), marker, time.Minute, nil, promslog.NewNopLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	route := &Route{
		RouteOpts: RouteOpts{
			GroupBy:       map[model.LabelName]struct{}{"alertname": {}},
			GroupWait:     0,
			GroupInterval: 5 * time.Minute, // Should never hit in this test.
		},
	}
	timeout := func(d time.Duration) time.Duration { return d }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}

	ctx := context.Background()
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, nil, promslog.NewNopLogger(), NewDispatcherMetrics(false, r))

	// Initialize the dispatcher's aggrGroupsPerRoute directly (avoid copying the struct)
	dispatcher.aggrGroupsPerRoute = routeGroups{
		groupsNum: &atomic.Int64{},
		limits:    nilLimits{},
	}
	groupsMap := dispatcher.aggrGroupsPerRoute.AddRoute(route)

	// Insert an aggregation group with no alerts.
	labels := model.LabelSet{"alertname": "1"}
	aggrGroup1 := newAggrGroup(ctx, labels, route, timeout, promslog.NewNopLogger())
	groupsMap.AddGroup(aggrGroup1.fingerprint(), aggrGroup1)
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
