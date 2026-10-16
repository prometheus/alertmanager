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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestSilenceLimits(t *testing.T) {
	s, err := New(Options{
		Limits: Limits{
			MaxSilences:         func() int { return 1 },
			MaxSilenceSizeBytes: func() int { return 2 << 11 }, // 4KB
		},
		Metrics: prometheus.NewRegistry(),
	})
	require.NoError(t, err)

	// Insert sil1 should succeed without error.
	sil1 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		}},
		StartsAt: timestamppb.New(time.Now()),
		EndsAt:   timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	require.NoError(t, s.Set(t.Context(), sil1))

	// Insert sil2 should fail because maximum number of silences has been
	// exceeded.
	sil2 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "c", Pattern: "d"}},
		}},
		StartsAt: timestamppb.New(time.Now()),
		EndsAt:   timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	require.EqualError(t, s.Set(t.Context(), sil2), "exceeded maximum number of silences: 1 (limit: 1)")

	// Expire sil1 and run the GC. This should allow sil2 to be inserted.
	require.NoError(t, s.Expire(t.Context(), sil1.Id))
	n, err := s.GC()
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, s.Set(t.Context(), sil2))

	// Expire sil2 and run the GC.
	require.NoError(t, s.Expire(t.Context(), sil2.Id))
	n, err = s.GC()
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Insert sil3 should fail because it exceeds maximum size.
	sil3 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{
				{
					Name:    strings.Repeat("e", 2<<9),
					Pattern: strings.Repeat("f", 2<<9),
				},
				{
					Name:    strings.Repeat("g", 2<<9),
					Pattern: strings.Repeat("h", 2<<9),
				},
			},
		}},
		CreatedBy: strings.Repeat("i", 2<<9),
		Comment:   strings.Repeat("j", 2<<9),
		StartsAt:  timestamppb.New(time.Now()),
		EndsAt:    timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	require.EqualError(t, s.Set(t.Context(), sil3), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", proto.Size(s.toMeshSilence(sil3))))

	// Should be able to insert sil4.
	sil4 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "k", Pattern: "l"}},
		}},
		StartsAt: timestamppb.New(time.Now()),
		EndsAt:   timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	require.NoError(t, s.Set(t.Context(), sil4))

	// Should be able to update sil4 without modifications. It is expected to
	// keep the same ID.
	sil5 := cloneSilence(sil4)
	require.NoError(t, s.Set(t.Context(), sil5))
	require.Equal(t, sil4.Id, sil5.Id)

	// Should be able to update the comment. It is also expected to keep the
	// same ID.
	sil6 := cloneSilence(sil5)
	sil6.Comment = "m"
	require.NoError(t, s.Set(t.Context(), sil6))
	require.Equal(t, sil5.Id, sil6.Id)

	// Should not be able to update the start and end time as this requires
	// sil6 to be expired and a new silence to be created. However, this would
	// exceed the maximum number of silences, which counts both active and
	// expired silences.
	sil7 := cloneSilence(sil6)
	sil7.StartsAt = timestamppb.New(time.Now().Add(1 * time.Minute))
	sil7.EndsAt = timestamppb.New(time.Now().Add(10 * time.Minute))
	require.EqualError(t, s.Set(t.Context(), sil7), "exceeded maximum number of silences: 1 (limit: 1)")

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(t.Context(), QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, SilenceStateActive, StateFromProtoAt(sil6, s.nowUTC()))

	// Should not be able to update with a comment that exceeds maximum size.
	// Need to increase the maximum number of silences to test this.
	s.limits.MaxSilences = func() int { return 2 }
	sil8 := cloneSilence(sil6)
	sil8.Comment = strings.Repeat("m", 2<<11)
	require.EqualError(t, s.Set(t.Context(), sil8), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", proto.Size(s.toMeshSilence(sil8))))

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(t.Context(), QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, SilenceStateActive, StateFromProtoAt(sil6, s.nowUTC()))

	// Should not be able to replace with a silence that exceeds maximum size.
	// This is different from the previous assertion as unlike when adding or
	// updating a comment, changing the matchers for a silence should expire
	// the existing silence, unless the silence that is replacing it exceeds
	// limits, in which case the operation should fail and the existing silence
	// should still be active.
	sil9 := cloneSilence(sil8)
	sil9.Matchers = []*pb.Matcher{{Name: "n", Pattern: "o"}}
	require.EqualError(t, s.Set(t.Context(), sil9), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", proto.Size(s.toMeshSilence(sil9))))

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(t.Context(), QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, SilenceStateActive, StateFromProtoAt(sil6, s.nowUTC()))
}

func TestSilenceNoLimits(t *testing.T) {
	s, err := New(Options{
		Limits:  Limits{},
		Metrics: prometheus.NewRegistry(),
	})
	require.NoError(t, err)

	// Insert sil should succeed without error.
	sil := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		}},
		StartsAt: timestamppb.New(time.Now()),
		EndsAt:   timestamppb.New(time.Now().Add(5 * time.Minute)),
		Comment:  strings.Repeat("c", 2<<9),
	}
	require.NoError(t, s.Set(t.Context(), sil))
	require.NotEmpty(t, sil.Id)
}
