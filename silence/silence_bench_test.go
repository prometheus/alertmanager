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

	s.st = state{}
	for i := 0; i < numSilences; i++ {
		id := strconv.Itoa(i)
		// Include an offset to avoid optimizations.
		patA := "A{4}|" + id
		patB := id // Does not match.
		if i%10 == 0 {
			// Every 10th time, have an actually matching pattern.
			patB = "B(B|C)B.|" + id
		}

		s.st[id] = &silencepb.MeshSilence{Silence: &silencepb.Silence{
			Id: id,
			Matchers: []*silencepb.Matcher{
				{Type: silencepb.Matcher_REGEXP, Name: "aaaa", Pattern: patA},
				{Type: silencepb.Matcher_REGEXP, Name: "bbbb", Pattern: patB},
			},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}}
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
