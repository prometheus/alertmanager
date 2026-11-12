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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestSilenceBackwardCompatibility(t *testing.T) {
	t.Run("postprocessUnmarshalledSilence converts old format to new", func(t *testing.T) {
		// Create a silence with only the old Matchers field (simulating old format)
		oldSilence := &pb.Silence{
			Id: "test-id",
			Matchers: []*pb.Matcher{
				{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
				{Name: "instance", Pattern: "web-1", Type: pb.Matcher_EQUAL},
			},
			StartsAt: timestamppb.New(time.Now()),
			EndsAt:   timestamppb.New(time.Now().Add(time.Hour)),
		}

		// Process as if unmarshalled from old version
		postprocessUnmarshalledSilence(oldSilence)

		// Verify conversion to MatcherSets
		require.Len(t, oldSilence.MatcherSets, 1, "should have exactly one matcher set")
		require.Len(t, oldSilence.MatcherSets[0].Matchers, 2, "matcher set should have 2 matchers")
		require.Equal(t, "job", oldSilence.MatcherSets[0].Matchers[0].Name)
		require.Equal(t, "test", oldSilence.MatcherSets[0].Matchers[0].Pattern)
		require.Equal(t, "instance", oldSilence.MatcherSets[0].Matchers[1].Name)
		require.Equal(t, "web-1", oldSilence.MatcherSets[0].Matchers[1].Pattern)

		// Verify old Matchers field is cleared
		require.Nil(t, oldSilence.Matchers, "old Matchers field should be cleared")
	})

	t.Run("prepareSilenceForMarshalling populates old format from new", func(t *testing.T) {
		// Create a silence with new MatcherSets field
		newSilence := &pb.Silence{
			Id: "test-id",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					{Name: "instance", Pattern: "web-1", Type: pb.Matcher_EQUAL},
				},
			}},
			StartsAt: timestamppb.New(time.Now()),
			EndsAt:   timestamppb.New(time.Now().Add(time.Hour)),
		}

		// Prepare for marshalling (for backward compatibility)
		prepareSilenceForMarshalling(newSilence)

		// Verify old Matchers field is populated from first matcher set
		require.Len(t, newSilence.Matchers, 2, "old Matchers field should be populated")
		require.Equal(t, "job", newSilence.Matchers[0].Name)
		require.Equal(t, "test", newSilence.Matchers[0].Pattern)
		require.Equal(t, "instance", newSilence.Matchers[1].Name)
		require.Equal(t, "web-1", newSilence.Matchers[1].Pattern)

		// Verify MatcherSets is still intact
		require.Len(t, newSilence.MatcherSets, 1)
	})

	t.Run("round-trip conversion preserves data", func(t *testing.T) {
		// Start with new format
		original := &pb.Silence{
			Id: "test-id",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					{Name: "method", Pattern: "GET", Type: pb.Matcher_REGEXP},
				},
			}},
			StartsAt:  timestamppb.New(time.Now().Truncate(time.Second)),
			EndsAt:    timestamppb.New(time.Now().Add(time.Hour).Truncate(time.Second)),
			CreatedBy: "test-user",
			Comment:   "test comment",
		}

		// Marshal (prepare for backward compatibility)
		prepareSilenceForMarshalling(original)
		require.Len(t, original.Matchers, 2, "should populate old Matchers field")

		// Simulate round-trip by creating a new silence with only Matchers field
		// (as if received from old client)
		received := &pb.Silence{
			Id:        original.Id,
			Matchers:  original.Matchers,
			StartsAt:  original.StartsAt,
			EndsAt:    original.EndsAt,
			CreatedBy: original.CreatedBy,
			Comment:   original.Comment,
		}

		// Unmarshal (convert to new format)
		postprocessUnmarshalledSilence(received)

		// Verify data is preserved
		require.Len(t, received.MatcherSets, 1)
		require.Len(t, received.MatcherSets[0].Matchers, 2)
		require.Equal(t, original.MatcherSets[0].Matchers[0].Name, received.MatcherSets[0].Matchers[0].Name)
		require.Equal(t, original.MatcherSets[0].Matchers[0].Pattern, received.MatcherSets[0].Matchers[0].Pattern)
		require.Equal(t, original.MatcherSets[0].Matchers[0].Type, received.MatcherSets[0].Matchers[0].Type)
		require.Nil(t, received.Matchers, "old Matchers field should be cleared after postprocess")
	})

	t.Run("postprocess handles empty Matchers gracefully", func(t *testing.T) {
		// Silence with no matchers at all
		silence := &pb.Silence{
			Id:       "test-id",
			StartsAt: timestamppb.New(time.Now()),
			EndsAt:   timestamppb.New(time.Now().Add(time.Hour)),
		}

		postprocessUnmarshalledSilence(silence)

		require.Nil(t, silence.Matchers)
		require.Nil(t, silence.MatcherSets)
	})

	t.Run("postprocess prefers MatcherSets when both fields set", func(t *testing.T) {
		// Silence with both old and new fields (can happen during migration)
		silence := &pb.Silence{
			Id: "test-id",
			Matchers: []*pb.Matcher{
				{Name: "job", Pattern: "old-value", Type: pb.Matcher_EQUAL},
			},
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "new-value", Type: pb.Matcher_EQUAL},
				},
			}},
			StartsAt: timestamppb.New(time.Now()),
			EndsAt:   timestamppb.New(time.Now().Add(time.Hour)),
		}

		postprocessUnmarshalledSilence(silence)

		// MatcherSets field should be preserved when already set
		require.Len(t, silence.MatcherSets, 1)
		require.Equal(t, "new-value", silence.MatcherSets[0].Matchers[0].Pattern)
		require.Nil(t, silence.Matchers)
	})

	t.Run("multi-matcher silence backward compat populates only first set", func(t *testing.T) {
		// Create a silence with multiple matcher sets
		multiSilence := &pb.Silence{
			Id: "test-id",
			MatcherSets: []*pb.MatcherSet{
				{
					Matchers: []*pb.Matcher{
						{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					},
				},
				{
					Matchers: []*pb.Matcher{
						{Name: "method", Pattern: "GET", Type: pb.Matcher_EQUAL},
					},
				},
			},
			StartsAt: timestamppb.New(time.Now()),
			EndsAt:   timestamppb.New(time.Now().Add(time.Hour)),
		}

		// Prepare for marshalling
		prepareSilenceForMarshalling(multiSilence)

		// Only first matcher set should be in old Matchers field
		require.Len(t, multiSilence.Matchers, 1, "should only populate first matcher set")
		require.Equal(t, "job", multiSilence.Matchers[0].Name)
		require.Equal(t, "test", multiSilence.Matchers[0].Pattern)

		// All matcher sets should still be intact
		require.Len(t, multiSilence.MatcherSets, 2)
	})
}

