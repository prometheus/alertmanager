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
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestSilencesSetSilence(t *testing.T) {
	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Minute,
	})
	require.NoError(t, err)

	nowpb := s.nowUTC()

	sil := &pb.Silence{
		Id: "some_id",
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "abc", Pattern: "def"}},
		}},
		StartsAt: timestamppb.New(nowpb),
		EndsAt:   timestamppb.New(nowpb),
	}

	want := state{
		"some_id": &pb.MeshSilence{
			Silence:   sil,
			ExpiresAt: timestamppb.New(nowpb.Add(time.Minute)),
		},
	}

	wantBroadcast := &pb.MeshSilence{
		Silence: &pb.Silence{
			Id:       "some_id",
			Matchers: sil.MatcherSets[0].Matchers, // Backward compatibility
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "abc", Pattern: "def"}},
			}},
			StartsAt: timestamppb.New(nowpb),
			EndsAt:   timestamppb.New(nowpb),
		},
		ExpiresAt: timestamppb.New(nowpb.Add(time.Minute)),
	}

	done := make(chan struct{})
	s.broadcast = func(b []byte) {
		var e pb.MeshSilence
		r := bytes.NewReader(b)
		err := protodelim.UnmarshalFrom(r, &e)
		require.NoError(t, err)

		require.True(t, proto.Equal(&e, wantBroadcast), "broadcast message mismatch")
		close(done)
	}

	// setSilence() is always called with s.mtx locked() in the application code
	func() {
		s.mtx.Lock()
		defer s.mtx.Unlock()
		_, _, err := s.setSilence(s.toMeshSilence(sil), nowpb)
		require.NoError(t, err)
	}()

	// Ensure broadcast was called.
	if _, isOpen := <-done; isOpen {
		t.Fatal("broadcast was not called")
	}

	requireStatesEqual(t, want, s.st, "Unexpected silence state")
}

// TestSilenceSetEndBoundary verifies that silences are correctly expired
// when their end time is reached, even if the system clock advances during
// the processing of the Set operation.
func TestSilenceSetEndBoundary(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
		require.NoError(t, err)
		now := store.nowUTC()
		original := newStateTestSilence(now, now.Add(time.Hour))
		require.NoError(t, store.Set(t.Context(), original))
		time.Sleep(time.Hour)
		updated := cloneSilence(original)
		updated.EndsAt = timestamppb.New(store.nowUTC().Add(time.Hour))
		require.NoError(t, store.Set(t.Context(), updated))
		require.NotEqual(t, original.Id, updated.Id)
		_, err = store.QueryOne(t.Context(), QIDs(updated.Id), QState(SilenceStateActive))
		require.NoError(t, err)
		previous, err := store.QueryOne(t.Context(), QIDs(original.Id), QState(SilenceStateExpired))
		require.NoError(t, err)
		require.True(t, proto.Equal(original, previous))
	})
}

// TestSilenceSetUsesOneEvaluationTime ensures that the evaluation time used
// for determining silence state is consistent across all operations within
// a single Set call, even if the system clock advances during processing.
func TestSilenceSetUsesOneEvaluationTime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		advance := false
		store, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
			Limits: Limits{MaxSilenceSizeBytes: func() int {
				if advance {
					time.Sleep(2 * time.Nanosecond)
				}
				return 0
			}},
		})
		require.NoError(t, err)
		now := store.nowUTC()
		original := newStateTestSilence(now, now.Add(time.Hour))
		require.NoError(t, store.Set(t.Context(), original))
		time.Sleep(time.Hour - time.Nanosecond)
		evaluatedAt := store.nowUTC()
		advance = true
		updated := cloneSilence(original)
		updated.MatcherSets[0].Matchers[0].Pattern = "Bar"
		updated.EndsAt = timestamppb.New(evaluatedAt.Add(time.Hour))
		require.NoError(t, store.Set(t.Context(), updated))
		require.True(t, store.nowUTC().After(original.EndsAt.AsTime()))
		require.NotEqual(t, original.Id, updated.Id)
		previous, err := store.QueryOne(t.Context(), QIDs(original.Id))
		require.NoError(t, err)
		require.Equal(t, evaluatedAt, previous.EndsAt.AsTime())
		require.Equal(t, evaluatedAt, previous.UpdatedAt.AsTime())
		require.Equal(t, evaluatedAt, updated.StartsAt.AsTime())
		require.Equal(t, evaluatedAt, updated.UpdatedAt.AsTime())
	})
}

