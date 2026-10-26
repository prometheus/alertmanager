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
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestSilenceExpire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
		require.NoError(t, err)

		now := s.nowUTC()

		m := &pb.Matcher{Type: pb.Matcher_EQUAL, Name: "a", Pattern: "b"}

		s.st = state{
			"pending": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "pending",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
			"active": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "active",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
			"expired": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "expired",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(-time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
		}
		s.vi = versionIndex{
			silenceVersion{id: "pending"},
			silenceVersion{id: "active"},
			silenceVersion{id: "expired"},
		}
		count, err := s.CountState(t.Context(), SilenceStatePending)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = s.CountState(t.Context(), SilenceStateExpired)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		require.NoError(t, s.Expire(t.Context(), "pending"))
		require.NoError(t, s.Expire(t.Context(), "active"))

		require.NoError(t, s.Expire(t.Context(), "expired"))

		sil, err := s.QueryOne(t.Context(), QIDs("pending"))
		require.NoError(t, err)
		expectedPending := &pb.Silence{
			Id: "pending",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{m},
			}},
			StartsAt:  timestamppb.New(now),
			EndsAt:    timestamppb.New(now),
			UpdatedAt: timestamppb.New(now),
		}
		require.True(t, proto.Equal(expectedPending, sil), "pending silence mismatch")

		// Let time pass...
		time.Sleep(time.Second)

		count, err = s.CountState(t.Context(), SilenceStatePending)
		require.NoError(t, err)
		require.Equal(t, 0, count)

		count, err = s.CountState(t.Context(), SilenceStateExpired)
		require.NoError(t, err)
		require.Equal(t, 3, count)

		// Expiring a pending Silence should make the API return the
		// SilenceStateExpired Silence state.
		silenceState := CurrentState(sil.StartsAt.AsTime(), sil.EndsAt.AsTime())
		require.Equal(t, SilenceStateExpired, silenceState)

		sil, err = s.QueryOne(t.Context(), QIDs("active"))
		require.NoError(t, err)
		expectedActive := &pb.Silence{
			Id: "active",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{m},
			}},
			StartsAt:  timestamppb.New(now.Add(-time.Minute)),
			EndsAt:    timestamppb.New(now),
			UpdatedAt: timestamppb.New(now),
		}
		require.True(t, proto.Equal(expectedActive, sil), "active silence mismatch")

		sil, err = s.QueryOne(t.Context(), QIDs("expired"))
		require.NoError(t, err)
		expectedExpired := &pb.Silence{
			Id: "expired",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{m},
			}},
			StartsAt:  timestamppb.New(now.Add(-time.Hour)),
			EndsAt:    timestamppb.New(now.Add(-time.Minute)),
			UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
		}
		require.True(t, proto.Equal(expectedExpired, sil), "expired silence mismatch")
	})
}

// TestSilenceExpireWithZeroRetention covers the problem that, with zero
// retention time, a silence explicitly set to expired will also immediately
// expire from the silence storage.
func TestSilenceExpireWithZeroRetention(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: 0})
		require.NoError(t, err)

		now := s.nowUTC()

		m := &pb.Matcher{Type: pb.Matcher_EQUAL, Name: "a", Pattern: "b"}

		s.st = state{
			"pending": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "pending",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
			"active": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "active",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Minute)),
				EndsAt:    timestamppb.New(now.Add(time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
			"expired": &pb.MeshSilence{Silence: &pb.Silence{
				Id: "expired",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{m},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(-time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}},
		}
		s.vi = versionIndex{
			silenceVersion{id: "pending"},
			silenceVersion{id: "active"},
			silenceVersion{id: "expired"},
		}

		count, err := s.CountState(t.Context(), SilenceStatePending)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = s.CountState(t.Context(), SilenceStateActive)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		count, err = s.CountState(t.Context(), SilenceStateExpired)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		// Advance time. The silence state management code uses update time when
		// merging, and the logic is "first write wins". So we must advance the clock
		// one tick for updates to take effect.
		time.Sleep(1 * time.Millisecond)

		require.NoError(t, s.Expire(t.Context(), "pending"))
		require.NoError(t, s.Expire(t.Context(), "active"))
		require.NoError(t, s.Expire(t.Context(), "expired"))

		// Advance time again. Despite what the function name says, s.Expire() does
		// not expire a silence. It sets the silence to EndAt the current time. This
		// means that the silence is active immediately after calling Expire.
		time.Sleep(1 * time.Millisecond)

		// Verify all silences have expired.
		count, err = s.CountState(t.Context(), SilenceStatePending)
		require.NoError(t, err)
		require.Equal(t, 0, count)

		count, err = s.CountState(t.Context(), SilenceStateActive)
		require.NoError(t, err)
		require.Equal(t, 0, count)

		count, err = s.CountState(t.Context(), SilenceStateExpired)
		require.NoError(t, err)
		require.Equal(t, 3, count)
	})
}

// This test checks that invalid silences can be expired.
func TestSilenceExpireInvalid(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
		require.NoError(t, err)

		now := s.nowUTC()

		// In this test the matcher has an invalid type.
		silence := pb.Silence{
			Id: "active",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Type: -1, Name: "a", Pattern: "b"}},
			}},
			StartsAt:  timestamppb.New(now.Add(-time.Minute)),
			EndsAt:    timestamppb.New(now.Add(time.Hour)),
			UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
		}
		// Assert that this silence is invalid.
		require.EqualError(t, validateSilence(&silence), "invalid label matcher 0 in set 0: unknown matcher type \"-1\"")

		s.st = state{"active": &pb.MeshSilence{Silence: &silence}}
		s.vi = versionIndex{silenceVersion{id: "active"}}

		// The silence should be active.
		count, err := s.CountState(t.Context(), SilenceStateActive)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		time.Sleep(time.Millisecond)
		require.NoError(t, s.Expire(t.Context(), "active"))
		time.Sleep(time.Millisecond)

		// The silence should be expired.
		count, err = s.CountState(t.Context(), SilenceStateActive)
		require.NoError(t, err)
		require.Equal(t, 0, count)
		count, err = s.CountState(t.Context(), SilenceStateExpired)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})
}
