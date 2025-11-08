// Copyright 2024 The Prometheus Authors
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

package silence

import (
	"bytes"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coder/quartz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

// BenchmarkMutes benchmarks the Mutes method for the Muter interface for
// different numbers of silences, where all silences match the alert.
func BenchmarkMutes(b *testing.B) {
	b.Run("1 silence mutes alert", func(b *testing.B) {
		benchmarkMutes(b, 1)
	})
	b.Run("10 silences mute alert", func(b *testing.B) {
		benchmarkMutes(b, 10)
	})
	b.Run("100 silences mute alert", func(b *testing.B) {
		benchmarkMutes(b, 100)
	})
	b.Run("1000 silences mute alert", func(b *testing.B) {
		benchmarkMutes(b, 1000)
	})
	b.Run("10000 silences mute alert", func(b *testing.B) {
		benchmarkMutes(b, 10000)
	})
}

func benchmarkMutes(b *testing.B, n int) {
	silences, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b)
	silences.clock = clock
	now := clock.Now()

	var silenceIDs []string
	for i := 0; i < n; i++ {
		s := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{{
				Type:    silencepb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: now,
			EndsAt:   now.Add(time.Minute),
		}
		require.NoError(b, silences.Set(s))
		require.NoError(b, err)
		silenceIDs = append(silenceIDs, s.Id)
	}
	require.Len(b, silenceIDs, n)

	m := types.NewMarker(prometheus.NewRegistry())
	s := NewSilencer(silences, m, promslog.NewNopLogger())

	for b.Loop() {
		s.Mutes(model.LabelSet{"foo": "bar"})
	}
	b.StopTimer()

	// The alert should be marked as silenced for each silence.
	activeIDs, pendingIDs, _, silenced := m.Silenced(model.LabelSet{"foo": "bar"}.Fingerprint())
	require.True(b, silenced)
	require.Empty(b, pendingIDs)
	require.Len(b, activeIDs, n)
}

// BenchmarkMutesIncremental tests the incremental query optimization when a small
// number of silences are added to a large existing set. This measures the real-world
// scenario that the QSince optimization is designed for.
func BenchmarkMutesIncremental(b *testing.B) {
	cases := []struct {
		name     string
		baseSize int
	}{
		{"1000 base silences", 1000},
		{"3000 base silences", 3000},
		{"7000 base silences", 7000},
		{"10000 base silences", 10000},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			silences, err := New(Options{Metrics: prometheus.NewRegistry()})
			require.NoError(b, err)

			clock := quartz.NewMock(b)
			silences.clock = clock
			now := clock.Now()

			// Create base set of silences - most don't match, some do
			// This simulates a realistic production scenario
			// Intersperse matching silences throughout the base set
			for i := 0; i < tc.baseSize; i++ {
				var s *silencepb.Silence
				if i%2000 == 0 && i > 0 {
					// Sprinkle 1 silence matching every 2000
					s = &silencepb.Silence{
						Matchers: []*silencepb.Matcher{
							{
								Type:    silencepb.Matcher_EQUAL,
								Name:    "service",
								Pattern: "test",
							},
							{
								Type:    silencepb.Matcher_EQUAL,
								Name:    "instance",
								Pattern: "instance1",
							},
						},
						StartsAt: now,
						EndsAt:   now.Add(time.Hour),
					}
				} else {
					s = &silencepb.Silence{
						Matchers: []*silencepb.Matcher{{
							Type:    silencepb.Matcher_EQUAL,
							Name:    "job",
							Pattern: "job" + strconv.Itoa(i),
						}},
						StartsAt: now,
						EndsAt:   now.Add(time.Hour),
					}
				}
				require.NoError(b, silences.Set(s))
			}

			marker := types.NewMarker(prometheus.NewRegistry())
			silencer := NewSilencer(silences, marker, promslog.NewNopLogger())

			// Warm up: Establish marker state (markerVersion = current version)
			// This simulates a system that has been running for a while
			lset := model.LabelSet{"service": "test", "instance": "instance1"}
			silencer.Mutes(lset)

			// Benchmark: Measure Mutes() performance with incremental additions
			// Every other iteration adds 1 new silence, all iterations call Mutes()
			// This simulates realistic traffic with a mix of incremental and cached queries
			// With QSince optimization, this should only scan new silences when added
			b.ResetTimer()
			iteration := 0
			for b.Loop() {
				// Don't measure the Set() time, only Mutes()
				b.StopTimer()

				// Add one new silence every other iteration to simulate realistic traffic
				// where Mutes() is sometimes called without new silences
				if iteration%2 == 0 {
					var s *silencepb.Silence
					if iteration%20 == 0 && iteration > 0 {
						// Only 1 in 20 silences matches the labelset
						s = &silencepb.Silence{
							Matchers: []*silencepb.Matcher{
								{
									Type:    silencepb.Matcher_EQUAL,
									Name:    "service",
									Pattern: "test",
								},
								{
									Type:    silencepb.Matcher_EQUAL,
									Name:    "instance",
									Pattern: "instance1",
								},
							},
							StartsAt: now,
							EndsAt:   now.Add(time.Hour),
						}
					} else {
						// Most don't match
						s = &silencepb.Silence{
							Matchers: []*silencepb.Matcher{{
								Type:    silencepb.Matcher_EQUAL,
								Name:    "instance",
								Pattern: "host" + strconv.Itoa(iteration),
							}},
							StartsAt: now,
							EndsAt:   now.Add(time.Hour),
						}
					}
					require.NoError(b, silences.Set(s))
				}

				b.StartTimer()
				// Now query - should use incremental path or cached paths
				silencer.Mutes(lset)
				iteration++
			}
		})
	}
}