func TestSilenceSet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
		})
		require.NoError(t, err)

		start1 := s.nowUTC()

		// Insert silence with fixed start time.
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
			}},
			StartsAt: timestamppb.New(start1.Add(2 * time.Minute)),
			EndsAt:   timestamppb.New(start1.Add(5 * time.Minute)),
		}
		versionBeforeOp := s.Version()
		require.NoError(t, s.Set(t.Context(), sil1))
		require.NotEmpty(t, sil1.Id)
		require.NotEqual(t, versionBeforeOp, s.Version())

		want := state{
			sil1.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil1.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(start1.Add(2 * time.Minute)),
					EndsAt:    timestamppb.New(start1.Add(5 * time.Minute)),
					UpdatedAt: timestamppb.New(start1),
				},
				ExpiresAt: timestamppb.New(start1.Add(5*time.Minute + s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")

		// Insert silence with unset start time. Must be set to now.
		time.Sleep(time.Minute)
		start2 := s.nowUTC()

		sil2 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
			}},
			EndsAt: timestamppb.New(start2.Add(1 * time.Minute)),
		}
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil2))
		require.NotEmpty(t, sil2.Id)
		require.NotEqual(t, versionBeforeOp, s.Version())

		want = state{
			sil1.Id: want[sil1.Id],
			sil2.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil2.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(start2),
					EndsAt:    timestamppb.New(start2.Add(1 * time.Minute)),
					UpdatedAt: timestamppb.New(start2),
				},
				ExpiresAt: timestamppb.New(start2.Add(1*time.Minute + s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")

		// Should be able to update silence without modifications. It is expected to
		// keep the same ID.
		sil3 := cloneSilence(sil2)
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil3))
		require.Equal(t, sil2.Id, sil3.Id)
		require.Equal(t, versionBeforeOp, s.Version())

		// Should be able to update silence with comment. It is also expected to
		// keep the same ID.
		sil4 := cloneSilence(sil3)
		sil4.Comment = "c"
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil4))
		require.Equal(t, sil3.Id, sil4.Id)
		require.Equal(t, versionBeforeOp, s.Version())

		// Extend sil4 to expire at a later time. This should not expire the
		// existing silence, and so should also keep the same ID.
		time.Sleep(time.Minute - time.Nanosecond)
		start5 := s.nowUTC()
		sil5 := cloneSilence(sil4)
		sil5.EndsAt = timestamppb.New(start5.Add(100 * time.Minute))
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil5))
		require.Equal(t, sil4.Id, sil5.Id)
		want = state{
			sil1.Id: want[sil1.Id],
			sil2.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil2.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(start2),
					EndsAt:    timestamppb.New(start5.Add(100 * time.Minute)),
					UpdatedAt: timestamppb.New(start5),
					Comment:   "c",
				},
				ExpiresAt: timestamppb.New(start5.Add(100*time.Minute + s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")
		require.Equal(t, versionBeforeOp, s.Version())

		// Replace the silence sil5 with another silence with different matchers.
		// Unlike previous updates, changing the matchers for an existing silence
		// will expire the existing silence and create a new silence. The new
		// silence is expected to have a different ID to preserve the history of
		// the previous silence.
		time.Sleep(time.Minute)
		start6 := s.nowUTC()

		sil6 := cloneSilence(sil5)
		sil6.MatcherSets = []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "a", Pattern: "c"}},
		}}
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil6))
		require.NotEqual(t, sil5.Id, sil6.Id)
		want = state{
			sil1.Id: want[sil1.Id],
			sil2.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil2.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(start2),
					EndsAt:    timestamppb.New(start6), // Expired
					UpdatedAt: timestamppb.New(start6),
					Comment:   "c",
				},
				ExpiresAt: timestamppb.New(start6.Add(s.retention)),
			},
			sil6.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil6.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "c"}},
					}},
					StartsAt:  timestamppb.New(start6),
					EndsAt:    timestamppb.New(start5.Add(100 * time.Minute)),
					UpdatedAt: timestamppb.New(start6),
					Comment:   "c",
				},
				ExpiresAt: timestamppb.New(start5.Add(100*time.Minute + s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")
		require.NotEqual(t, versionBeforeOp, s.Version())

		// Re-create the silence that we just replaced. Changing the start time,
		// just like changing the matchers, creates a new silence with a different
		// ID. This is again to preserve the history of the original silence.
		time.Sleep(time.Minute)
		start7 := s.nowUTC()
		sil7 := cloneSilence(sil5)
		sil7.StartsAt = timestamppb.New(start1)
		sil7.EndsAt = timestamppb.New(start1.Add(5 * time.Minute))
		versionBeforeOp = s.Version()
		require.NoError(t, s.Set(t.Context(), sil7))
		require.NotEqual(t, sil2.Id, sil7.Id)
		want = state{
			sil1.Id: want[sil1.Id],
			sil2.Id: want[sil2.Id],
			sil6.Id: want[sil6.Id],
			sil7.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil7.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(start7), // New silences have their start time set to "now" when created.
					EndsAt:    timestamppb.New(start1.Add(5 * time.Minute)),
					UpdatedAt: timestamppb.New(start7),
					Comment:   "c",
				},
				ExpiresAt: timestamppb.New(start1.Add(5*time.Minute + s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")
		require.NotEqual(t, versionBeforeOp, s.Version())

		// Updating an existing silence with an invalid silence should not expire
		// the original silence.
		time.Sleep(time.Millisecond)
		sil8 := cloneSilence(sil7)
		sil8.EndsAt = nil // nil represents zero timestamp
		versionBeforeOp = s.Version()
		require.EqualError(t, s.Set(t.Context(), sil8), "invalid silence: invalid zero end timestamp")

		// sil7 should not be expired because the update failed.
		time.Sleep(time.Millisecond)
		sil7, err = s.QueryOne(t.Context(), QIDs(sil7.Id))
		require.NoError(t, err)
		require.Equal(t, SilenceStateActive, StateFromProtoAt(sil7, s.nowUTC()))
		require.Equal(t, versionBeforeOp, s.Version())
	})
}

func TestSetActiveSilence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
		})
		require.NoError(t, err)

		now := time.Now().UTC()

		startsAt := now.Add(-1 * time.Minute)
		endsAt := now.Add(5 * time.Minute)
		// Insert silence with fixed start time.
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
			}},
			StartsAt: timestamppb.New(startsAt),
			EndsAt:   timestamppb.New(endsAt),
		}
		require.NoError(t, s.Set(t.Context(), sil1))

		// Update silence with 2 extra nanoseconds so the "seconds" part should not change

		newStartsAt := now.Add(2 * time.Nanosecond)
		newEndsAt := endsAt.Add(2 * time.Minute)

		sil2 := cloneSilence(sil1)
		sil2.Id = sil1.Id
		sil2.StartsAt = timestamppb.New(newStartsAt)
		sil2.EndsAt = timestamppb.New(newEndsAt)

		time.Sleep(time.Minute)
		now = s.nowUTC()
		require.NoError(t, s.Set(t.Context(), sil2))
		require.Equal(t, sil1.Id, sil2.Id)

		want := state{
			sil2.Id: &pb.MeshSilence{
				Silence: &pb.Silence{
					Id: sil1.Id,
					MatcherSets: []*pb.MatcherSet{{
						Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
					}},
					StartsAt:  timestamppb.New(newStartsAt),
					EndsAt:    timestamppb.New(newEndsAt),
					UpdatedAt: timestamppb.New(now),
				},
				ExpiresAt: timestamppb.New(newEndsAt.Add(s.retention)),
			},
		}
		requireStatesEqual(t, want, s.st, "unexpected state after silence creation")
	})
}

