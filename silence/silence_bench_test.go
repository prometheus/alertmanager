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

package silence

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coder/quartz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

// BenchmarkMutes benchmarks the Mutes method for the Muter interface for
// different numbers of silences with varying match ratios.
func BenchmarkMutes(b *testing.B) {
	b.Run("0 total, 0 matching", func(b *testing.B) {
		benchmarkMutes(b, 0, 0)
	})
	b.Run("1 total, 1 matching", func(b *testing.B) {
		benchmarkMutes(b, 1, 1)
	})
	b.Run("100 total, 10 matching", func(b *testing.B) {
		benchmarkMutes(b, 100, 10)
	})
	b.Run("1000 total, 1 matching", func(b *testing.B) {
		benchmarkMutes(b, 1000, 1)
	})
	b.Run("1000 total, 10 matching", func(b *testing.B) {
		benchmarkMutes(b, 1000, 10)
	})
	b.Run("1000 total, 100 matching", func(b *testing.B) {
		benchmarkMutes(b, 1000, 100)
	})
	b.Run("10000 total, 0 matching", func(b *testing.B) {
		benchmarkMutes(b, 10000, 10)
	})
	b.Run("10000 total, 10 matching", func(b *testing.B) {
		benchmarkMutes(b, 10000, 10)
	})
	b.Run("10000 total, 1000 matching", func(b *testing.B) {
		benchmarkMutes(b, 10000, 1000)
	})
}

