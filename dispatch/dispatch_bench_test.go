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
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/template"
)

// buildDeepRouteTree creates a multi-level hierarchical route tree:
// - numTeams routes at top level
// - Each team has numClusters cluster sub-routes
// - Each cluster has numPriorities priority sub-routes
// - The labels are attached to the root route, and thus inherited by every leaf
// Total: numTeams * numClusters * numPriorities leaf routes with 3 levels of depth.
func buildDeepRouteTree(numTeams, numClusters, numPriorities int, labels model.LabelSet) *Route {
	groupWait := model.Duration(30 * time.Second)
	groupInterval := model.Duration(5 * time.Minute)
	repeatInterval := model.Duration(4 * time.Hour)

	root := &config.Route{
		Receiver:       "default",
		GroupBy:        []model.LabelName{"alertname"},
		GroupWait:      &groupWait,
		GroupInterval:  &groupInterval,
		RepeatInterval: &repeatInterval,
		Labels:         labels,
	}

	// Create team routes, each with cluster sub-routes, each with priority sub-routes
	root.Routes = make([]*config.Route, 0, numTeams)
	for i := range numTeams {
		teamRoute := &config.Route{
			Receiver:       fmt.Sprintf("team-%d-default", i),
			Match:          map[string]string{"team": fmt.Sprintf("team-%d", i)},
			GroupBy:        []model.LabelName{"alertname"},
			GroupWait:      &groupWait,
			GroupInterval:  &groupInterval,
			RepeatInterval: &repeatInterval,
		}

		// Add cluster sub-routes
		teamRoute.Routes = make([]*config.Route, 0, numClusters)
		for j := range numClusters {
			clusterRoute := &config.Route{
				Receiver:       fmt.Sprintf("team-%d-cluster-%d-default", i, j),
				Match:          map[string]string{"cluster": fmt.Sprintf("cluster-%d", j)},
				GroupBy:        []model.LabelName{"alertname"},
				GroupWait:      &groupWait,
				GroupInterval:  &groupInterval,
				RepeatInterval: &repeatInterval,
			}

			// Add priority sub-routes
			clusterRoute.Routes = make([]*config.Route, 0, numPriorities)
			for k := range numPriorities {
				sevRoute := &config.Route{
					Receiver:       fmt.Sprintf("team-%d-cluster-%d-p%d", i, j, k),
					Match:          map[string]string{"priority": fmt.Sprintf("p%d", k)},
					GroupBy:        []model.LabelName{"alertname"},
					GroupWait:      &groupWait,
					GroupInterval:  &groupInterval,
					RepeatInterval: &repeatInterval,
				}
				clusterRoute.Routes = append(clusterRoute.Routes, sevRoute)
			}

			teamRoute.Routes = append(teamRoute.Routes, clusterRoute)
		}

		root.Routes = append(root.Routes, teamRoute)
	}

	return NewRoute(root, nil)
}

// newBenchAlert creates a simple alert with given labels for benchmarking.
func newBenchAlert(labels model.LabelSet) *alert.Alert {
	now := time.Now()
	return alert.New(model.Alert{
		Labels:       labels,
		Annotations:  model.LabelSet{"description": "benchmark alert"},
		StartsAt:     now,
		EndsAt:       now.Add(time.Hour),
		GeneratorURL: "http://localhost",
	}, now, false)
}

// makeBenchAlertBatch creates a batch of alerts distributed across route tree dimensions:
//   - offset is added to the index to create unique alerts across multiple batches,
//     exercising the whole route tree.
//   - numTeams, numClusters, numPriorities define the route tree structure.
func makeBenchAlertBatch(size, offset, numTeams, numClusters, numPriorities int) []*alert.Alert {
	batch := make([]*alert.Alert, size)
	for i := range size {
		idx := offset + i
		labels := model.LabelSet{
			"alertname": model.LabelValue(fmt.Sprintf("alert-%d", idx)),
			"instance":  model.LabelValue(fmt.Sprintf("instance-%d", idx)),
		}

		// Distribute alerts across teams, clusters, priorities using simple modulo
		// This ensures each batch hits all dimensions evenly
		if numTeams > 0 {
			labels["team"] = model.LabelValue(fmt.Sprintf("team-%d", idx%numTeams))
			labels["cluster"] = model.LabelValue(fmt.Sprintf("cluster-%d", idx%numClusters))
			labels["priority"] = model.LabelValue(fmt.Sprintf("p%d", idx%numPriorities))
		}

		batch[i] = newBenchAlert(labels)
	}
	return batch
}

