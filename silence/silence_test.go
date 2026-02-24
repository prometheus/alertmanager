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
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/quartz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

func checkErr(t *testing.T, expected string, got error) {
	t.Helper()

	if expected == "" {
		require.NoError(t, got)
		return
	}

	if got == nil {
		t.Errorf("expected error containing %q but got none", expected)
		return
	}

	require.Contains(t, got.Error(), expected)
}

// requireStatesEqual compares two silence states using proto.Equal for proper protobuf comparison.
func requireStatesEqual(t *testing.T, expected, actual state, msgAndArgs ...any) {
	t.Helper()
	require.Len(t, actual, len(expected), msgAndArgs...)
	for id, expectedSil := range expected {
		actualSil, ok := actual[id]
		require.True(t, ok, "silence %s missing from actual state", id)
		require.True(t, proto.Equal(expectedSil, actualSil), "silence %s mismatch: expected %v, got %v", id, expectedSil, actualSil)
	}
}

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		options *Options
		err     string
	}{
		{
			options: &Options{
				Metrics:        prometheus.NewRegistry(),
				SnapshotReader: &bytes.Buffer{},
			},
		},
		{
			options: &Options{
				Metrics:      prometheus.NewRegistry(),
				SnapshotFile: "test.bkp",
			},
		},
		{
			options: &Options{
				Metrics:        prometheus.NewRegistry(),
				SnapshotFile:   "test bkp",
				SnapshotReader: &bytes.Buffer{},
			},
			err: "only one of SnapshotFile and SnapshotReader must be set",
		},
	}

	for _, c := range cases {
		checkErr(t, c.err, c.options.validate())
	}
}