func TestSilencesSetFail(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(t, err)

	now := time.Now()

	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
				}},
				EndsAt: timestamppb.New(now.Add(5 * time.Minute)),
			},
			err: ErrNotFound.Error(),
		}, {
			s:   &pb.Silence{}, // Silence without matcher.
			err: "invalid silence",
		},
	}
	for _, c := range cases {
		checkErr(t, c.err, s.Set(t.Context(), c.s))
	}
}

func TestSilenceCanUpdate(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		a, b *pb.Silence
		ok   bool
	}{
		// Bad arguments.
		{
			a: &pb.Silence{},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now.Add(-time.Minute)),
			},
			ok: false,
		},
		// Expired silence.
		{
			a: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Hour)),
				EndsAt:   timestamppb.New(now.Add(-time.Second)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now),
			},
			ok: false,
		},
		// Pending silences.
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Minute)),
				EndsAt:   timestamppb.New(now.Add(time.Hour)),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(time.Minute)),
				EndsAt:   timestamppb.New(now.Add(time.Minute)),
			},
			ok: true,
		},
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now), // set to exactly start now.
				EndsAt:   timestamppb.New(now.Add(2 * time.Hour)),
			},
			ok: true,
		},
		// Active silences.
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Minute)),
				EndsAt:   timestamppb.New(now.Add(2 * time.Hour)),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Hour)),
				EndsAt:   timestamppb.New(now.Add(-time.Second)),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Hour)),
				EndsAt:   timestamppb.New(now),
			},
			ok: true,
		},
		{
			a: &pb.Silence{
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(2 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			},
			b: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(-time.Hour)),
				EndsAt:   timestamppb.New(now.Add(3 * time.Hour)),
			},
			ok: true,
		},
	}
	for _, c := range cases {
		ok := canUpdate(c.a, c.b, now)
		if ok && !c.ok {
			t.Errorf("expected not-updateable but was: %v, %v", c.a, c.b)
		}
		if !ok && c.ok {
			t.Errorf("expected updateable but was not: %v, %v", c.a, c.b)
		}
	}
}