// setupDispatcher creates a dispatcher with the given route for benchmarking.
func setupDispatcher(b *testing.B, route *Route) (*Dispatcher, *mem.Alerts, *recordStage) {
	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()

	alerts, err := mem.NewAlerts(context.Background(), time.Hour, 0, nil, logger, eventrecorder.NopRecorder(), reg, nil)
	require.NoError(b, err)
	b.Cleanup(func() { alerts.Close() })

	tmpl, err := template.FromGlobs([]string{})
	require.NoError(b, err)
	tmpl.ExternalURL = &url.URL{Scheme: "http", Host: "example.com"}

	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*alert.Alert)}
	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	metrics := NewDispatcherMetrics(false, reg, nil)

	dispatcher := NewDispatcher(alerts, route, recorder, nil, timeout, 30*time.Second, nil, logger, eventrecorder.NopRecorder(), metrics, tmpl)

	return dispatcher, alerts, recorder
}

// waitProcessed blocks until the dispatcher has routed at least want alerts in
// total, so that a following Groups call observes every insert.
func waitProcessed(b *testing.B, d *Dispatcher, want uint64) {
	b.Helper()
	var m dto.Metric
	require.Eventually(b, func() bool {
		require.NoError(b, d.metrics.processingDuration.Write(&m))
		return m.GetSummary().GetSampleCount() >= want
	}, time.Minute, time.Millisecond)
}

// populateGroups pre-populates the dispatcher with aggregation groups.
// It puts a total of numGroups groups, each with alertsPerGroup alerts, spread across
// numTeams, numClusters, numPriorities route dimensions.
func populateGroups(b *testing.B, d *Dispatcher, alerts *mem.Alerts, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities, expectedMinGroups int) {
	ctx := context.Background()

	for i := range numGroups {
		groupAlerts := make([]*alert.Alert, 0, alertsPerGroup)
		for j := range alertsPerGroup {
			labels := model.LabelSet{
				"alertname": model.LabelValue(fmt.Sprintf("alert-%d", i)),
				"instance":  model.LabelValue(fmt.Sprintf("instance-%d", j)),
			}
			// Distribute alerts across teams, clusters, priorities (for deep route tree)
			if numTeams > 0 {
				labels["team"] = model.LabelValue(fmt.Sprintf("team-%d", i%numTeams))
				labels["cluster"] = model.LabelValue(fmt.Sprintf("cluster-%d", (i/numTeams)%numClusters))
				labels["priority"] = model.LabelValue(fmt.Sprintf("p%d", (i/(numTeams*numClusters))%numPriorities))
			}
			groupAlerts = append(groupAlerts, newBenchAlert(labels))
		}
		require.NoError(b, alerts.Put(ctx, groupAlerts...))
	}

	// Wait for dispatcher to create all expected groups
	require.Eventually(b, func() bool {
		groups, _, _ := d.Groups(ctx,
			func(*Route) bool { return true },
			func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true },
		)
		return len(groups) >= expectedMinGroups
	}, 30*time.Second, 10*time.Millisecond, "expected %d groups to be created", expectedMinGroups)
}

// BenchmarkGroups simulates a realistic production scenario:
// - 500 leaf routes in a deep hierarchy (25 teams × 4 clusters × 5 priorities)
// - 5000 stable aggregation groups (average ~10 per leaf route)
// - Measures Groups() API latency (simulates GET /api/v2/alerts/groups)
//
// This benchmark demonstrates the concurrent dispatcher's benefit:
// - main branch: Global lock blocks all Groups() calls during any alert processing
// - concurrent dispatcher: Per-route locks allow Groups() to run mostly lock-free.
func BenchmarkGroups(b *testing.B) {
	b.Run("500 routes, 5000 groups", func(b *testing.B) {
		benchmarkGroups(b, 5000, 3, 25, 4, 5)
	})
	b.Run("400 routes, 10000 groups", func(b *testing.B) {
		benchmarkGroups(b, 10000, 3, 20, 4, 5)
	})
}

