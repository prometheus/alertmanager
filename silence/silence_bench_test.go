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
	silences, err := New(Options{})
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
	s, err := New(Options{})
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sils, _, err := s.Query(
			QState(types.SilenceStateActive),
			QMatches(lset),
		)
		require.NoError(b, err)
		require.Len(b, sils, numSilences/10)
	}
}