func TestSilenceGCOverTime(t *testing.T) {
	t.Run("GC does not remove active silences", func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		s.clock = quartz.NewMock(t)
		now := s.nowUTC()
		initialState := state{
			"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}, ExpiresAt: timestamppb.New(now)},
			"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}, ExpiresAt: timestamppb.New(now.Add(-time.Second))},
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: timestamppb.New(now.Add(time.Second))},
		}
		for _, sil := range initialState {
			s.st[sil.Silence.Id] = sil
			s.indexSilence(sil.Silence)
		}
		want := state{
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: timestamppb.New(now.Add(time.Second))},
		}
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		requireStatesEqual(t, want, s.st)
	})

	t.Run("GC does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(clock.Now()),
			EndsAt:   timestamppb.New(clock.Now().Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), sil1))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	t.Run("replacing a silences does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(clock.Now()),
			EndsAt:   timestamppb.New(clock.Now().Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), sil1))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// must clone sil1 before replacing it.
		sil2 := cloneSilence(sil1)
		sil2.MatcherSets = []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{
				Type:    pb.Matcher_EQUAL,
				Name:    "bar",
				Pattern: "baz",
			}},
		}}
		require.NoError(t, s.Set(t.Context(), sil2))
		require.Len(t, s.st, 2)
		require.Len(t, s.mi, 2)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	// This test checks for a memory leak that occurred in the matcher cache when
	// updating an existing silence.
	t.Run("updating a silence does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			Id: "1",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(clock.Now()),
			EndsAt:   timestamppb.New(clock.Now().Add(time.Minute)),
		}
		s.st["1"] = &pb.MeshSilence{Silence: sil1, ExpiresAt: timestamppb.New(clock.Now().Add(time.Minute))}
		s.indexSilence(sil1)
		require.Len(t, s.mi, 1)
		// must clone sil1 before updating it.
		sil2 := cloneSilence(sil1)
		require.NoError(t, s.Set(t.Context(), sil2))
		// The memory leak occurred because updating a silence would add a new
		// entry in the matcher cache even though no new silence was created.
		// This check asserts that this no longer happens.
		s.Query(t.Context(), QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	t.Run("GC collects silences in multiple rounds", func(t *testing.T) {
		s, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
		})
		clock := quartz.NewMock(t)
		s.clock = clock
		require.NoError(t, err)
		now := s.nowUTC().UTC()

		matcher := &pb.Matcher{
			Type:    pb.Matcher_EQUAL,
			Name:    "job",
			Pattern: "test",
		}

		// Create silences that expire at different times.
		// Directly set them in state to create pre-expired silences.
		// Group 1: expires at now+30min (with retention: now+90min)
		// Group 2: expires at now+45min (with retention: now+105min)
		// Group 3: expires at now+60min (with retention: now+120min)
		// Group 4: active, expires at now+3hours (with retention: now+4hours)

		sils := make([]*pb.Silence, 0, 60)
		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group1-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(30 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group2-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(45 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group3-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(60 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 30 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("active-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(3 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		// Shuffle silences to ensure GC order is not dependent on insertion order.
		rand.Shuffle(len(sils), func(i, j int) {
			sils[i], sils[j] = sils[j], sils[i]
		})
		for _, sil := range sils {
			ms := s.toMeshSilence(sil)
			s.st[ms.Silence.Id] = ms
			s.indexSilence(ms.Silence)
		}

		require.Len(t, s.st, 60)
		require.Len(t, s.mi, 60)

		// First GC: nothing should be collected yet
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 0, n)
		require.Len(t, s.st, 60)
		require.Len(t, s.mi, 60)

		// Advance time to 91 minutes - Group 1 should be GC'd
		clock.Advance(91 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 50)
		require.Len(t, s.mi, 50)

		// Advance time to 106 minutes - Group 2 should be GC'd
		clock.Advance(15 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 40)
		require.Len(t, s.mi, 40)

		// Advance time to 121 minutes - Group 3 should be GC'd
		clock.Advance(15 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 30)
		require.Len(t, s.mi, 30)

		// Verify all remaining silences are active
		for id := range s.st {
			require.Contains(t, id, "active-")
		}
	})

	t.Run("GC continues and removes erroneous silences", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		s, err := New(Options{Metrics: reg})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		now := clock.Now()

		// Create a valid silence
		validSil := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), validSil))
		validID := validSil.Id

		// Manually add an erroneous silence with zero expiration
		erroneousSil := &pb.MeshSilence{
			Silence: &pb.Silence{
				Id: "erroneous",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{{
						Type:    pb.Matcher_EQUAL,
						Name:    "bar",
						Pattern: "baz",
					}},
				}},
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now.Add(time.Minute)),
			},
			ExpiresAt: nil, // Zero expiration - invalid
		}
		s.st["erroneous"] = erroneousSil
		s.vi.add(s.version+1, "erroneous")
		s.version++

		// Manually add an entry to version index that doesn't exist in state
		s.vi.add(s.version+1, "missing")
		s.version++

		require.Len(t, s.st, 2)
		require.Len(t, s.vi, 3)

		// Run GC - should continue despite errors
		n, err := s.GC()
		require.Error(t, err)
		require.Contains(t, err.Error(), "zero expiration timestamp")
		require.Contains(t, err.Error(), "missing from state")

		// GC should have removed erroneous silences
		require.Equal(t, 1, n) // Only the erroneous silence with zero expiration
		require.Len(t, s.st, 1)
		require.Len(t, s.vi, 1)
		require.Contains(t, s.st, validID)
		require.NotContains(t, s.st, "erroneous")

		// Check that the error metric was incremented
		metricValue := testutil.ToFloat64(s.metrics.gcErrorsTotal)
		require.Equal(t, float64(2), metricValue)
	})
}

func TestSilencesSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	now := quartz.NewMock(t).Now().UTC()

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
}

// This tests a regression introduced by https://github.com/prometheus/alertmanager/pull/2689.
func TestSilences_Maintenance_DefaultMaintenanceFuncDoesntCrash(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "snapshot")
	require.NoError(t, err, "creating temp file failed")
	clock := quartz.NewMock(t)
	s := &Silences{st: state{}, logger: promslog.NewNopLogger(), clock: clock, metrics: newMetrics(nil, nil)}
	stopc := make(chan struct{})

	done := make(chan struct{})
	go func() {
		s.Maintenance(100*time.Millisecond, f.Name(), stopc, nil)
		close(done)
	}()
	runtime.Gosched()

	clock.Advance(100 * time.Millisecond)
	close(stopc)

	<-done
}