func benchmarkGroups(b *testing.B, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities int) {
	route := buildDeepRouteTree(numTeams, numClusters, numPriorities, nil)

	b.ReportAllocs()

	dispatcher, alerts, _ := setupDispatcher(b, route)
	go dispatcher.Run(time.Now())
	defer dispatcher.Stop()

	// Pre-populate with stable groups (uses existing helper)
	populateGroups(b, dispatcher, alerts, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities, numGroups)

	ctx := context.Background()

	routeFilter := func(*Route) bool { return true }
	alertFilter := func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true }

	b.ResetTimer()

	// Measure Groups() API latency
	for b.Loop() {
		groups, _, _ := dispatcher.Groups(ctx, routeFilter, alertFilter)
		if len(groups) != numGroups {
			b.Fatalf("unexpected group count: %d (expected %d)", len(groups), numGroups)
		}
	}

	b.StopTimer()
}

// BenchmarkIngestionUnderGroupsLoad measures alert ingestion latency
// while concurrent Groups() API calls are happening.
//
// This demonstrates the key benefit of the concurrent dispatcher:
// - Main branch: Groups() holds global RLock, blocks ingestion (needs WLock)
// - Concurrent dispatcher: Groups() iterates sync.Maps, minimal blocking
//
// We measure ingestion latency (time for alerts.Put to complete) as we
// increase the number of concurrent Groups() callers from 0 to 100.
func BenchmarkIngestionUnderGroupsLoad(b *testing.B) {
	b.Run("500 routes, 0/s Groups() callers", func(b *testing.B) {
		benchmarkIngestionUnderGroupsLoad(b, 0, 0*time.Millisecond)
	})
	b.Run("500 routes, 1 10/s Groups() callers", func(b *testing.B) {
		benchmarkIngestionUnderGroupsLoad(b, 1, 100*time.Millisecond)
	})
	b.Run("500 routes, 10 10/s Groups() callers", func(b *testing.B) {
		benchmarkIngestionUnderGroupsLoad(b, 10, 100*time.Millisecond)
	})
	b.Run("500 routes, 25 10/s Groups() callers", func(b *testing.B) {
		benchmarkIngestionUnderGroupsLoad(b, 25, 100*time.Millisecond)
	})
	b.Run("500 routes, 25 20/s Groups() callers", func(b *testing.B) {
		benchmarkIngestionUnderGroupsLoad(b, 25, 50*time.Millisecond)
	})
}

func benchmarkIngestionUnderGroupsLoad(b *testing.B, numGroupsCallers int, groupsTick time.Duration) {
	route := buildDeepRouteTree(25, 4, 5, nil)

	b.ReportAllocs()

	dispatcher, alerts, _ := setupDispatcher(b, route)
	go dispatcher.Run(time.Now())
	defer dispatcher.Stop()

	// Pre-populate 5000 stable groups across routes
	populateGroups(b, dispatcher, alerts, 5000, 3, 25, 4, 5, 5000)

	ctx := context.Background()
	stopCh := make(chan struct{})
	defer close(stopCh)

	// Start concurrent Groups() callers (simulating dashboard queries)
	for range numGroupsCallers {
		go func() {
			ticker := time.NewTicker(groupsTick)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					dispatcher.Groups(ctx,
						func(*Route) bool { return true },
						func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true })
				case <-stopCh:
					return
				}
			}
		}()
	}

	// Let Groups() callers stabilize
	time.Sleep(500 * time.Millisecond)

	b.ResetTimer()

	counter := 0
	for b.Loop() {
		batch := makeBenchAlertBatch(100, counter*100, 25, 4, 5)

		// Put alerts into provider
		err := alerts.Put(ctx, batch...)
		if err != nil {
			b.Fatal(err)
		}
		counter++
	}
}