// BenchmarkQuery benchmarks the Query method for the Silences struct
// for different numbers of silences. Not all silences match the query
// to prevent compiler and runtime optimizations from affecting the benchmarks.
func BenchmarkQuery(b *testing.B) {
	b.Run("100 silences", func(b *testing.B) {
		benchmarkQuery(b, 100)
	})
	b.Run("1000 silences", func(b *testing.B) {
		benchmarkQuery(b, 1000)
	})
	b.Run("10000 silences", func(b *testing.B) {
		benchmarkQuery(b, 10000)
	})
}

func benchmarkQuery(b *testing.B, numSilences int) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create silences using Set() to properly populate indices
	for i := 0; i < numSilences; i++ {
		id := strconv.Itoa(i)
		// Include an offset to avoid optimizations.
		patA := "A{4}|" + id
		patB := id // Does not match.
		if i%10 == 0 {
			// Every 10th time, have an actually matching pattern.
			patB = "B(B|C)B.|" + id
		}

		sil := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{
				{Type: silencepb.Matcher_REGEXP, Name: "aaaa", Pattern: patA},
				{Type: silencepb.Matcher_REGEXP, Name: "bbbb", Pattern: patB},
			},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}
		require.NoError(b, s.Set(sil))
	}

	// Run things once to populate the matcherCache.
	sils, _, err := s.Query(
		QState(types.SilenceStateActive),
		QMatches(lset),
	)
	require.NoError(b, err)
	require.Len(b, sils, numSilences/10)

	for b.Loop() {
		sils, _, err := s.Query(
			QState(types.SilenceStateActive),
			QMatches(lset),
		)
		require.NoError(b, err)
		require.Len(b, sils, numSilences/10)
	}
}

// BenchmarkQueryParallel benchmarks concurrent queries to demonstrate
// the performance improvement from using read locks (RLock) instead of
// write locks (Lock). With the pre-compiled matcher cache, multiple
// queries can now execute in parallel.
func BenchmarkQueryParallel(b *testing.B) {
	b.Run("100 silences, 1 goroutine", func(b *testing.B) {
		benchmarkQueryParallel(b, 100, 1)
	})
	b.Run("100 silences, 2 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 100, 2)
	})
	b.Run("100 silences, 4 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 100, 4)
	})
	b.Run("100 silences, 8 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 100, 8)
	})
	b.Run("1000 silences, 1 goroutine", func(b *testing.B) {
		benchmarkQueryParallel(b, 1000, 1)
	})
	b.Run("1000 silences, 2 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 1000, 2)
	})
	b.Run("1000 silences, 4 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 1000, 4)
	})
	b.Run("1000 silences, 8 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 1000, 8)
	})
	b.Run("10000 silences, 1 goroutine", func(b *testing.B) {
		benchmarkQueryParallel(b, 10000, 1)
	})
	b.Run("10000 silences, 2 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 10000, 2)
	})
	b.Run("10000 silences, 4 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 10000, 4)
	})
	b.Run("10000 silences, 8 goroutines", func(b *testing.B) {
		benchmarkQueryParallel(b, 10000, 8)
	})
}

