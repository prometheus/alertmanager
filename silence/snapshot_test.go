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
	"bytes"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestSilencesSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Check whether storing and loading the snapshot is symmetric.
		now := time.Now().UTC()

		cases := []struct {
			entries []*pb.MeshSilence
		}{
			{
				entries: []*pb.MeshSilence{
					{
						Silence: &pb.Silence{
							Id: "3be80475-e219-4ee7-b6fc-4b65114e362f",
							MatcherSets: []*pb.MatcherSet{{
								Matchers: []*pb.Matcher{
									{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
									{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
								},
							}},
							StartsAt:  timestamppb.New(now),
							EndsAt:    timestamppb.New(now),
							UpdatedAt: timestamppb.New(now),
						},
						ExpiresAt: timestamppb.New(now),
					},
					{
						Silence: &pb.Silence{
							Id: "3dfb2528-59ce-41eb-b465-f875a4e744a4",
							MatcherSets: []*pb.MatcherSet{{
								Matchers: []*pb.Matcher{
									{Name: "label1", Pattern: "val1", Type: pb.Matcher_NOT_EQUAL},
									{Name: "label2", Pattern: "val.+", Type: pb.Matcher_NOT_REGEXP},
								},
							}},
							StartsAt:  timestamppb.New(now),
							EndsAt:    timestamppb.New(now),
							UpdatedAt: timestamppb.New(now),
						},
						ExpiresAt: timestamppb.New(now),
					},
					{
						Silence: &pb.Silence{
							Id: "4b1e760d-182c-4980-b873-c1a6827c9817",
							MatcherSets: []*pb.MatcherSet{{
								Matchers: []*pb.Matcher{
									{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								},
							}},
							StartsAt:  timestamppb.New(now.Add(time.Hour)),
							EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
							UpdatedAt: timestamppb.New(now),
						},
						ExpiresAt: timestamppb.New(now.Add(24 * time.Hour)),
					},
				},
			},
		}

		for _, c := range cases {
			f, err := os.CreateTemp(t.TempDir(), "snapshot")
			require.NoError(t, err, "creating temp file failed")

			s1 := &Silences{st: state{}, metrics: newMetrics(nil, nil)}
			// Setup internal state manually.
			for _, e := range c.entries {
				s1.st[e.Silence.Id] = e
			}
			_, err = s1.Snapshot(f)
			require.NoError(t, err, "creating snapshot failed")

			require.NoError(t, f.Close(), "closing snapshot file failed")

			f, err = os.Open(f.Name())
			require.NoError(t, err, "opening snapshot file failed")

			// Check again against new nlog instance.
			s2 := &Silences{mi: matcherIndex{}, st: state{}}
			err = s2.loadSnapshot(f)
			require.NoError(t, err, "error loading snapshot")
			require.Len(t, s2.st, len(s1.st), "state length mismatch after loading snapshot")
			for id, expected := range s1.st {
				actual, ok := s2.st[id]
				require.True(t, ok, "silence %s missing from loaded state", id)
				require.True(t, proto.Equal(expected, actual), "silence %s mismatch after loading snapshot", id)
			}

			require.NoError(t, f.Close(), "closing snapshot file failed")
		}
	})
}

func TestSilencesLoadSnapshotDropsUncompilableSilences(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	entries := []*pb.MeshSilence{
		{
			Silence: &pb.Silence{
				Id: "valid-silence",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
					},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now),
			},
			ExpiresAt: timestamppb.New(now.Add(2 * time.Hour)),
		},
		{
			Silence: &pb.Silence{
				Id: "invalid-regex-silence",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "label1", Pattern: "[unterminated", Type: pb.Matcher_REGEXP},
					},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now),
			},
			ExpiresAt: timestamppb.New(now.Add(2 * time.Hour)),
		},
		{
			Silence: &pb.Silence{
				Id: "unknown-type-silence",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "label1", Pattern: "val1", Type: pb.Matcher_Type(999)},
					},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now),
			},
			ExpiresAt: timestamppb.New(now.Add(2 * time.Hour)),
		},
	}

	var buf bytes.Buffer
	for _, e := range entries {
		_, err := protodelim.MarshalTo(&buf, e)
		require.NoError(t, err)
	}

	reg := prometheus.NewRegistry()
	s, err := New(Options{
		SnapshotReader: bytes.NewReader(buf.Bytes()),
		Metrics:        reg,
	})
	require.NoError(t, err)

	// Uncompilable silences must be removed from state as well as omitted from
	// the matcher and version indices.
	require.Equal(t, float64(2), testutil.ToFloat64(s.metrics.matcherCompileLoadSnapshotErrorsTotal))
	require.Equal(t, float64(1), testutil.ToFloat64(s.metrics.stateSize))
	require.Equal(t, float64(1), testutil.ToFloat64(s.metrics.matcherIndexSize))
	require.Equal(t, float64(1), testutil.ToFloat64(s.metrics.versionIndexSize))

	// Both full scan and ID queries must agree and exclude the dropped silences.
	all, _, err := s.Query(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "valid-silence", all[0].Id)

	byID, _, err := s.Query(t.Context(), QIDs("invalid-regex-silence", "unknown-type-silence"))
	require.NoError(t, err)
	require.Empty(t, byID)

	matched, _, err := s.Query(
		t.Context(),
		QIDs("invalid-regex-silence", "unknown-type-silence", "valid-silence"),
		QMatches(model.LabelSet{"label1": "val1"}),
	)
	require.NoError(t, err)
	require.Len(t, matched, 1)
	require.Equal(t, "valid-silence", matched[0].Id)

	// Re-snapshotting must not persist dropped silences back to disk.
	var snap bytes.Buffer
	_, err = s.Snapshot(&snap)
	require.NoError(t, err)
	decoded, err := decodeState(&snap)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	require.Contains(t, decoded, "valid-silence")
}
