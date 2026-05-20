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
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
)

type groupKeyMetricsFlagger struct {
	featurecontrol.NoopFlags
}

func (groupKeyMetricsFlagger) EnableGroupKeyInMetrics() bool { return true }

func TestAlertStateCollector_SuppressedMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewDispatcherMetrics(false, reg, featurecontrol.NoopFlags{})
	dispatcher := newRunningMetricsDispatcher(metrics, 2)

	routeOne := testMetricRoute(0)
	routeTwo := testMetricRoute(1)

	active := testMetricAlert("active", time.Now().Add(time.Hour))
	silenced := testMetricAlert("silenced", time.Now().Add(time.Hour))
	inhibited := testMetricAlert("inhibited", time.Now().Add(time.Hour))
	silencedAndInhibited := testMetricAlert("silenced-and-inhibited", time.Now().Add(time.Hour))
	duplicateSilenced := testMetricAlert("duplicate-silenced", time.Now().Add(time.Hour))
	resolvedSilenced := testMetricAlert("resolved-silenced", time.Now().Add(-time.Hour))

	groupOne := testMetricAggrGroup(t, routeOne, model.LabelSet{"group": "one"}, active, silenced, inhibited, silencedAndInhibited, duplicateSilenced, resolvedSilenced)
	groupOne.marker.SetSilenced(active.Fingerprint(), nil)
	groupOne.marker.SetSilenced(silenced.Fingerprint(), []string{"silence-1"})
	groupOne.marker.SetInhibited(inhibited.Fingerprint(), []string{"source-1"})
	groupOne.marker.SetSilenced(silencedAndInhibited.Fingerprint(), []string{"silence-2"})
	groupOne.marker.SetInhibited(silencedAndInhibited.Fingerprint(), []string{"source-2"})
	groupOne.marker.SetSilenced(duplicateSilenced.Fingerprint(), []string{"silence-3"})
	groupOne.marker.SetSilenced(resolvedSilenced.Fingerprint(), []string{"silence-4"})

	groupTwo := testMetricAggrGroup(t, routeTwo, model.LabelSet{"group": "two"}, duplicateSilenced)
	groupTwo.marker.SetSilenced(duplicateSilenced.Fingerprint(), []string{"silence-5"})

	dispatcher.routeGroupsSlice[routeOne.Idx].groups.Store(groupOne.fingerprint(), groupOne)
	dispatcher.routeGroupsSlice[routeTwo.Idx].groups.Store(groupTwo.fingerprint(), groupTwo)

	require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
# HELP alertmanager_alerts_suppressed How many alerts are currently suppressed by reason. Alerts with multiple suppression reasons are counted once for each reason.
# TYPE alertmanager_alerts_suppressed gauge
alertmanager_alerts_suppressed{reason="inhibition"} 2
alertmanager_alerts_suppressed{reason="silence"} 3
`), "alertmanager_alerts_suppressed"))
}

func TestAlertStateCollector_SuppressedMetricsWithGroupKey(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewDispatcherMetrics(false, reg, groupKeyMetricsFlagger{})
	dispatcher := newRunningMetricsDispatcher(metrics, 2)

	routeOne := testMetricRoute(0)
	routeTwo := testMetricRoute(1)

	silenced := testMetricAlert("silenced", time.Now().Add(time.Hour))
	silencedAndInhibited := testMetricAlert("silenced-and-inhibited", time.Now().Add(time.Hour))

	groupOne := testMetricAggrGroup(t, routeOne, model.LabelSet{"group": "one"}, silenced)
	groupOne.marker.SetSilenced(silenced.Fingerprint(), []string{"silence-1"})

	groupTwo := testMetricAggrGroup(t, routeTwo, model.LabelSet{"group": "two"}, silencedAndInhibited)
	groupTwo.marker.SetSilenced(silencedAndInhibited.Fingerprint(), []string{"silence-2"})
	groupTwo.marker.SetInhibited(silencedAndInhibited.Fingerprint(), []string{"source-1"})

	dispatcher.routeGroupsSlice[routeOne.Idx].groups.Store(groupOne.fingerprint(), groupOne)
	dispatcher.routeGroupsSlice[routeTwo.Idx].groups.Store(groupTwo.fingerprint(), groupTwo)

	expected := fmt.Sprintf(`
# HELP alertmanager_alerts_suppressed How many alerts are currently suppressed by reason. Alerts with multiple suppression reasons are counted once for each reason.
# TYPE alertmanager_alerts_suppressed gauge
alertmanager_alerts_suppressed{group_key=%q,reason="inhibition"} 0
alertmanager_alerts_suppressed{group_key=%q,reason="silence"} 1
alertmanager_alerts_suppressed{group_key=%q,reason="inhibition"} 1
alertmanager_alerts_suppressed{group_key=%q,reason="silence"} 1
`, groupOne.GroupKey(), groupOne.GroupKey(), groupTwo.GroupKey(), groupTwo.GroupKey())

	require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(expected), "alertmanager_alerts_suppressed"))
}

func newRunningMetricsDispatcher(metrics *DispatcherMetrics, routes int) *Dispatcher {
	dispatcher := &Dispatcher{
		routeGroupsSlice: make([]routeAggrGroups, routes),
	}
	dispatcher.state.Store(DispatcherStateRunning)
	metrics.alertsCollector.dispatcher.Store(dispatcher)
	return dispatcher
}

func testMetricRoute(idx int) *Route {
	return &Route{
		RouteOpts: RouteOpts{
			Receiver:       "test",
			GroupBy:        map[model.LabelName]struct{}{"group": {}},
			GroupWait:      time.Hour,
			GroupInterval:  time.Hour,
			RepeatInterval: time.Hour,
		},
		Idx: idx,
	}
}

func testMetricAggrGroup(t *testing.T, route *Route, labels model.LabelSet, alerts ...*alert.Alert) *aggrGroup {
	t.Helper()

	group := newAggrGroup(context.Background(), labels, route, func(d time.Duration) time.Duration { return d }, eventrecorder.NopRecorder(), promslog.NewNopLogger())
	for _, alert := range alerts {
		require.True(t, group.insert(context.Background(), alert))
	}
	return group
}

func testMetricAlert(name string, endsAt time.Time) *alert.Alert {
	now := time.Now()
	return &alert.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"alertname": model.LabelValue(name)},
			Annotations:  model.LabelSet{"summary": "test"},
			StartsAt:     now.Add(-time.Hour),
			EndsAt:       endsAt,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: now,
	}
}
