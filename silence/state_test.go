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