// BenchmarkGroupsRouteLabels measures Groups() (the /api/v2/alerts/groups read
// path, which renders each group's route labels) while every alert held by the
// dispatcher is re-sent between calls with an extended EndsAt, as Prometheus
// does for firing alerts. Per iteration it reports:
//   - ns/resend: putting every alert again and waiting until it is dispatched.
//   - ns/groups-after-resend: the first Groups() call after the re-send.
//   - ns/groups-warm: a second Groups() call with no intervening change.
func BenchmarkGroupsRouteLabels(b *testing.B) {
	b.Run("no route labels", func(b *testing.B) {
		benchmarkGroupsRouteLabels(b, nil)
	})
	b.Run("templated route labels", func(b *testing.B) {
		benchmarkGroupsRouteLabels(b, model.LabelSet{
			"owner":     "ops",
			"alertname": "{{ .GroupLabels.alertname }}",
			"count":     "{{ len .Alerts }}",
		})
	})
}

func benchmarkGroupsRouteLabels(b *testing.B, labels model.LabelSet) {
	const (
		numTeams, numClusters, numPriorities = 10, 2, 5
		numGroups                            = 2000
		alertsPerGroup                       = 3
	)
	route := buildDeepRouteTree(numTeams, numClusters, numPriorities, labels)
	d, alerts, _ := setupDispatcher(b, route)
	go d.Run(time.Now())
	defer d.Stop()

	ctx := context.Background()
	now := time.Now()
	population := make([]*alert.Alert, 0, numGroups*alertsPerGroup)
	for i := range numGroups {
		for j := range alertsPerGroup {
			population = append(population, alert.New(model.Alert{
				Labels: model.LabelSet{
					"alertname": model.LabelValue(fmt.Sprintf("alert-%d", i)),
					"instance":  model.LabelValue(fmt.Sprintf("instance-%d", j)),
					"team":      model.LabelValue(fmt.Sprintf("team-%d", i%numTeams)),
					"cluster":   model.LabelValue(fmt.Sprintf("cluster-%d", (i/numTeams)%numClusters)),
					"priority":  model.LabelValue(fmt.Sprintf("p%d", (i/(numTeams*numClusters))%numPriorities)),
				},
				Annotations:  model.LabelSet{"description": "benchmark alert"},
				StartsAt:     now.Add(-time.Hour),
				EndsAt:       now.Add(time.Hour),
				GeneratorURL: "http://localhost",
			}, now, false))
		}
	}
	require.NoError(b, alerts.Put(ctx, population...))
	processed := uint64(len(population))
	waitProcessed(b, d, processed)

	routeFilter := func(*Route) bool { return true }
	alertFilter := func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true }
	groups, _, _ := d.Groups(ctx, routeFilter, alertFilter)
	require.Len(b, groups, numGroups)

	var resendDur, afterResendDur, warmDur time.Duration
	resent := make([]*alert.Alert, len(population))
	for b.Loop() {
		now := time.Now()
		for i, a := range population {
			cp := *a
			cp.UpdatedAt = now
			cp.EndsAt = now.Add(time.Hour)
			resent[i] = &cp
		}

		t0 := time.Now()
		require.NoError(b, alerts.Put(ctx, resent...))
		processed += uint64(len(resent))
		waitProcessed(b, d, processed)
		t1 := time.Now()
		groups, _, _ = d.Groups(ctx, routeFilter, alertFilter)
		t2 := time.Now()
		if len(groups) != numGroups {
			b.Fatalf("unexpected group count: %d (expected %d)", len(groups), numGroups)
		}
		d.Groups(ctx, routeFilter, alertFilter)
		t3 := time.Now()

		resendDur += t1.Sub(t0)
		afterResendDur += t2.Sub(t1)
		warmDur += t3.Sub(t2)
	}

	n := float64(b.N)
	b.ReportMetric(float64(resendDur)/n, "ns/resend")
	b.ReportMetric(float64(afterResendDur)/n, "ns/groups-after-resend")
	b.ReportMetric(float64(warmDur)/n, "ns/groups-warm")
}
