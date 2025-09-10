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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

// setupBenchmarkDispatcher creates a dispatcher with the specified number of aggregation groups.
func setupBenchmarkDispatcher(totalGroups, emptyGroups int) (*Dispatcher, func()) {
	r := prometheus.NewRegistry()
	marker := types.NewMarker(r)
	logger := promslog.NewNopLogger()

	alerts, err := mem.NewAlerts(context.Background(), marker, time.Hour, nil, logger, nil)
	if err != nil {
		panic(err)
	}

	// Create route with fine-grained grouping to maximize group count
	route := &Route{
		RouteOpts: RouteOpts{
			Receiver:       "default",
			GroupBy:        map[model.LabelName]struct{}{"alertname": {}, "instance": {}, "job": {}},
			GroupWait:      0,
			GroupInterval:  1 * time.Hour, // Long interval to avoid interference
			RepeatInterval: 1 * time.Hour,
		},
	}

	timeout := func(d time.Duration) time.Duration { return d }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*types.Alert)}
	dispatcher := NewDispatcher(alerts, route, recorder, marker, timeout, nil, logger, NewDispatcherMetrics(false, r))

	// Start the dispatcher to initialize context
	go dispatcher.Run()

	// Wait a bit for dispatcher to initialize
	time.Sleep(10 * time.Millisecond)

	// Create aggregation groups by processing alerts
	nonEmptyCount := totalGroups - emptyGroups

	// Create alerts that will generate the desired number of groups
	for i := 0; i < totalGroups; i++ {
		alert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": model.LabelValue(fmt.Sprintf("Alert_%d", i)),
					"instance":  model.LabelValue(fmt.Sprintf("inst_%d", i)),
					"job":       model.LabelValue(fmt.Sprintf("job_%d", i)),
				},
				StartsAt: time.Now().Add(-time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
			UpdatedAt: time.Now(),
		}

		// Put alerts only for non-empty groups
		if i < nonEmptyCount {
			alerts.Put(alert)
		} else {
			// For empty groups, put and then immediately expire the alert
			alerts.Put(alert)
			// Create a resolved version to make group empty
			resolvedAlert := *alert
			resolvedAlert.EndsAt = time.Now().Add(-time.Second)
			alerts.Put(&resolvedAlert)
		}
	}

	// Wait for alerts to be processed and groups to be created
	time.Sleep(100 * time.Millisecond)

	cleanup := func() {
		dispatcher.Stop()
		alerts.Close()
	}

	return dispatcher, cleanup
}

// Benchmark maintenance impact on alert processing.
func BenchmarkDispatch_100k_AggregationGroups_10k_Empty(b *testing.B) {
	benchmarkProcessAlertDuringMaintenance(b, 100_000, 10_000)
}

func BenchmarkDispatch_100k_AggregationGroups_20k_Empty(b *testing.B) {
	benchmarkProcessAlertDuringMaintenance(b, 100_000, 20_000)
}

func BenchmarkDispatch_100k_AggregationGroups_30k_Empty(b *testing.B) {
	benchmarkProcessAlertDuringMaintenance(b, 100_000, 30_000)
}

func BenchmarkDispatch_100k_AggregationGroups_40k_Empty(b *testing.B) {
	benchmarkProcessAlertDuringMaintenance(b, 100_000, 40_000)
}

func BenchmarkDispatch_100k_AggregationGroups_50k_Empty(b *testing.B) {
	benchmarkProcessAlertDuringMaintenance(b, 100_000, 50_000)
}

// Benchmark Groups() impact on alert processing.
func BenchmarkDispatch_20k_AggregationGroups_Groups_Impact(b *testing.B) {
	benchmarkProcessAlertDuringGroups(b, 20_000, 2_000)
}

func BenchmarkDispatch_50k_AggregationGroups_Groups_Impact(b *testing.B) {
	benchmarkProcessAlertDuringGroups(b, 50_000, 5_000)
}

func BenchmarkDispatch_100k_AggregationGroups_Groups_Impact(b *testing.B) {
	benchmarkProcessAlertDuringGroups(b, 100_000, 10_000)
}