func benchmarkQueryParallel(b *testing.B, numSilences, numGoroutines int) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create silences with pre-compiled matchers
	for i := 0; i < numSilences; i++ {
		id := strconv.Itoa(i)
		patA := "A{4}|" + id
		patB := id
		if i%10 == 0 {
			patB = "B(B|C)B.|" + id
		}

		sil := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{
				{Type: silencepb.Matcher_REGEXP, Name: "aaaa", Pattern: patA},
				{Type: silencepb.Matcher_REGEXP, Name: "bbbb", Pattern: patB},
			},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}
		require.NoError(b, s.Set(sil))
	}

	// Verify initial query works
	sils, _, err := s.Query(
		QState(types.SilenceStateActive),
		QMatches(lset),
	)
	require.NoError(b, err)
	require.Len(b, sils, numSilences/10)

	b.ResetTimer()

	// Run queries in parallel across multiple goroutines
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sils, _, err := s.Query(
				QState(types.SilenceStateActive),
				QMatches(lset),
			)
			if err != nil {
				b.Error(err)
			}
			if len(sils) != numSilences/10 {
				b.Errorf("expected %d silences, got %d", numSilences/10, len(sils))
			}
		}
	})
}

// BenchmarkQueryWithConcurrentAdds benchmarks the behavior when queries
// are running concurrently with silence additions. This demonstrates how
// the system handles read-heavy workloads with occasional writes.
func BenchmarkQueryWithConcurrentAdds(b *testing.B) {
	b.Run("1000 initial silences, 10% add rate", func(b *testing.B) {
		benchmarkQueryWithConcurrentAdds(b, 1000, 0.1)
	})
	b.Run("1000 initial silences, 1% add rate", func(b *testing.B) {
		benchmarkQueryWithConcurrentAdds(b, 1000, 0.01)
	})
	b.Run("1000 initial silences, 0.1% add rate", func(b *testing.B) {
		benchmarkQueryWithConcurrentAdds(b, 1000, 0.001)
	})
	b.Run("10000 initial silences, 1% add rate", func(b *testing.B) {
		benchmarkQueryWithConcurrentAdds(b, 10000, 0.01)
	})
	b.Run("10000 initial silences, 0.1% add rate", func(b *testing.B) {
		benchmarkQueryWithConcurrentAdds(b, 10000, 0.001)
	})
}

func benchmarkQueryWithConcurrentAdds(b *testing.B, initialSilences int, addRatio float64) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create initial silences
	for i := 0; i < initialSilences; i++ {
		id := strconv.Itoa(i)
		patA := "A{4}|" + id
		patB := id
		if i%10 == 0 {
			patB = "B(B|C)B.|" + id
		}

		sil := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{
				{Type: silencepb.Matcher_REGEXP, Name: "aaaa", Pattern: patA},
				{Type: silencepb.Matcher_REGEXP, Name: "bbbb", Pattern: patB},
			},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}
		require.NoError(b, s.Set(sil))
	}

	var addCounter int
	var mu sync.Mutex

	b.ResetTimer()

	// Run parallel operations
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Determine if this iteration should add a silence
			mu.Lock()
			shouldAdd := float64(addCounter) < float64(b.N)*addRatio
			if shouldAdd {
				addCounter++
			}
			localCounter := addCounter + initialSilences
			mu.Unlock()

			if shouldAdd {
				// Add a new silence
				id := strconv.Itoa(localCounter)
				patA := "A{4}|" + id
				patB := "B(B|C)B.|" + id

				sil := &silencepb.Silence{
					Matchers: []*silencepb.Matcher{
						{Type: silencepb.Matcher_REGEXP, Name: "aaaa", Pattern: patA},
						{Type: silencepb.Matcher_REGEXP, Name: "bbbb", Pattern: patB},
					},
					StartsAt:  now.Add(-time.Minute),
					EndsAt:    now.Add(time.Hour),
					UpdatedAt: now.Add(-time.Hour),
				}
				if err := s.Set(sil); err != nil {
					b.Error(err)
				}
			} else {
				// Query silences (the common operation)
				_, _, err := s.Query(
					QState(types.SilenceStateActive),
					QMatches(lset),
				)
				if err != nil {
					b.Error(err)
				}
			}
		}
	})
}