func TestStateUnmarshalling(t *testing.T) {
	// test that we can decode silences with the old format (without MatcherSets field)
	now := time.Now().UTC()

	testCases := []struct {
		name     string
		silence  *pb.MeshSilence
		expected *pb.MeshSilence
	}{
		{
			name: "empty silence",
			silence: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
			expected: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
		},
		{
			name: "silence with matcher sets",
			silence: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
			expected: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
		},
		{
			name: "silence with multiple matcher sets",
			silence: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val2", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val2.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
			expected: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val2", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val2.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
		},
		{
			name: "silence with both classic matchers and matcher sets",
			silence: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					Matchers: []*pb.Matcher{
						{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
					},
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val2", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val2.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
			expected: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val2", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val2.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
		},
		{
			name: "silence with classic matchers",
			silence: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					Matchers: []*pb.Matcher{
						{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
			expected: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: "silence1",
					MatcherSets: []*pb.MatcherSet{
						{
							Matchers: []*pb.Matcher{
								{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
								{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
							},
						},
					},
				},
				ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the silence to binary format
			in := state{
				tt.silence.Silence.Id: tt.silence,
			}

			msg, err := in.MarshalBinary()
			require.NoError(t, err)

			decoded, err := decodeState(bytes.NewReader(msg))
			require.NoError(t, err, "decoding message failed")

			require.True(t, proto.Equal(tt.expected, decoded[tt.silence.Silence.Id]), "decoded data doesn't match encoded data")
		})
	}
}

func TestStateMerge(t *testing.T) {
	now := time.Now().UTC()

	// We only care about key names and timestamps for the
	// merging logic.
	newSilence := func(id string, ts, exp time.Time) *pb.MeshSilence {
		return &pb.MeshSilence{
			Silence:   &pb.Silence{Id: id, UpdatedAt: timestamppb.New(ts)},
			ExpiresAt: timestamppb.New(exp),
		}
	}

	exp := now.Add(time.Minute)

	cases := []struct {
		a, b  state
		final state
	}{
		{
			a: state{
				"a1": newSilence("a1", now, exp),
				"a2": newSilence("a2", now, exp),
				"a3": newSilence("a3", now, exp),
			},
			b: state{
				"b1": newSilence("b1", now, exp),                                          // new key, should be added
				"a2": newSilence("a2", now.Add(-time.Minute), exp),                        // older timestamp, should be dropped
				"a3": newSilence("a3", now.Add(time.Minute), exp),                         // newer timestamp, should overwrite
				"a4": newSilence("a4", now.Add(-time.Minute), now.Add(-time.Millisecond)), // new key, expired, should not be added
			},
			final: state{
				"a1": newSilence("a1", now, exp),
				"a2": newSilence("a2", now, exp),
				"a3": newSilence("a3", now.Add(time.Minute), exp),
				"b1": newSilence("b1", now, exp),
			},
		},
	}

	for _, c := range cases {
		for _, e := range c.b {
			c.a.merge(e, now)
		}

		require.Equal(t, c.final, c.a, "Merge result should match expectation")
	}
}

func TestStateCoding(t *testing.T) {
	// Check whether encoding and decoding the data is symmetric.
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
			},
		},
	}

	for _, c := range cases {
		// Create gossip data from input.
		in := state{}
		for _, e := range c.entries {
			in[e.Silence.Id] = e
		}
		msg, err := in.MarshalBinary()
		require.NoError(t, err)

		out, err := decodeState(bytes.NewReader(msg))
		require.NoError(t, err, "decoding message failed")

		require.Len(t, out, len(in), "decoded state length mismatch")
		for id, expected := range in {
			actual, ok := out[id]
			require.True(t, ok, "silence %s missing from decoded state", id)
			require.True(t, proto.Equal(expected, actual), "silence %s mismatch after decoding", id)
		}
	}
}

func TestStateDecodingError(t *testing.T) {
	// Check whether decoding copes with erroneous data.
	s := state{"": &pb.MeshSilence{}}

	msg, err := s.MarshalBinary()
	require.NoError(t, err)

	_, err = decodeState(bytes.NewReader(msg))
	require.Equal(t, ErrInvalidState, err)
}