func benchmarkMutes(b *testing.B, totalSilences, matchingSilences int) {
	require.LessOrEqual(b, matchingSilences, totalSilences)

	silences, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	silences.clock = clock
	now := clock.Now()

	// Calculate interval to intersperse matching silences
	var interval int
	if matchingSilences > 0 {
		interval = totalSilences / matchingSilences
	}

	// Create silences with matching ones interspersed throughout
	matchingCreated := 0
	for i := range totalSilences {
		var s *silencepb.Silence
		// Create matching silences at calculated intervals, but make sure there are always enough
		if matchingCreated < matchingSilences && (i%interval == 0 || i == totalSilences-matchingSilences+matchingCreated) {
			// Create a matching silence
			s = &silencepb.Silence{
				Matchers: []*silencepb.Matcher{{
					Type:    silencepb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now.Add(time.Minute)),
			}
			matchingCreated++
		} else {
			// Create a non-matching silence
			s = &silencepb.Silence{
				Matchers: []*silencepb.Matcher{{
					Type:    silencepb.Matcher_EQUAL,
					Name:    "job",
					Pattern: "job" + strconv.Itoa(i),
				}},
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now.Add(time.Minute)),
			}
		}
		require.NoError(b, silences.Set(b.Context(), s))
	}

	m := types.NewMarker(prometheus.NewRegistry())
	s := NewSilencer(silences, m, promslog.NewNopLogger())

	for b.Loop() {
		s.Mutes(context.Background(), model.LabelSet{"foo": "bar"})
	}
	b.StopTimer()

	// The alert should be marked as silenced for each matching silence.
	activeIDs, silenced := m.Silenced(model.LabelSet{"foo": "bar"}.Fingerprint())
	require.True(b, silenced || matchingSilences == 0)
	require.Len(b, activeIDs, matchingSilences)
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

			clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
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
						StartsAt: timestamppb.New(now),
						EndsAt:   timestamppb.New(now.Add(time.Hour)),
					}
				} else {
					s = &silencepb.Silence{
						Matchers: []*silencepb.Matcher{{
							Type:    silencepb.Matcher_EQUAL,
							Name:    "job",
							Pattern: "job" + strconv.Itoa(i),
						}},
						StartsAt: timestamppb.New(now),
						EndsAt:   timestamppb.New(now.Add(time.Hour)),
					}
				}
				require.NoError(b, silences.Set(b.Context(), s))
			}

			marker := types.NewMarker(prometheus.NewRegistry())
			silencer := NewSilencer(silences, marker, promslog.NewNopLogger())

			// Warm up: Establish cache state (cachedEntry.version = current version)
			// This simulates a system that has been running for a while
			lset := model.LabelSet{"service": "test", "instance": "instance1"}
			silencer.Mutes(context.Background(), lset)

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
							StartsAt: timestamppb.New(now),
							EndsAt:   timestamppb.New(now.Add(time.Hour)),
						}
					} else {
						// Most don't match
						s = &silencepb.Silence{
							Matchers: []*silencepb.Matcher{{
								Type:    silencepb.Matcher_EQUAL,
								Name:    "instance",
								Pattern: "host" + strconv.Itoa(iteration),
							}},
							StartsAt: timestamppb.New(now),
							EndsAt:   timestamppb.New(now.Add(time.Hour)),
						}
					}
					require.NoError(b, silences.Set(b.Context(), s))
				}

				b.StartTimer()
				// Now query - should use incremental path or cached paths
				silencer.Mutes(context.Background(), lset)
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

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create silences using Set() to properly populate indices
	for i := range numSilences {
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
			StartsAt:  timestamppb.New(now.Add(-time.Minute)),
			EndsAt:    timestamppb.New(now.Add(time.Hour)),
			UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
		}
		require.NoError(b, s.Set(b.Context(), sil))
	}

	// Run things once to populate the matcherCache.
	sils, _, err := s.Query(
		b.Context(),
		QState(SilenceStateActive),
		QMatches(lset),
	)
	require.NoError(b, err)
	require.Len(b, sils, numSilences/10)

	for b.Loop() {
		sils, _, err := s.Query(
			b.Context(),
			QState(SilenceStateActive),
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
	b.Run("100 silences", func(b *testing.B) {
		benchmarkQueryParallel(b, 100)
	})
	b.Run("1000 silences", func(b *testing.B) {
		benchmarkQueryParallel(b, 1000)
	})
	b.Run("10000 silences", func(b *testing.B) {
		benchmarkQueryParallel(b, 10000)
	})
}

func benchmarkQueryParallel(b *testing.B, numSilences int) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create silences with pre-compiled matchers
	for i := range numSilences {
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
			StartsAt:  timestamppb.New(now.Add(-time.Minute)),
			EndsAt:    timestamppb.New(now.Add(time.Hour)),
			UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
		}
		require.NoError(b, s.Set(b.Context(), sil))
	}

	// Verify initial query works
	sils, _, err := s.Query(
		b.Context(),
		QState(SilenceStateActive),
		QMatches(lset),
	)
	require.NoError(b, err)
	require.Len(b, sils, numSilences/10)

	b.ResetTimer()

	// Run queries in parallel across multiple goroutines
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sils, _, err := s.Query(
				b.Context(),
				QState(SilenceStateActive),
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

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	s.clock = clock
	now := clock.Now()

	lset := model.LabelSet{"aaaa": "AAAA", "bbbb": "BBBB", "cccc": "CCCC"}

	// Create initial silences
	for i := range initialSilences {
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
			StartsAt:  timestamppb.New(now.Add(-time.Minute)),
			EndsAt:    timestamppb.New(now.Add(time.Hour)),
			UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
		}
		require.NoError(b, s.Set(b.Context(), sil))
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
					StartsAt:  timestamppb.New(now.Add(-time.Minute)),
					EndsAt:    timestamppb.New(now.Add(time.Hour)),
					UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
				}
				if err := s.Set(b.Context(), sil); err != nil {
					b.Error(err)
				}
			} else {
				// Query silences (the common operation)
				_, _, err := s.Query(
					b.Context(),
					QState(SilenceStateActive),
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
	b.Run("100 silences", func(b *testing.B) {
		benchmarkMutesParallel(b, 100)
	})
	b.Run("1000 silences", func(b *testing.B) {
		benchmarkMutesParallel(b, 1000)
	})
	b.Run("10000 silences", func(b *testing.B) {
		benchmarkMutesParallel(b, 10000)
	})
}

func benchmarkMutesParallel(b *testing.B, numSilences int) {
	silences, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(b, err)

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	silences.clock = clock
	now := clock.Now()

	// Create silences that will match the alert
	for range numSilences {
		s := &silencepb.Silence{
			Matchers: []*silencepb.Matcher{{
				Type:    silencepb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		require.NoError(b, silences.Set(b.Context(), s))
	}

	m := types.NewMarker(prometheus.NewRegistry())
	silencer := NewSilencer(silences, m, promslog.NewNopLogger())

	b.ResetTimer()

	// Run Mutes in parallel
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			silencer.Mutes(b.Context(), model.LabelSet{"foo": "bar"})
		}
	})
}

// BenchmarkGC benchmarks the garbage collection performance for different
// numbers of silences and different ratios of expired silences.
func BenchmarkGC(b *testing.B) {
	b.Run("1000 silences, 0% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.0)
	})
	b.Run("1000 silences, 30% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.3)
	})
	b.Run("1000 silences, 80% expired", func(b *testing.B) {
		benchmarkGC(b, 1000, 0.8)
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
	b.Run("10000 silences, 80% expired", func(b *testing.B) {
		benchmarkGC(b, 10000, 0.8)
	})
}

func benchmarkGC(b *testing.B, numSilences int, expiredRatio float64) {
	b.ReportAllocs()

	clock := quartz.NewMock(b).WithLogger(quartz.NoOpLogger)
	now := clock.Now()

	numExpired := int(float64(numSilences) * expiredRatio)
	numActive := numSilences - numExpired

	matchers := []*silencepb.Matcher{{
		Type:    silencepb.Matcher_EQUAL,
		Name:    "foo",
		Pattern: "bar",
	}}
	startTime := timestamppb.New(now.Add(-2 * time.Hour))
	updateTime := timestamppb.New(now.Add(-2 * time.Hour))
	endTime := timestamppb.New(now.Add(-time.Hour))
	expireTime := timestamppb.New(now.Add(-time.Minute))
	activeTime := timestamppb.New(now.Add(2 * time.Hour))

	sils := make([]*silencepb.MeshSilence, 0, numSilences)

	for _, j := range rand.Perm(numSilences) {
		if j < numExpired {
			sil := &silencepb.MeshSilence{
				Silence: &silencepb.Silence{
					Id:        fmt.Sprintf("expired-%d", j),
					Matchers:  matchers,
					StartsAt:  startTime,
					EndsAt:    endTime,
					UpdatedAt: updateTime,
				},
				ExpiresAt: expireTime,
			}
			sils = append(sils, sil)
		} else {
			sil := &silencepb.MeshSilence{
				Silence: &silencepb.Silence{
					Id:        fmt.Sprintf("active-%d", j),
					Matchers:  matchers,
					StartsAt:  startTime,
					EndsAt:    endTime,
					UpdatedAt: updateTime,
				},
				ExpiresAt: activeTime,
			}
			sils = append(sils, sil)
		}
	}

	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()

		s, err := New(Options{
			Metrics: prometheus.NewRegistry(),
		})
		require.NoError(b, err)
		s.clock = clock

		for _, sil := range sils {
			s.st[sil.Silence.Id] = sil
			s.indexSilence(sil.Silence)
		}

		b.StartTimer()
		n1, err := s.GC()
		require.NoError(b, err)
		n2, err := s.GC()
		require.NoError(b, err)
		b.StopTimer()

		require.NoError(b, err)
		require.Equal(b, numExpired, n1)
		require.Equal(b, 0, n2)
		require.Len(b, s.st, numActive)
		require.Len(b, s.mi, numActive)
		b.StartTimer()
	}
}