// BenchmarkMutesParallel benchmarks concurrent Mutes calls to demonstrate
// the performance improvement from parallel query execution.
func BenchmarkMutesParallel(b *testing.B) {
	b.Run("100 silences, 4 goroutines", func(b *testing.B) {
		benchmarkMutesParallel(b, 100, 4)
	})
	b.Run("1000 silences, 4 goroutines", func(b *testing.B) {
		benchmarkMutesParallel(b, 1000, 4)
	})
	b.Run("10000 silences, 4 goroutines", func(b *testing.B) {
		benchmarkMutesParallel(b, 10000, 4)
	})
	b.Run("10000 silences, 8 goroutines", func(b *testing.B) {
		benchmarkMutesParallel(b, 10000, 8)
	})
}

func benchmarkMutesParallel(b *testing.B, numSilences, numGoroutines int) {
	silences, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b)
	silences.clock = clock
	now := clock.Now()

	// Create silences that will match the alert
	for i := 0; i < numSilences; i++ {
		s := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{{
				Type:    silencepb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: now,
			EndsAt:   now.Add(time.Minute),
		}
		require.NoError(b, silences.Set(s))
	}

	m := types.NewMarker(prometheus.NewRegistry())
	silencer := NewSilencer(silences, m, promslog.NewNopLogger())

	b.ResetTimer()

	// Run Mutes in parallel
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			silencer.Mutes(model.LabelSet{"foo": "bar"})
		}
	})
}

// BenchmarkGC benchmarks the garbage collection performance for different
// numbers of silences and different ratios of expired silences.
func BenchmarkGC(b *testing.B) {
	b.Run("100 silences, 0% expired", func(b *testing.B) {
		benchmarkGC(b, 100, 0.0)
	})
	b.Run("100 silences, 10% expired", func(b *testing.B) {
		benchmarkGC(b, 100, 0.1)
	})
	b.Run("100 silences, 50% expired", func(b *testing.B) {
		benchmarkGC(b, 100, 0.5)
	})
	b.Run("100 silences, 90% expired", func(b *testing.B) {
		benchmarkGC(b, 100, 0.9)
	})
	b.Run("1000 silences, 0% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.0)
	})
	b.Run("1000 silences, 10% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.1)
	})
	b.Run("1000 silences, 50% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.5)
	})
	b.Run("1000 silences, 90% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.9)
	})
	b.Run("10000 silences, 0% expired", func(b *testing.B) {
		benchmarkGC(b, 10000, 0.0)
	})
	b.Run("10000 silences, 10% expired", func(b *testing.B) {
		benchmarkGC(b, 10000, 0.1)
	})
	b.Run("10000 silences, 50% expired", func(b *testing.B) {
		benchmarkGC(b, 10000, 0.5)
	})
	b.Run("10000 silences, 90% expired", func(b *testing.B) {
		benchmarkGC(b, 10000, 0.9)
	})
}