func TestSilences_Maintenance_SupportsCustomCallback(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "snapshot")
	require.NoError(t, err, "creating temp file failed")
	clock := quartz.NewMock(t)
	reg := prometheus.NewRegistry()
	s := &Silences{st: state{}, logger: promslog.NewNopLogger(), clock: clock}
	s.metrics = newMetrics(reg, s)
	stopc := make(chan struct{})

	var calls atomic.Int32
	var wg sync.WaitGroup

	wg.Go(func() {
		s.Maintenance(10*time.Second, f.Name(), stopc, func() (int64, error) {
			calls.Add(1)
			return 0, nil
		})
	})
	gosched()

	// Before the first tick, no maintenance executed.
	clock.Advance(9 * time.Second)
	require.EqualValues(t, 0, calls.Load())

	// Tick once.
	clock.Advance(1 * time.Second)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 5*time.Second, time.Second)

	// Stop the maintenance loop. We should get exactly one more execution of the maintenance func.
	close(stopc)
	wg.Wait()

	require.EqualValues(t, 2, calls.Load())

	// Check the maintenance metrics.
	require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
# HELP alertmanager_silences_maintenance_errors_total How many maintenances were executed for silences that failed.
# TYPE alertmanager_silences_maintenance_errors_total counter
alertmanager_silences_maintenance_errors_total 0
# HELP alertmanager_silences_maintenance_total How many maintenances were executed for silences.
# TYPE alertmanager_silences_maintenance_total counter
alertmanager_silences_maintenance_total 2
`), "alertmanager_silences_maintenance_total", "alertmanager_silences_maintenance_errors_total"))
}

func TestSilencesSetSilence(t *testing.T) {
	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Minute,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock

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
		require.NoError(t, s.setSilence(s.toMeshSilence(sil), nowpb))
	}()

	// Ensure broadcast was called.
	if _, isOpen := <-done; isOpen {
		t.Fatal("broadcast was not called")
	}

	requireStatesEqual(t, want, s.st, "Unexpected silence state")
}

func TestSilenceSet(t *testing.T) {
	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Hour,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
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
	clock.Advance(time.Minute)
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
	clock.Advance(time.Minute)
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
	clock.Advance(time.Minute)
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
	clock.Advance(time.Minute)
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
	clock.Advance(time.Millisecond)
	sil8 := cloneSilence(sil7)
	sil8.EndsAt = nil // nil represents zero timestamp
	versionBeforeOp = s.Version()
	require.EqualError(t, s.Set(t.Context(), sil8), "invalid silence: invalid zero end timestamp")

	// sil7 should not be expired because the update failed.
	clock.Advance(time.Millisecond)
	sil7, err = s.QueryOne(t.Context(), QIDs(sil7.Id))
	require.NoError(t, err)
	require.Equal(t, SilenceStateActive, getState(sil7, s.nowUTC()))
	require.Equal(t, versionBeforeOp, s.Version())
}

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
	require.Equal(t, SilenceStateActive, getState(sil6, s.nowUTC()))

	// Should not be able to update with a comment that exceeds maximum size.
	// Need to increase the maximum number of silences to test this.
	s.limits.MaxSilences = func() int { return 2 }
	sil8 := cloneSilence(sil6)
	sil8.Comment = strings.Repeat("m", 2<<11)
	require.EqualError(t, s.Set(t.Context(), sil8), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", proto.Size(s.toMeshSilence(sil8))))

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(t.Context(), QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, SilenceStateActive, getState(sil6, s.nowUTC()))

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
	require.Equal(t, SilenceStateActive, getState(sil6, s.nowUTC()))
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

func TestSetActiveSilence(t *testing.T) {
	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Hour,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	now := clock.Now()

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

	clock.Advance(time.Minute)
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
}

func TestSilencesSetFail(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock

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
				EndsAt: timestamppb.New(clock.Now().Add(5 * time.Minute)),
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

func TestQState(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		sil    *pb.Silence
		states []SilenceState
		keep   bool
	}{
		{
			sil: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(time.Minute)),
				EndsAt:   timestamppb.New(now.Add(time.Hour)),
			},
			states: []SilenceState{SilenceStateActive, SilenceStateExpired},
			keep:   false,
		},
		{
			sil: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(time.Minute)),
				EndsAt:   timestamppb.New(now.Add(time.Hour)),
			},
			states: []SilenceState{SilenceStatePending},
			keep:   true,
		},
		{
			sil: &pb.Silence{
				StartsAt: timestamppb.New(now.Add(time.Minute)),
				EndsAt:   timestamppb.New(now.Add(time.Hour)),
			},
			states: []SilenceState{SilenceStateExpired, SilenceStatePending},
			keep:   true,
		},
	}
	for i, c := range cases {
		q := &query{}
		QState(c.states...)(q)
		f := q.filters[0]

		keep, err := f(c.sil, nil, now)
		require.NoError(t, err)
		require.Equal(t, c.keep, keep, "unexpected filter result for case %d", i)
	}
}

func TestQMatches(t *testing.T) {
	qp := QMatches(model.LabelSet{
		"job":      "test",
		"instance": "web-1",
		"path":     "/user/profile",
		"method":   "GET",
	})

	q := &query{}
	qp(q)
	f := q.filters[0]

	cases := []struct {
		sil  *pb.Silence
		drop bool
	}{
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					},
				}},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "job", Pattern: "test", Type: pb.Matcher_NOT_EQUAL},
					},
				}},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
						{Name: "method", Pattern: "POST", Type: pb.Matcher_EQUAL},
					},
				}},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
						{Name: "method", Pattern: "POST", Type: pb.Matcher_NOT_EQUAL},
					},
				}},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_REGEXP},
					},
				}},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{
					{
						Matchers: []*pb.Matcher{
							{
								Name: "path", Pattern: "/user/.+", Type: pb.Matcher_NOT_REGEXP,
							},
						},
					},
				},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{
					{
						Matchers: []*pb.Matcher{
							{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_REGEXP},
							{Name: "path", Pattern: "/nothing/.+", Type: pb.Matcher_REGEXP},
						},
					},
				},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET", Type: pb.Matcher_NOT_EQUAL},
						},
					},
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET|POST", Type: pb.Matcher_REGEXP},
							{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
						},
					},
				},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET", Type: pb.Matcher_EQUAL},
						},
					},
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET|POST", Type: pb.Matcher_REGEXP},
							{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
						},
					},
				},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				MatcherSets: []*pb.MatcherSet{
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET", Type: pb.Matcher_NOT_EQUAL},
						},
					},
					{
						Matchers: []*pb.Matcher{
							{Name: "method", Pattern: "GET|POST", Type: pb.Matcher_REGEXP},
							{Name: "job", Pattern: "test", Type: pb.Matcher_NOT_EQUAL},
						},
					},
				},
			},
			drop: false,
		},
	}
	for _, c := range cases {
		silences := &Silences{mi: matcherIndex{}, st: state{}}
		silences.mi.add(c.sil)
		drop, err := f(c.sil, silences, time.Time{})
		require.NoError(t, err)
		require.Equal(t, c.drop, drop, "unexpected filter result")
	}
}

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

func TestQSince(t *testing.T) {
	type testCase struct {
		index versionIndex

		since   int
		results []string
	}

	cases := map[string]testCase{
		"skips current version": {
			index: versionIndex{
				{id: "1", version: 1},
				{id: "2", version: 2},
			},

			since:   1,
			results: []string{"2"},
		},
		"skips any number of old versions": {
			index: versionIndex{
				{id: "1", version: 1},
				{id: "2", version: 2},
				{id: "3", version: 2},
				{id: "4", version: 3},
				{id: "5", version: 4},
			},

			since:   3,
			results: []string{"5"},
		},
		"since 0 returns everything": {
			index: versionIndex{
				{id: "1", version: 1},
				{id: "2", version: 2},
			},

			since:   0,
			results: []string{"1", "2"},
		},
		"returns all elements of a group with the same version": {
			index: versionIndex{
				{id: "1", version: 1},
				{id: "2", version: 2},
				{id: "3", version: 3},
				{id: "4", version: 3},
			},

			since:   2,
			results: []string{"3", "4"},
		},
		"returns everything after the provided version": {
			index: versionIndex{
				{id: "1", version: 1},
				{id: "2", version: 2},
				{id: "3", version: 3},
				{id: "4", version: 3},
				{id: "5", version: 4},
				{id: "6", version: 5},
			},

			since:   2,
			results: []string{"3", "4", "5", "6"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			silences, err := New(Options{Metrics: prometheus.NewRegistry()})
			require.NoError(t, err)
			// build state from index so test cases are easier to write
			st := state{}
			for _, mapping := range c.index {
				st[mapping.id] = &pb.MeshSilence{Silence: &pb.Silence{Id: mapping.id}}
			}
			silences.st = st
			silences.vi = c.index

			res, _, err := silences.Query(t.Context(), QSince(c.since))
			require.NoError(t, err)
			resultIds := []string{}
			for _, sil := range res {
				resultIds = append(resultIds, sil.Id)
			}

			sort.StringSlice(c.results).Sort()
			sort.StringSlice(resultIds).Sort()

			require.Equal(t, c.results, resultIds)
		})
	}
}

func TestSilencesQuery(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(t, err)

	s.st = state{
		"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}},
		"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}},
		"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}},
		"4": &pb.MeshSilence{Silence: &pb.Silence{Id: "4"}},
		"5": &pb.MeshSilence{Silence: &pb.Silence{Id: "5"}},
	}
	s.vi = versionIndex{
		{id: "1"},
		{id: "2"},
		{id: "3"},
		{id: "4"},
		{id: "5"},
	}
	cases := []struct {
		q   *query
		exp []*pb.Silence
	}{
		{
			// Default query of retrieving all silences.
			q: &query{},
			exp: []*pb.Silence{
				{Id: "1"},
				{Id: "2"},
				{Id: "3"},
				{Id: "4"},
				{Id: "5"},
			},
		},
		{
			// Retrieve by IDs.
			q: &query{
				ids: []string{"2", "5"},
			},
			exp: []*pb.Silence{
				{Id: "2"},
				{Id: "5"},
			},
		},
		{
			// Retrieve all and filter
			q: &query{
				filters: []silenceFilter{
					func(sil *pb.Silence, _ *Silences, _ time.Time) (bool, error) {
						return sil.Id == "1" || sil.Id == "2", nil
					},
				},
			},
			exp: []*pb.Silence{
				{Id: "1"},
				{Id: "2"},
			},
		},
		{
			// Retrieve by IDs and filter
			q: &query{
				ids: []string{"2", "5"},
				filters: []silenceFilter{
					func(sil *pb.Silence, _ *Silences, _ time.Time) (bool, error) {
						return sil.Id == "1" || sil.Id == "2", nil
					},
				},
			},
			exp: []*pb.Silence{
				{Id: "2"},
			},
		},
	}

	for _, c := range cases {
		// Run default query of retrieving all silences.
		res, _, err := s.query(c.q, time.Time{})
		require.NoError(t, err, "unexpected error on querying")

		// Currently there are no sorting guarantees in the querying API.
		sort.Sort(silencesByID(c.exp))
		sort.Sort(silencesByID(res))
		for i := range c.exp {
			require.True(t, proto.Equal(c.exp[i], res[i]), "unexpected silence in result")
		}
	}
}

func TestQIDs(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry()})
	require.NoError(t, err)

	s.st = state{
		"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}},
		"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}},
		"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}},
		"4": &pb.MeshSilence{Silence: &pb.Silence{Id: "4"}},
	}

	// Test QIDs with empty arguments returns an error
	_, _, err = s.Query(t.Context(), QIDs())
	require.Error(t, err, "expected error when QIDs is called with no arguments")
	require.Contains(t, err.Error(), "QIDs filter must have at least one id")

	// Test QIDs with empty arguments returns an error via QueryOne
	_, err = s.QueryOne(t.Context(), QIDs())
	require.Error(t, err, "expected error when QIDs is called with no arguments")
	require.Contains(t, err.Error(), "QIDs filter must have at least one id")

	// Test QIDs with single ID works
	res, _, err := s.Query(t.Context(), QIDs("1"))
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.Equal(t, "1", res[0].Id)

	// Test QIDs with multiple IDs works
	res, _, err = s.Query(t.Context(), QIDs("1", "2"))
	require.NoError(t, err)
	require.Len(t, res, 2)

	// Test QueryOne with single ID works
	sil, err := s.QueryOne(t.Context(), QIDs("1"))
	require.NoError(t, err)
	require.Equal(t, "1", sil.Id)
}

type silencesByID []*pb.Silence

func (s silencesByID) Len() int           { return len(s) }
func (s silencesByID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s silencesByID) Less(i, j int) bool { return s[i].Id < s[j].Id }

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
		if ok && !c.ok {
			t.Errorf("expected updateable but was not: %v, %v", c.a, c.b)
		}
	}
}

func TestSilenceExpire(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
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
	clock.Advance(time.Second)

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
}

// TestSilenceExpireWithZeroRetention covers the problem that, with zero
// retention time, a silence explicitly set to expired will also immediately
// expire from the silence storage.
func TestSilenceExpireWithZeroRetention(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: 0})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
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
	clock.Advance(1 * time.Millisecond)

	require.NoError(t, s.Expire(t.Context(), "pending"))
	require.NoError(t, s.Expire(t.Context(), "active"))
	require.NoError(t, s.Expire(t.Context(), "expired"))

	// Advance time again. Despite what the function name says, s.Expire() does
	// not expire a silence. It sets the silence to EndAt the current time. This
	// means that the silence is active immediately after calling Expire.
	clock.Advance(1 * time.Millisecond)

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
}

// This test checks that invalid silences can be expired.
func TestSilenceExpireInvalid(t *testing.T) {
	s, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
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

	clock.Advance(time.Millisecond)
	require.NoError(t, s.Expire(t.Context(), "active"))
	clock.Advance(time.Millisecond)

	// The silence should be expired.
	count, err = s.CountState(t.Context(), SilenceStateActive)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	count, err = s.CountState(t.Context(), SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSilencer(t *testing.T) {
	ss, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	ss.clock = clock
	now := ss.nowUTC()

	m := types.NewMarker(prometheus.NewRegistry())
	s := NewSilencer(ss, m, promslog.NewNopLogger())

	require.False(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert not silenced without any silences")

	sil1 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "foo", Pattern: "baz"}},
		}},
		StartsAt: timestamppb.New(now.Add(-time.Hour)),
		EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
	}
	require.NoError(t, ss.Set(t.Context(), sil1))

	require.False(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert not silenced by non-matching silence")

	sil2 := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
		}},
		StartsAt: timestamppb.New(now.Add(-time.Hour)),
		EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
	}
	require.NoError(t, ss.Set(t.Context(), sil2))
	require.NotEmpty(t, sil2.Id)

	require.True(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert silenced by matching silence")

	// One hour passes, silence expires.
	clock.Advance(time.Hour)
	now = ss.nowUTC()

	require.False(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert not silenced by expired silence")

	// Update silence to start in the future.
	err = ss.Set(t.Context(), &pb.Silence{
		Id: sil2.Id,
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
		}},
		StartsAt: timestamppb.New(now.Add(time.Hour)),
		EndsAt:   timestamppb.New(now.Add(3 * time.Hour)),
	})
	require.NoError(t, err)

	require.False(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert not silenced by future silence")

	// Two hours pass, silence becomes active.
	clock.Advance(2 * time.Hour)
	now = ss.nowUTC()

	// Exposes issue #2426.
	require.True(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert silenced by activated silence")

	err = ss.Set(t.Context(), &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "foo", Pattern: "b..", Type: pb.Matcher_REGEXP}},
		}},
		StartsAt: timestamppb.New(now.Add(time.Hour)),
		EndsAt:   timestamppb.New(now.Add(3 * time.Hour)),
	})
	require.NoError(t, err)

	// Note that issue #2426 doesn't apply anymore because we added a new silence.
	require.True(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert still silenced by activated silence")

	// Two hours pass, first silence expires, overlapping second silence becomes active.
	clock.Advance(2 * time.Hour)

	// Another variant of issue #2426 (overlapping silences).
	require.True(t, s.Mutes(t.Context(), model.LabelSet{"foo": "bar"}), "expected alert silenced by activated second silence")
}

func TestSilencerPostDeleteEvictsCache(t *testing.T) {
	ss, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	ss.clock = clock
	now := ss.nowUTC()

	m := types.NewMarker(prometheus.NewRegistry())
	s := NewSilencer(ss, m, promslog.NewNopLogger())

	lset := model.LabelSet{"foo": "bar"}
	fp := lset.Fingerprint()

	// Create a matching silence.
	sil := &pb.Silence{
		MatcherSets: []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
		}},
		StartsAt: timestamppb.New(now.Add(-time.Hour)),
		EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
	}
	require.NoError(t, ss.Set(t.Context(), sil))

	// Mutes populates the cache.
	require.True(t, s.Mutes(t.Context(), lset))
	entry := s.cache.get(fp)
	require.Positive(t, entry.count(), "cache should have entries after Mutes()")

	// PostGC evicts the cache entry for this fingerprint.
	s.PostGC(model.Fingerprints{fp})
	entry = s.cache.get(fp)
	require.Equal(t, 0, entry.count(), "cache should be empty after PostGC()")
	require.Equal(t, 0, entry.version, "version should be zero for evicted entry")

	// Mutes re-evaluates from scratch (cache miss) and still finds the silence.
	require.True(t, s.Mutes(t.Context(), lset), "expected alert still silenced after cache eviction")
	entry = s.cache.get(fp)
	require.Positive(t, entry.count(), "cache should be repopulated after Mutes()")

	// Expire the silence, advance time so it's truly expired.
	clock.Advance(time.Hour)

	// PostGC for a different fingerprint should not affect this entry.
	otherLset := model.LabelSet{"other": "alert"}
	s.PostGC(model.Fingerprints{otherLset.Fingerprint()})
	entry = s.cache.get(fp)
	require.Positive(t, entry.count(), "unrelated PostGC should not evict other entries")
}

func TestValidateClassicMatcher(t *testing.T) {
	cases := []struct {
		m   *pb.Matcher
		err string
	}{
		{
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "00",
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label name",
		}, {
			m: &pb.Matcher{
				Name:    "\xf0\x9f\x99\x82", // U+1F642
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label name",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "((",
				Type:    pb.Matcher_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "))",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xff",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label value",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xf0\x9f\x99\x82", // U+1F642
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    333,
			},
			err: "unknown matcher type",
		},
	}

	for _, c := range cases {
		checkErr(t, c.err, validateMatcher(c.m))
	}
}

func TestValidateUTF8Matcher(t *testing.T) {
	cases := []struct {
		m   *pb.Matcher
		err string
	}{
		{
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "00",
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "\xf0\x9f\x99\x82", // U+1F642
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "((",
				Type:    pb.Matcher_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "))",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xff",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label value",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xf0\x9f\x99\x82", // U+1F642
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    333,
			},
			err: "unknown matcher type",
		},
	}

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	for _, c := range cases {
		checkErr(t, c.err, validateMatcher(c.m))
	}
}

func TestValidateSilence(t *testing.T) {
	var (
		now            = time.Now().UTC()
		zeroTimestamp  *timestamppb.Timestamp // nil represents zero timestamp
		validTimestamp = timestamppb.New(now)
	)
	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "matcher set 0 is empty",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
					{Name: "00", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid label matcher",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: ""},
					{Name: "b", Pattern: ".*", Type: pb.Matcher_REGEXP},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "at least one matcher must not match the empty string",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  timestamppb.New(now),
				EndsAt:    timestamppb.New(now.Add(-time.Second)),
				UpdatedAt: validTimestamp,
			},
			err: "end time must not be before start time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  zeroTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero start timestamp",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    zeroTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero end timestamp",
		},
	}
	for _, c := range cases {
		checkErr(t, c.err, validateSilence(c.s))
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

// runtime.Gosched() does not "suspend" the current goroutine so there's no guarantee that the main goroutine won't
// be able to continue. For more see https://pkg.go.dev/runtime#Gosched.
func gosched() {
	time.Sleep(1 * time.Millisecond)
}

func TestSilenceAnnotations(t *testing.T) {
	s, err := New(Options{
		Metrics:   prometheus.NewRegistry(),
		Retention: time.Hour,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	now := s.nowUTC()

	// Create a silence with annotations
	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "job", Pattern: "test"}},
		StartsAt: timestamppb.New(now),
		EndsAt:   timestamppb.New(now.Add(time.Hour)),
		Annotations: map[string]string{
			"ticket": "JIRA-123",
			"type":   "planned",
			"test":   "integration",
		},
	}

	// Set the silence via the API
	require.NoError(t, s.Set(t.Context(), sil1))
	require.NotEmpty(t, sil1.Id)

	// Query the silence back by ID
	queriedSil, err := s.QueryOne(t.Context(), QIDs(sil1.Id))
	require.NoError(t, err)

	// Verify all annotations are returned correctly
	require.NotNil(t, queriedSil.Annotations)
	require.Equal(t, "JIRA-123", queriedSil.Annotations["ticket"])
	require.Equal(t, "planned", queriedSil.Annotations["type"])
	require.Equal(t, "integration", queriedSil.Annotations["test"])

	// Test querying all silences
	allSils, _, err := s.Query(t.Context())
	require.NoError(t, err)
	require.Len(t, allSils, 1)
	require.Equal(t, queriedSil.Annotations, allSils[0].Annotations)

	// Create a second silence with different annotations
	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "job", Pattern: "frontend"}},
		StartsAt: timestamppb.New(now),
		EndsAt:   timestamppb.New(now.Add(time.Hour)),
		Annotations: map[string]string{
			"ticket": "JIRA-456",
		},
	}
	require.NoError(t, s.Set(t.Context(), sil2))

	// Query by state and verify both silences have their annotations
	activeSils, _, err := s.Query(t.Context(), QState(SilenceStateActive))
	require.NoError(t, err)
	require.Len(t, activeSils, 2)

	for _, sil := range activeSils {
		require.NotNil(t, sil.Annotations)
		switch sil.Id {
		case sil1.Id:
			require.Len(t, sil.Annotations, 3)
			require.Equal(t, "JIRA-123", sil.Annotations["ticket"])
		case sil2.Id:
			require.Len(t, sil.Annotations, 1)
			require.Equal(t, "JIRA-456", sil.Annotations["ticket"])
		default:
			t.Fatalf("unexpected silence ID: %s", sil.Id)
		}
	}

	// Test updating a silence with new annotations
	clock.Advance(time.Minute)
	sil1Updated := &pb.Silence{
		Id:       sil1.Id,
		Matchers: []*pb.Matcher{{Name: "job", Pattern: "test"}},
		StartsAt: sil1.StartsAt,
		EndsAt:   sil1.EndsAt,
		Annotations: map[string]string{
			"ticket": "JIRA-123",
			"type":   "emergency", // changed
			"test":   "load",      // changed
		},
	}
	require.NoError(t, s.Set(t.Context(), sil1Updated))

	// Query back and verify annotations were updated
	queriedUpdated, err := s.QueryOne(t.Context(), QIDs(sil1.Id))
	require.NoError(t, err)
	require.Len(t, queriedUpdated.Annotations, 3)
	require.Equal(t, "emergency", queriedUpdated.Annotations["type"])
	require.Equal(t, "load", queriedUpdated.Annotations["test"])

	// Test silence with nil annotations
	sil3 := &pb.Silence{
		Matchers:    []*pb.Matcher{{Name: "job", Pattern: "backend"}},
		StartsAt:    timestamppb.New(now),
		EndsAt:      timestamppb.New(now.Add(time.Hour)),
		Annotations: nil,
	}
	require.NoError(t, s.Set(t.Context(), sil3))
	queriedSil3, err := s.QueryOne(t.Context(), QIDs(sil3.Id))
	require.NoError(t, err)
	// nil annotations should be preserved or converted to empty map
	if queriedSil3.Annotations != nil {
		require.Empty(t, queriedSil3.Annotations)
	}

	// Test silence with empty annotations map
	sil4 := &pb.Silence{
		Matchers:    []*pb.Matcher{{Name: "job", Pattern: "database"}},
		StartsAt:    timestamppb.New(now),
		EndsAt:      timestamppb.New(now.Add(time.Hour)),
		Annotations: map[string]string{},
	}
	require.NoError(t, s.Set(t.Context(), sil4))
	queriedSil4, err := s.QueryOne(t.Context(), QIDs(sil4.Id))
	require.NoError(t, err)
	if queriedSil4.Annotations != nil {
		require.Empty(t, queriedSil4.Annotations)
	}
}
