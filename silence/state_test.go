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

func newStateTestSilence(start, end time.Time) *pb.Silence {
	return &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{Matchers: []*pb.Matcher{{Name: "alertname", Pattern: "Foo"}}}},
		StartsAt:    timestamppb.New(start),
		EndsAt:      timestamppb.New(end),
	}
}

func TestSilenceStateBoundaries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		offset, duration time.Duration
		want             SilenceState
	}{
		{"before start", -time.Nanosecond, time.Hour, SilenceStatePending},
		{"at start", 0, time.Hour, SilenceStateActive},
		{"before end", time.Hour - time.Nanosecond, time.Hour, SilenceStateActive},
		{"at end", time.Hour, time.Hour, SilenceStateExpired},
		{"after end", time.Hour + time.Nanosecond, time.Hour, SilenceStateExpired},
		{"zero duration", 0, 0, SilenceStateExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now().Add(time.Hour)
				end := start.Add(tc.duration)
				at := start.Add(tc.offset)
				sil := newStateTestSilence(start, end)
				require.Equal(t, tc.want, StateAt(start, end, at))
				require.Equal(t, tc.want, StateFromProtoAt(sil, at))
				time.Sleep(time.Until(at))
				require.Equal(t, tc.want, CurrentState(start, end))
			})
		})
	}
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