func benchmarkGC(b *testing.B, numSilences int, expiredRatio float64) {
	b.ReportAllocs()

	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Hour,
	})
	require.NoError(b, err)

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	s.clock = clock
	now := clock.Now()

	// Create silences - intersperse expired and active with some clustering
	// This simulates realistic production where silences expire at different times
	// Divide expired silences into 3 groups that expire at different times
	numExpired := int(float64(numSilences) * expiredRatio)
	numExpiredPerGroup := numExpired / 3
	expiredGroup1 := 0
	expiredGroup2 := 0
	expiredGroup3 := 0

	// Vary cluster size for more realistic distribution (1, 3, or 5 silences per cluster)
	clusterSizes := []int{1, 3, 5}
	currentClusterIdx := 0
	clusterRemaining := clusterSizes[0]

	for i := range numSilences {
		var startsAt, endsAt time.Time

		// When cluster is exhausted, pick next cluster size
		if clusterRemaining == 0 {
			currentClusterIdx = (currentClusterIdx + 1) % len(clusterSizes)
			clusterRemaining = clusterSizes[currentClusterIdx]
		}
		clusterSize := clusterSizes[currentClusterIdx]

		// Determine which group this silence belongs to
		totalExpiredCreated := expiredGroup1 + expiredGroup2 + expiredGroup3
		shouldBeExpired := false
		expiredGroup := 0

		if totalExpiredCreated < numExpired {
			// Use clustering: within a cluster, all have same state
			clusterIndex := i / clusterSize
			// Alternate clusters, but respect the total expired ratio
			if clusterIndex%3 == 0 || (float64(totalExpiredCreated)/float64(i+1)) < expiredRatio {
				shouldBeExpired = true
				// Distribute across 3 groups
				if expiredGroup1 < numExpiredPerGroup {
					expiredGroup = 1
				} else if expiredGroup2 < numExpiredPerGroup {
					expiredGroup = 2
				} else {
					expiredGroup = 3
				}
			}
		}

		if shouldBeExpired && totalExpiredCreated < numExpired {
			startsAt = now.Add(-time.Hour)
			// Group 1 expires at 30min, Group 2 at 45min, Group 3 at 60min
			switch expiredGroup {
			case 1:
				endsAt = now.Add(30 * time.Minute)
				expiredGroup1++
			case 2:
				endsAt = now.Add(45 * time.Minute)
				expiredGroup2++
			case 3:
				endsAt = now.Add(60 * time.Minute)
				expiredGroup3++
			}
		} else {
			// Active silences (will survive all 3 GC runs)
			startsAt = now.Add(-time.Hour)
			endsAt = now.Add(3 * time.Hour)
		}

		clusterRemaining--

		sil := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{{
				Type:    silencepb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: startsAt,
			EndsAt:   endsAt,
		}
		require.NoError(b, s.Set(sil))
	}

	// Snapshot the initial state at current time
	var snapshot bytes.Buffer
	_, err = s.Snapshot(&snapshot)
	require.NoError(b, err)
	snapshotBytes := snapshot.Bytes()

	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()

		// Create fresh clock at the original time for this iteration
		iterClock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)

		// Restore from snapshot
		snapshotReader := bytes.NewReader(snapshotBytes)
		s, err = New(Options{
			Metrics:        prometheus.NewRegistry(),
			Retention:      1 * time.Hour,
			SnapshotReader: snapshotReader,
		})
		require.NoError(b, err)
		s.clock = iterClock

		b.StartTimer()

		// Run 3 GC cycles at different times to collect the 3 expiry groups
		// Group 1: EndsAt = now+30min → ExpiresAt = now+90min
		iterClock.Advance(91 * time.Minute)
		numGC1, err := s.GC()
		require.NoError(b, err)

		// Group 2: EndsAt = now+45min → ExpiresAt = now+105min
		iterClock.Advance(15 * time.Minute)
		numGC2, err := s.GC()
		require.NoError(b, err)

		// Group 3: EndsAt = now+60min → ExpiresAt = now+120min
		iterClock.Advance(15 * time.Minute)
		numGC3, err := s.GC()
		require.NoError(b, err)

		b.StopTimer()

		require.Equal(b, expiredGroup1, numGC1, "GC1 should remove %d expired silences, but removed %d", expiredGroup1, numGC1)
		require.Equal(b, expiredGroup2, numGC2, "GC2 should remove %d expired silences, but removed %d", expiredGroup2, numGC2)
		require.Equal(b, expiredGroup3, numGC3, "GC3 should remove %d expired silences, but removed %d", expiredGroup3, numGC3)

		// Verify total GC removed the expected number of silences
		totalRemoved := numGC1 + numGC2 + numGC3
		require.Equal(b, numExpired, totalRemoved, "GC should remove %d expired silences total, but removed %d", numExpired, totalRemoved)

		require.Len(b, s.st, numSilences-totalRemoved, "After GC, expected %d silences to remain in state", numSilences-totalRemoved)
		require.Len(b, s.mi, numSilences-totalRemoved, "After GC, expected %d silences to remain in matcher index", numSilences-totalRemoved)

		b.StartTimer()
	}
}