func benchmarkProcessAlertDuringMaintenance(b *testing.B, totalGroups, emptyGroups int) {
	dispatcher, cleanup := setupBenchmarkDispatcher(totalGroups-emptyGroups, 0) // Start with non-empty groups only
	defer cleanup()

	// Create test route
	route := &Route{
		RouteOpts: RouteOpts{
			Receiver:       "test",
			GroupBy:        map[model.LabelName]struct{}{"alertname": {}, "instance": {}}, // Use instance for more groups
			GroupWait:      0,
			GroupInterval:  1 * time.Hour,
			RepeatInterval: 1 * time.Hour,
		},
	}

	// Pre-create test alerts for main processing
	alerts := make([]*types.Alert, b.N)
	for i := 0; i < b.N; i++ {
		alerts[i] = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "BenchmarkAlert",
					"instance":  model.LabelValue(fmt.Sprintf("bench_inst_%d", i%100)),
				},
				StartsAt: time.Now().Add(-time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
			UpdatedAt: time.Now(),
		}
	}

	b.ResetTimer()

	// Measure baseline alert processing rate (no maintenance)
	start := time.Now()
	for i := 0; i < b.N; i++ {
		dispatcher.processAlert(alerts[i], route)
	}
	baselineDuration := time.Since(start)

	// Now measure processing rate during continuous maintenance with empty group creation
	var maintenanceTime time.Duration
	var duration time.Duration

	// Run maintenance and empty group generation in background
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Goroutine 1: Maintenance
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		maintenanceStart := time.Now()
		for {
			select {
			case <-ticker.C:
				dispatcher.doMaintenance()
			case <-ctx.Done():
				maintenanceTime = time.Since(maintenanceStart)
				return
			}
		}
	}()

	// Goroutine 2: Continuously create empty groups for maintenance to clean up
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond) // Create empty groups more frequently
		defer ticker.Stop()

		emptyGroupCounter := 0
		for {
			select {
			case <-ticker.C:
				// Create a batch of empty groups for maintenance to clean up
				batchSize := emptyGroups / 5 // Create 1/5th of target empty groups each cycle
				if batchSize < 100 {
					batchSize = 100 // Minimum batch size
				}

				for i := 0; i < batchSize; i++ {
					// Create alert that will form a new group
					emptyAlert := &types.Alert{
						Alert: model.Alert{
							Labels: model.LabelSet{
								"alertname": "EmptyGroupAlert",
								"instance":  model.LabelValue(fmt.Sprintf("empty_%d_%d", emptyGroupCounter, i)),
							},
							StartsAt: time.Now().Add(-time.Minute),
							EndsAt:   time.Now().Add(time.Hour),
						},
						UpdatedAt: time.Now(),
					}

					// Process the alert to create the group
					dispatcher.processAlert(emptyAlert, route)

					// Immediately resolve it to make the group empty
					resolvedAlert := *emptyAlert
					resolvedAlert.EndsAt = time.Now().Add(-time.Second)
					dispatcher.processAlert(&resolvedAlert, route)
				}
				emptyGroupCounter++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Small delay to let empty group generation start
	time.Sleep(50 * time.Millisecond)

	// Measure processing under contention
	start = time.Now()
	for i := 0; i < b.N; i++ {
		dispatcher.processAlert(alerts[i], route)
	}
	duration = time.Since(start)
	cancel()
	wg.Wait()

	baselineRate := float64(b.N) / baselineDuration.Seconds()
	rate := float64(b.N) / duration.Seconds()
	impact := (duration.Seconds() - baselineDuration.Seconds()) / baselineDuration.Seconds() * 100

	// Report metrics
	b.ReportMetric(float64(maintenanceTime.Milliseconds()), "ms/maintenance")
	b.ReportMetric(baselineRate, "baseline_alerts/sec")
	b.ReportMetric(rate, "alerts/sec")
	b.ReportMetric(impact, "maintenance_overhead_%")
}

func benchmarkProcessAlertDuringGroups(b *testing.B, totalGroups, emptyGroups int) {
	dispatcher, cleanup := setupBenchmarkDispatcher(totalGroups-emptyGroups, 0) // Start with non-empty groups only
	defer cleanup()

	// Create test route
	route := &Route{
		RouteOpts: RouteOpts{
			Receiver:       "test",
			GroupBy:        map[model.LabelName]struct{}{"alertname": {}, "instance": {}}, // Use instance for more groups
			GroupWait:      0,
			GroupInterval:  1 * time.Hour,
			RepeatInterval: 1 * time.Hour,
		},
	}

	// Pre-create test alerts for main processing
	alerts := make([]*types.Alert, b.N)
	for i := 0; i < b.N; i++ {
		alerts[i] = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "BenchmarkAlert",
					"instance":  model.LabelValue(fmt.Sprintf("bench_inst_%d", i%100)),
				},
				StartsAt: time.Now().Add(-time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
			UpdatedAt: time.Now(),
		}
	}

	b.ResetTimer()

	// Measure baseline alert processing rate (no Groups() calls)
	start := time.Now()
	for i := 0; i < b.N; i++ {
		dispatcher.processAlert(alerts[i], route)
	}
	baselineDuration := time.Since(start)

	// Now measure processing rate during continuous Groups() calls with empty group creation
	var groupsTime time.Duration
	var groupsCallCount int64
	var duration time.Duration

	// Run Groups() calls and empty group generation in background
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Goroutine 1: Continuous Groups() calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(50 * time.Millisecond) // Call Groups() frequently to measure impact
		defer ticker.Stop()

		groupsStart := time.Now()
		for {
			select {
			case <-ticker.C:
				// Call Groups() with no filters to get all groups (worst case)
				_, _ = dispatcher.Groups(func(*Route) bool { return true }, func(*types.Alert, time.Time) bool { return true })
				groupsCallCount++
			case <-ctx.Done():
				groupsTime = time.Since(groupsStart)
				return
			}
		}
	}()

	// Goroutine 2: Continuously create empty groups for more realistic Groups() load
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		emptyGroupCounter := 0
		for {
			select {
			case <-ticker.C:
				// Create a batch of empty groups for Groups() to process
				batchSize := emptyGroups / 10 // Create 1/10th of target empty groups each cycle
				if batchSize < 50 {
					batchSize = 50 // Minimum batch size
				}

				for i := 0; i < batchSize; i++ {
					// Create alert that will form a new group
					emptyAlert := &types.Alert{
						Alert: model.Alert{
							Labels: model.LabelSet{
								"alertname": "EmptyGroupAlert",
								"instance":  model.LabelValue(fmt.Sprintf("empty_%d_%d", emptyGroupCounter, i)),
							},
							StartsAt: time.Now().Add(-time.Minute),
							EndsAt:   time.Now().Add(time.Hour),
						},
						UpdatedAt: time.Now(),
					}

					// Process the alert to create the group
					dispatcher.processAlert(emptyAlert, route)

					// Immediately resolve it to make the group empty
					resolvedAlert := *emptyAlert
					resolvedAlert.EndsAt = time.Now().Add(-time.Second)
					dispatcher.processAlert(&resolvedAlert, route)
				}
				emptyGroupCounter++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Small delay to let Groups() calls and empty group generation start
	time.Sleep(50 * time.Millisecond)

	// Measure processing under Groups() call contention
	start = time.Now()
	for i := 0; i < b.N; i++ {
		dispatcher.processAlert(alerts[i], route)
	}
	duration = time.Since(start)
	cancel()
	wg.Wait()

	rate := float64(b.N) / duration.Seconds()
	baselineRate := float64(b.N) / baselineDuration.Seconds()
	impact := (duration.Seconds() - baselineDuration.Seconds()) / baselineDuration.Seconds() * 100
	groupsRate := float64(groupsCallCount) / groupsTime.Seconds()

	// Report metrics
	b.ReportMetric(float64(groupsCallCount), "groups_calls_total")
	b.ReportMetric(groupsRate, "groups_calls/sec")
	b.ReportMetric(baselineRate, "baseline_alerts/sec")
	b.ReportMetric(rate, "alerts/sec")
	b.ReportMetric(impact, "groups_overhead_%")
}
