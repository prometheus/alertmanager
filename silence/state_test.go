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
	"bufio"
	"bytes"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestCurrentState(t *testing.T) {
	var (
		pastStartTime = time.Now()
		pastEndTime   = time.Now()

		futureStartTime = time.Now().Add(time.Hour)
		futureEndTime   = time.Now().Add(time.Hour)
	)

	expected := CurrentState(futureStartTime, futureEndTime)
	require.Equal(t, SilenceStatePending, expected)

	expected = CurrentState(pastStartTime, futureEndTime)
	require.Equal(t, SilenceStateActive, expected)

	expected = CurrentState(pastStartTime, pastEndTime)
	require.Equal(t, SilenceStateExpired, expected)
}

func TestSilenceStateBoundaries(t *testing.T) {
	t.Parallel()

	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, tc := range []struct {
		name string
		now  time.Time
		end  time.Time
		want SilenceState
	}{
		{"before start", start.Add(-time.Nanosecond), end, SilenceStatePending},
		{"at start", start, end, SilenceStateActive},
		{"before end", end.Add(-time.Nanosecond), end, SilenceStateActive},
		{"at end", end, end, SilenceStateExpired},
		{"after end", end.Add(time.Nanosecond), end, SilenceStateExpired},
		{"zero duration", start, start, SilenceStateExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sil := &pb.Silence{StartsAt: timestamppb.New(start), EndsAt: timestamppb.New(tc.end)}
			require.Equal(t, tc.want, getState(sil, tc.now))
			require.Equal(t, tc.want, StateAt(start, tc.end, tc.now))
			q := &query{}
			require.NoError(t, QState(tc.want)(q))
			matches, err := q.filters[0](sil, nil, tc.now)
			require.NoError(t, err)
			require.True(t, matches)
		})
	}
}

func TestSetAtEndBoundaryPreservesExpiredHistory(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		now := time.Now().UTC()
		store, err := New(Options{Retention: time.Hour, Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		original := &pb.Silence{
			StartsAt: timestamppb.New(now), EndsAt: timestamppb.New(now.Add(time.Hour)),
			MatcherSets: []*pb.MatcherSet{{Matchers: []*pb.Matcher{{Name: "alertname", Pattern: "Foo"}}}},
		}
		require.NoError(t, store.Set(t.Context(), original))
		time.Sleep(time.Hour)
		now = time.Now().UTC()
		replacement := cloneSilence(original)
		replacement.EndsAt = timestamppb.New(now.Add(time.Hour))
		require.NoError(t, store.Set(t.Context(), replacement))
		require.NotEqual(t, original.Id, replacement.Id)
		stored, err := store.QueryOne(t.Context(), QIDs(original.Id), QState(SilenceStateExpired))
		require.NoError(t, err)
		require.Equal(t, now, stored.EndsAt.AsTime())
		_, err = store.QueryOne(t.Context(), QIDs(replacement.Id), QState(SilenceStateActive))
		require.NoError(t, err)
	})
}

// TestStateMarshalBinaryPopulatesLegacyMatchers asserts that snapshots and
// cluster local-state payloads written by state.MarshalBinary include the
// deprecated Silence.Matchers field, so that older Alertmanagers that don't
// understand MatcherSets still see the silence's first matcher set.
func TestStateMarshalBinaryPopulatesLegacyMatchers(t *testing.T) {
	now := time.Now()
	matchers := []*pb.Matcher{
		{Name: "alertname", Pattern: "Foo", Type: pb.Matcher_EQUAL},
		{Name: "severity", Pattern: "warn|crit", Type: pb.Matcher_REGEXP},
	}
	sil := &pb.Silence{
		Id:          "abc",
		MatcherSets: []*pb.MatcherSet{{Matchers: matchers}},
		StartsAt:    timestamppb.New(now),
		EndsAt:      timestamppb.New(now.Add(time.Hour)),
	}
	st := state{sil.Id: &pb.MeshSilence{
		Silence:   sil,
		ExpiresAt: timestamppb.New(now.Add(2 * time.Hour)),
	}}

	b, err := st.MarshalBinary()
	require.NoError(t, err)

	require.Nil(t, sil.Matchers, "MarshalBinary must not mutate in-memory silences")

	// Decode directly via protodelim, bypassing decodeState — decodeState
	// strips the legacy field, but we want to observe the on-the-wire shape.
	var got pb.MeshSilence
	require.NoError(t, protodelim.UnmarshalFrom(bufio.NewReader(bytes.NewReader(b)), &got))

	require.Len(t, got.Silence.MatcherSets, 1)
	require.Len(t, got.Silence.Matchers, len(matchers))
	for i, m := range matchers {
		require.True(t, proto.Equal(m, got.Silence.Matchers[i]),
			"legacy Matchers[%d] mismatch", i)
		require.True(t, proto.Equal(m, got.Silence.MatcherSets[0].Matchers[i]),
			"MatcherSets[0].Matchers[%d] mismatch", i)
	}
}
