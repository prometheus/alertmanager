// Copyright 2025 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

// buildDeepRouteTree creates a multi-level hierarchical route tree:
// - numTeams routes at top level
// - Each team has numClusters cluster sub-routes
// - Each cluster has numPriorities priority sub-routes
// Total: numTeams * numClusters * numPriorities leaf routes with 3 levels of depth.
func buildDeepRouteTree(numTeams, numClusters, numPriorities int) *Route {
	groupWait := model.Duration(30 * time.Second)
	groupInterval := model.Duration(5 * time.Minute)
	repeatInterval := model.Duration(4 * time.Hour)

	root := &config.Route{
		Receiver:       "default",
		GroupBy:        []model.LabelName{"alertname"},
		GroupWait:      &groupWait,
		GroupInterval:  &groupInterval,
		RepeatInterval: &repeatInterval,
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
func newBenchAlert(labels model.LabelSet) *types.Alert {
	now := time.Now()
	return &types.Alert{
		Alert: model.Alert{
			Labels:       labels,
			Annotations:  model.LabelSet{"description": "benchmark alert"},
			StartsAt:     now,
			EndsAt:       now.Add(time.Hour),
			GeneratorURL: "http://localhost",
		},
		UpdatedAt: now,
	}
}

// makeBenchAlertBatch creates a batch of alerts distributed across route tree dimensions:
//   - offset is added to the index to create unique alerts across multiple batches,
//     exercising the whole route tree.
//   - numTeams, numClusters, numPriorities define the route tree structure.
func makeBenchAlertBatch(size, offset, numTeams, numClusters, numPriorities int) []*types.Alert {
	batch := make([]*types.Alert, size)
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
	marker := types.NewMarker(reg)

	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, 0, nil, logger, reg, nil)
	require.NoError(b, err)
	b.Cleanup(func() { alerts.Close() })

	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	timeout := func(d time.Duration) time.Duration { return time.Duration(0) }
	metrics := NewDispatcherMetrics(false, reg)

	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, 30*time.Second, nil, logger, metrics)

	return dispatcher, alerts, recorder
}

// populateGroups pre-populates the dispatcher with aggregation groups.
// It puts a total of numGroups groups, each with alertsPerGroup alerts, spread across
// numTeams, numClusters, numPriorities route dimensions.
func populateGroups(b *testing.B, d *Dispatcher, alerts *mem.Alerts, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities, expectedMinGroups int) {
	ctx := context.Background()

	for i := range numGroups {
		groupAlerts := make([]*types.Alert, 0, alertsPerGroup)
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
			func(*types.Alert, time.Time) bool { return true },
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
	b.Run("120 routes, 10000 groups", func(b *testing.B) {
		benchmarkGroups(b, 10000, 3, 20, 4, 5)
	})
}

func benchmarkGroups(b *testing.B, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities int) {
	route := buildDeepRouteTree(numTeams, numClusters, numPriorities)

	b.ReportAllocs()

	dispatcher, alerts, _ := setupDispatcher(b, route)
	go dispatcher.Run(time.Now())
	defer dispatcher.Stop()

	// Pre-populate with stable groups (uses existing helper)
	populateGroups(b, dispatcher, alerts, numGroups, alertsPerGroup, numTeams, numClusters, numPriorities, numGroups)

	ctx := context.Background()

	routeFilter := func(*Route) bool { return true }
	alertFilter := func(*types.Alert, time.Time) bool { return true }

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
	route := buildDeepRouteTree(25, 4, 5)

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
						func(*types.Alert, time.Time) bool { return true })
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
