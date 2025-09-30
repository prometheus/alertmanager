// Copyright 2016 Prometheus Team
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
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/quartz"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

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

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		options *Options
		err     string
	}{
		{
			options: &Options{
				SnapshotReader: &bytes.Buffer{},
			},
		},
		{
			options: &Options{
				SnapshotFile: "test.bkp",
			},
		},
		{
			options: &Options{
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
		s, err := New(Options{})
		require.NoError(t, err)
		s.clock = quartz.NewMock(t)
		now := s.nowUTC()
		s.st = state{
			"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}, ExpiresAt: now},
			"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}, ExpiresAt: now.Add(-time.Second)},
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: now.Add(time.Second)},
		}
		want := state{
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: now.Add(time.Second)},
		}
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, want, s.st)
	})

	t.Run("GC does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			Matchers: []*pb.Matcher{{
				Type:    pb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: clock.Now(),
			EndsAt:   clock.Now().Add(time.Minute),
		}
		require.NoError(t, s.Set(sil1))
		// Need to query the silence to populate the matcher cache.
		s.Query(QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.st, 1)
		require.Len(t, s.mc, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mc)
	})

	t.Run("replacing a silences does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			Matchers: []*pb.Matcher{{
				Type:    pb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: clock.Now(),
			EndsAt:   clock.Now().Add(time.Minute),
		}
		require.NoError(t, s.Set(sil1))
		// Need to query the silence to populate the matcher cache.
		s.Query(QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.st, 1)
		require.Len(t, s.mc, 1)
		// must clone sil1 before replacing it.
		sil2 := cloneSilence(sil1)
		sil2.Matchers = []*pb.Matcher{{
			Type:    pb.Matcher_EQUAL,
			Name:    "bar",
			Pattern: "baz",
		}}
		require.NoError(t, s.Set(sil2))
		// Need to query the silence to populate the matcher cache.
		s.Query(QMatches(model.LabelSet{"bar": "baz"}))
		require.Len(t, s.st, 2)
		require.Len(t, s.mc, 2)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mc)
	})

	// This test checks for a memory leak that occurred in the matcher cache when
	// updating an existing silence.
	t.Run("updating a silences does not leak cache entries", func(t *testing.T) {
		s, err := New(Options{})
		require.NoError(t, err)
		clock := quartz.NewMock(t)
		s.clock = clock
		sil1 := &pb.Silence{
			Id: "1",
			Matchers: []*pb.Matcher{{
				Type:    pb.Matcher_EQUAL,
				Name:    "foo",
				Pattern: "bar",
			}},
			StartsAt: clock.Now(),
			EndsAt:   clock.Now().Add(time.Minute),
		}
		s.st["1"] = &pb.MeshSilence{Silence: sil1, ExpiresAt: clock.Now().Add(time.Minute)}
		// Need to query the silence to populate the matcher cache.
		s.Query(QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.mc, 1)
		// must clone sil1 before updating it.
		sil2 := cloneSilence(sil1)
		require.NoError(t, s.Set(sil2))
		// The memory leak occurred because updating a silence would add a new
		// entry in the matcher cache even though no new silence was created.
		// This check asserts that this no longer happens.
		s.Query(QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.st, 1)
		require.Len(t, s.mc, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		clock.Advance(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mc)
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
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
							{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
						},
						StartsAt:  now,
						EndsAt:    now,
						UpdatedAt: now,
					},
					ExpiresAt: now,
				},
				{
					Silence: &pb.Silence{
						Id: "3dfb2528-59ce-41eb-b465-f875a4e744a4",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_NOT_EQUAL},
							{Name: "label2", Pattern: "val.+", Type: pb.Matcher_NOT_REGEXP},
						},
						StartsAt:  now,
						EndsAt:    now,
						UpdatedAt: now,
					},
					ExpiresAt: now,
				},
				{
					Silence: &pb.Silence{
						Id: "4b1e760d-182c-4980-b873-c1a6827c9817",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						},
						StartsAt:  now.Add(time.Hour),
						EndsAt:    now.Add(2 * time.Hour),
						UpdatedAt: now,
					},
					ExpiresAt: now.Add(24 * time.Hour),
				},
			},
		},
	}

	for _, c := range cases {
		f, err := os.CreateTemp("", "snapshot")
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
		s2 := &Silences{mc: matcherCache{}, st: state{}}
		err = s2.loadSnapshot(f)
		require.NoError(t, err, "error loading snapshot")
		require.Equal(t, s1.st, s2.st, "state after loading snapshot did not match snapshotted state")

		require.NoError(t, f.Close(), "closing snapshot file failed")
	}
}

// This tests a regression introduced by https://github.com/prometheus/alertmanager/pull/2689.
func TestSilences_Maintenance_DefaultMaintenanceFuncDoesntCrash(t *testing.T) {
	f, err := os.CreateTemp("", "snapshot")
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
	f, err := os.CreateTemp("", "snapshot")
	require.NoError(t, err, "creating temp file failed")
	clock := quartz.NewMock(t)
	reg := prometheus.NewRegistry()
	s := &Silences{st: state{}, logger: promslog.NewNopLogger(), clock: clock}
	s.metrics = newMetrics(reg, s)
	stopc := make(chan struct{})

	var calls atomic.Int32
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Maintenance(10*time.Second, f.Name(), stopc, func() (int64, error) {
			calls.Add(1)
			return 0, nil
		})
	}()
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
		Retention: time.Minute,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock

	nowpb := s.nowUTC()

	sil := &pb.Silence{
		Id:       "some_id",
		Matchers: []*pb.Matcher{{Name: "abc", Pattern: "def"}},
		StartsAt: nowpb,
		EndsAt:   nowpb,
	}

	want := state{
		"some_id": &pb.MeshSilence{
			Silence:   sil,
			ExpiresAt: nowpb.Add(time.Minute),
		},
	}

	done := make(chan struct{})
	s.broadcast = func(b []byte) {
		var e pb.MeshSilence
		r := bytes.NewReader(b)
		_, err := pbutil.ReadDelimited(r, &e)
		require.NoError(t, err)

		require.Equal(t, &e, want["some_id"])
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

	require.Equal(t, want, s.st, "Unexpected silence state")
}

func TestSilenceSet(t *testing.T) {
	s, err := New(Options{
		Retention: time.Hour,
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	start1 := s.nowUTC()

	// Insert silence with fixed start time.
	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: start1.Add(2 * time.Minute),
		EndsAt:   start1.Add(5 * time.Minute),
	}
	versionBeforeOp := s.Version()
	require.NoError(t, s.Set(sil1))
	require.NotEmpty(t, sil1.Id)
	require.NotEqual(t, versionBeforeOp, s.Version())

	want := state{
		sil1.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil1.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  start1.Add(2 * time.Minute),
				EndsAt:    start1.Add(5 * time.Minute),
				UpdatedAt: start1,
			},
			ExpiresAt: start1.Add(5*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Insert silence with unset start time. Must be set to now.
	clock.Advance(time.Minute)
	start2 := s.nowUTC()

	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		EndsAt:   start2.Add(1 * time.Minute),
	}
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil2))
	require.NotEmpty(t, sil2.Id)
	require.NotEqual(t, versionBeforeOp, s.Version())

	want = state{
		sil1.Id: want[sil1.Id],
		sil2.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil2.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  start2,
				EndsAt:    start2.Add(1 * time.Minute),
				UpdatedAt: start2,
			},
			ExpiresAt: start2.Add(1*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Should be able to update silence without modifications. It is expected to
	// keep the same ID.
	sil3 := cloneSilence(sil2)
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil3))
	require.Equal(t, sil2.Id, sil3.Id)
	require.Equal(t, versionBeforeOp, s.Version())

	// Should be able to update silence with comment. It is also expected to
	// keep the same ID.
	sil4 := cloneSilence(sil3)
	sil4.Comment = "c"
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil4))
	require.Equal(t, sil3.Id, sil4.Id)
	require.Equal(t, versionBeforeOp, s.Version())

	// Extend sil4 to expire at a later time. This should not expire the
	// existing silence, and so should also keep the same ID.
	clock.Advance(time.Minute)
	start5 := s.nowUTC()
	sil5 := cloneSilence(sil4)
	sil5.EndsAt = start5.Add(100 * time.Minute)
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil5))
	require.Equal(t, sil4.Id, sil5.Id)
	want = state{
		sil1.Id: want[sil1.Id],
		sil2.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil2.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  start2,
				EndsAt:    start5.Add(100 * time.Minute),
				UpdatedAt: start5,
				Comment:   "c",
			},
			ExpiresAt: start5.Add(100*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")
	require.Equal(t, versionBeforeOp, s.Version())

	// Replace the silence sil5 with another silence with different matchers.
	// Unlike previous updates, changing the matchers for an existing silence
	// will expire the existing silence and create a new silence. The new
	// silence is expected to have a different ID to preserve the history of
	// the previous silence.
	clock.Advance(time.Minute)
	start6 := s.nowUTC()

	sil6 := cloneSilence(sil5)
	sil6.Matchers = []*pb.Matcher{{Name: "a", Pattern: "c"}}
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil6))
	require.NotEqual(t, sil5.Id, sil6.Id)
	want = state{
		sil1.Id: want[sil1.Id],
		sil2.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil2.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  start2,
				EndsAt:    start6, // Expired
				UpdatedAt: start6,
				Comment:   "c",
			},
			ExpiresAt: start6.Add(s.retention),
		},
		sil6.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil6.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "c"}},
				StartsAt:  start6,
				EndsAt:    start5.Add(100 * time.Minute),
				UpdatedAt: start6,
				Comment:   "c",
			},
			ExpiresAt: start5.Add(100*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")
	require.NotEqual(t, versionBeforeOp, s.Version())

	// Re-create the silence that we just replaced. Changing the start time,
	// just like changing the matchers, creates a new silence with a different
	// ID. This is again to preserve the history of the original silence.
	clock.Advance(time.Minute)
	start7 := s.nowUTC()
	sil7 := cloneSilence(sil5)
	sil7.StartsAt = start1
	sil7.EndsAt = start1.Add(5 * time.Minute)
	versionBeforeOp = s.Version()
	require.NoError(t, s.Set(sil7))
	require.NotEqual(t, sil2.Id, sil7.Id)
	want = state{
		sil1.Id: want[sil1.Id],
		sil2.Id: want[sil2.Id],
		sil6.Id: want[sil6.Id],
		sil7.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil7.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  start7, // New silences have their start time set to "now" when created.
				EndsAt:    start1.Add(5 * time.Minute),
				UpdatedAt: start7,
				Comment:   "c",
			},
			ExpiresAt: start1.Add(5*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")
	require.NotEqual(t, versionBeforeOp, s.Version())

	// Updating an existing silence with an invalid silence should not expire
	// the original silence.
	clock.Advance(time.Millisecond)
	sil8 := cloneSilence(sil7)
	sil8.EndsAt = time.Time{}
	versionBeforeOp = s.Version()
	require.EqualError(t, s.Set(sil8), "invalid silence: invalid zero end timestamp")

	// sil7 should not be expired because the update failed.
	clock.Advance(time.Millisecond)
	sil7, err = s.QueryOne(QIDs(sil7.Id))
	require.NoError(t, err)
	require.Equal(t, types.SilenceStateActive, getState(sil7, s.nowUTC()))
	require.Equal(t, versionBeforeOp, s.Version())
}

func TestSilenceLimits(t *testing.T) {
	s, err := New(Options{
		Limits: Limits{
			MaxSilences:         func() int { return 1 },
			MaxSilenceSizeBytes: func() int { return 2 << 11 }, // 4KB
		},
	})
	require.NoError(t, err)

	// Insert sil1 should succeed without error.
	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, s.Set(sil1))

	// Insert sil2 should fail because maximum number of silences has been
	// exceeded.
	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "c", Pattern: "d"}},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(5 * time.Minute),
	}
	require.EqualError(t, s.Set(sil2), "exceeded maximum number of silences: 1 (limit: 1)")

	// Expire sil1 and run the GC. This should allow sil2 to be inserted.
	require.NoError(t, s.Expire(sil1.Id))
	n, err := s.GC()
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, s.Set(sil2))

	// Expire sil2 and run the GC.
	require.NoError(t, s.Expire(sil2.Id))
	n, err = s.GC()
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Insert sil3 should fail because it exceeds maximum size.
	sil3 := &pb.Silence{
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
		CreatedBy: strings.Repeat("i", 2<<9),
		Comment:   strings.Repeat("j", 2<<9),
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(5 * time.Minute),
	}
	require.EqualError(t, s.Set(sil3), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", s.toMeshSilence(sil3).Size()))

	// Should be able to insert sil4.
	sil4 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "k", Pattern: "l"}},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, s.Set(sil4))

	// Should be able to update sil4 without modifications. It is expected to
	// keep the same ID.
	sil5 := cloneSilence(sil4)
	require.NoError(t, s.Set(sil5))
	require.Equal(t, sil4.Id, sil5.Id)

	// Should be able to update the comment. It is also expected to keep the
	// same ID.
	sil6 := cloneSilence(sil5)
	sil6.Comment = "m"
	require.NoError(t, s.Set(sil6))
	require.Equal(t, sil5.Id, sil6.Id)

	// Should not be able to update the start and end time as this requires
	// sil6 to be expired and a new silence to be created. However, this would
	// exceed the maximum number of silences, which counts both active and
	// expired silences.
	sil7 := cloneSilence(sil6)
	sil7.StartsAt = time.Now().Add(5 * time.Minute)
	sil7.EndsAt = time.Now().Add(10 * time.Minute)
	require.EqualError(t, s.Set(sil7), "exceeded maximum number of silences: 1 (limit: 1)")

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, types.SilenceStateActive, getState(sil6, s.nowUTC()))

	// Should not be able to update with a comment that exceeds maximum size.
	// Need to increase the maximum number of silences to test this.
	s.limits.MaxSilences = func() int { return 2 }
	sil8 := cloneSilence(sil6)
	sil8.Comment = strings.Repeat("m", 2<<11)
	require.EqualError(t, s.Set(sil8), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", s.toMeshSilence(sil8).Size()))

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, types.SilenceStateActive, getState(sil6, s.nowUTC()))

	// Should not be able to replace with a silence that exceeds maximum size.
	// This is different from the previous assertion as unlike when adding or
	// updating a comment, changing the matchers for a silence should expire
	// the existing silence, unless the silence that is replacing it exceeds
	// limits, in which case the operation should fail and the existing silence
	// should still be active.
	sil9 := cloneSilence(sil8)
	sil9.Matchers = []*pb.Matcher{{Name: "n", Pattern: "o"}}
	require.EqualError(t, s.Set(sil9), fmt.Sprintf("silence exceeded maximum size: %d bytes (limit: 4096 bytes)", s.toMeshSilence(sil9).Size()))

	// sil6 should not be expired because the update failed.
	sil6, err = s.QueryOne(QIDs(sil6.Id))
	require.NoError(t, err)
	require.Equal(t, types.SilenceStateActive, getState(sil6, s.nowUTC()))
}

func TestSilenceNoLimits(t *testing.T) {
	s, err := New(Options{
		Limits: Limits{},
	})
	require.NoError(t, err)

	// Insert sil should succeed without error.
	sil := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(5 * time.Minute),
		Comment:  strings.Repeat("c", 2<<9),
	}
	require.NoError(t, s.Set(sil))
	require.NotEmpty(t, sil.Id)
}

func TestSetActiveSilence(t *testing.T) {
	s, err := New(Options{
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
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: startsAt,
		EndsAt:   endsAt,
	}
	require.NoError(t, s.Set(sil1))

	// Update silence with 2 extra nanoseconds so the "seconds" part should not change

	newStartsAt := now.Add(2 * time.Nanosecond)
	newEndsAt := endsAt.Add(2 * time.Minute)

	sil2 := cloneSilence(sil1)
	sil2.Id = sil1.Id
	sil2.StartsAt = newStartsAt
	sil2.EndsAt = newEndsAt

	clock.Advance(time.Minute)
	now = s.nowUTC()
	require.NoError(t, s.Set(sil2))
	require.Equal(t, sil1.Id, sil2.Id)

	want := state{
		sil2.Id: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        sil1.Id,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  newStartsAt,
				EndsAt:    newEndsAt,
				UpdatedAt: now,
			},
			ExpiresAt: newEndsAt.Add(s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")
}

func TestSilencesSetFail(t *testing.T) {
	s, err := New(Options{})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock

	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id:       "some_id",
				Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
				EndsAt:   clock.Now().Add(5 * time.Minute),
			},
			err: ErrNotFound.Error(),
		}, {
			s:   &pb.Silence{}, // Silence without matcher.
			err: "invalid silence",
		},
	}
	for _, c := range cases {
		checkErr(t, c.err, s.Set(c.s))
	}
}

func TestQState(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		sil    *pb.Silence
		states []types.SilenceState
		keep   bool
	}{
		{
			sil: &pb.Silence{
				StartsAt: now.Add(time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
			states: []types.SilenceState{types.SilenceStateActive, types.SilenceStateExpired},
			keep:   false,
		},
		{
			sil: &pb.Silence{
				StartsAt: now.Add(time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
			states: []types.SilenceState{types.SilenceStatePending},
			keep:   true,
		},
		{
			sil: &pb.Silence{
				StartsAt: now.Add(time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
			states: []types.SilenceState{types.SilenceStateExpired, types.SilenceStatePending},
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
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
				},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_NOT_EQUAL},
				},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					{Name: "method", Pattern: "POST", Type: pb.Matcher_EQUAL},
				},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					{Name: "method", Pattern: "POST", Type: pb.Matcher_NOT_EQUAL},
				},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_REGEXP},
				},
			},
			drop: true,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_NOT_REGEXP},
				},
			},
			drop: false,
		},
		{
			sil: &pb.Silence{
				Matchers: []*pb.Matcher{
					{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_REGEXP},
					{Name: "path", Pattern: "/nothing/.+", Type: pb.Matcher_REGEXP},
				},
			},
			drop: false,
		},
	}
	for _, c := range cases {
		drop, err := f(c.sil, &Silences{mc: matcherCache{}, st: state{}}, time.Time{})
		require.NoError(t, err)
		require.Equal(t, c.drop, drop, "unexpected filter result")
	}
}

func TestSilencesQuery(t *testing.T) {
	s, err := New(Options{})
	require.NoError(t, err)

	s.st = state{
		"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}},
		"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}},
		"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}},
		"4": &pb.MeshSilence{Silence: &pb.Silence{Id: "4"}},
		"5": &pb.MeshSilence{Silence: &pb.Silence{Id: "5"}},
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
		require.Equal(t, c.exp, res, "unexpected silences in result")
	}
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
				StartsAt: now,
				EndsAt:   now.Add(-time.Minute),
			},
			ok: false,
		},
		// Expired silence.
		{
			a: &pb.Silence{
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(-time.Second),
			},
			b: &pb.Silence{
				StartsAt: now,
				EndsAt:   now,
			},
			ok: false,
		},
		// Pending silences.
		{
			a: &pb.Silence{
				StartsAt:  now.Add(time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  now.Add(time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(time.Minute),
				EndsAt:   now.Add(time.Minute),
			},
			ok: true,
		},
		{
			a: &pb.Silence{
				StartsAt:  now.Add(time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now, // set to exactly start now.
				EndsAt:   now.Add(2 * time.Hour),
			},
			ok: true,
		},
		// Active silences.
		{
			a: &pb.Silence{
				StartsAt:  now.Add(-time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(2 * time.Hour),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  now.Add(-time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(-time.Second),
			},
			ok: false,
		},
		{
			a: &pb.Silence{
				StartsAt:  now.Add(-time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now,
			},
			ok: true,
		},
		{
			a: &pb.Silence{
				StartsAt:  now.Add(-time.Hour),
				EndsAt:    now.Add(2 * time.Hour),
				UpdatedAt: now.Add(-time.Hour),
			},
			b: &pb.Silence{
				StartsAt: now.Add(-time.Hour),
				EndsAt:   now.Add(3 * time.Hour),
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
	s, err := New(Options{Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	now := s.nowUTC()

	m := &pb.Matcher{Type: pb.Matcher_EQUAL, Name: "a", Pattern: "b"}

	s.st = state{
		"pending": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "pending",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}},
		"active": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "active",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}},
		"expired": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "expired",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(-time.Hour),
			EndsAt:    now.Add(-time.Minute),
			UpdatedAt: now.Add(-time.Hour),
		}},
	}

	count, err := s.CountState(types.SilenceStatePending)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	count, err = s.CountState(types.SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	require.NoError(t, s.Expire("pending"))
	require.NoError(t, s.Expire("active"))

	require.NoError(t, s.Expire("expired"))

	sil, err := s.QueryOne(QIDs("pending"))
	require.NoError(t, err)
	require.Equal(t, &pb.Silence{
		Id:        "pending",
		Matchers:  []*pb.Matcher{m},
		StartsAt:  now,
		EndsAt:    now,
		UpdatedAt: now,
	}, sil)

	// Let time pass...
	clock.Advance(time.Second)

	count, err = s.CountState(types.SilenceStatePending)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	count, err = s.CountState(types.SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// Expiring a pending Silence should make the API return the
	// SilenceStateExpired Silence state.
	silenceState := types.CalcSilenceState(sil.StartsAt, sil.EndsAt)
	require.Equal(t, types.SilenceStateExpired, silenceState)

	sil, err = s.QueryOne(QIDs("active"))
	require.NoError(t, err)
	require.Equal(t, &pb.Silence{
		Id:        "active",
		Matchers:  []*pb.Matcher{m},
		StartsAt:  now.Add(-time.Minute),
		EndsAt:    now,
		UpdatedAt: now,
	}, sil)

	sil, err = s.QueryOne(QIDs("expired"))
	require.NoError(t, err)
	require.Equal(t, &pb.Silence{
		Id:        "expired",
		Matchers:  []*pb.Matcher{m},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Hour),
	}, sil)
}

// TestSilenceExpireWithZeroRetention covers the problem that, with zero
// retention time, a silence explicitly set to expired will also immediately
// expire from the silence storage.
func TestSilenceExpireWithZeroRetention(t *testing.T) {
	s, err := New(Options{Retention: 0})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	now := s.nowUTC()

	m := &pb.Matcher{Type: pb.Matcher_EQUAL, Name: "a", Pattern: "b"}

	s.st = state{
		"pending": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "pending",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}},
		"active": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "active",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(-time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-time.Hour),
		}},
		"expired": &pb.MeshSilence{Silence: &pb.Silence{
			Id:        "expired",
			Matchers:  []*pb.Matcher{m},
			StartsAt:  now.Add(-time.Hour),
			EndsAt:    now.Add(-time.Minute),
			UpdatedAt: now.Add(-time.Hour),
		}},
	}

	count, err := s.CountState(types.SilenceStatePending)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	count, err = s.CountState(types.SilenceStateActive)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	count, err = s.CountState(types.SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Advance time. The silence state management code uses update time when
	// merging, and the logic is "first write wins". So we must advance the clock
	// one tick for updates to take effect.
	clock.Advance(1 * time.Millisecond)

	require.NoError(t, s.Expire("pending"))
	require.NoError(t, s.Expire("active"))
	require.NoError(t, s.Expire("expired"))

	// Advance time again. Despite what the function name says, s.Expire() does
	// not expire a silence. It sets the silence to EndAt the current time. This
	// means that the silence is active immediately after calling Expire.
	clock.Advance(1 * time.Millisecond)

	// Verify all silences have expired.
	count, err = s.CountState(types.SilenceStatePending)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	count, err = s.CountState(types.SilenceStateActive)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	count, err = s.CountState(types.SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

// This test checks that invalid silences can be expired.
func TestSilenceExpireInvalid(t *testing.T) {
	s, err := New(Options{Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	s.clock = clock
	now := s.nowUTC()

	// In this test the matcher has an invalid type.
	silence := pb.Silence{
		Id:        "active",
		Matchers:  []*pb.Matcher{{Type: -1, Name: "a", Pattern: "b"}},
		StartsAt:  now.Add(-time.Minute),
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now.Add(-time.Hour),
	}
	// Assert that this silence is invalid.
	require.EqualError(t, validateSilence(&silence), "invalid label matcher 0: unknown matcher type \"-1\"")

	s.st = state{"active": &pb.MeshSilence{Silence: &silence}}

	// The silence should be active.
	count, err := s.CountState(types.SilenceStateActive)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	clock.Advance(time.Millisecond)
	require.NoError(t, s.Expire("active"))
	clock.Advance(time.Millisecond)

	// The silence should be expired.
	count, err = s.CountState(types.SilenceStateActive)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	count, err = s.CountState(types.SilenceStateExpired)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSilencer(t *testing.T) {
	ss, err := New(Options{Retention: time.Hour})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	ss.clock = clock
	now := ss.nowUTC()

	m := types.NewMarker(prometheus.NewRegistry())
	s := NewSilencer(ss, m, promslog.NewNopLogger())

	require.False(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert not silenced without any silences")

	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "foo", Pattern: "baz"}},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(5 * time.Minute),
	}
	require.NoError(t, ss.Set(sil1))

	require.False(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert not silenced by non-matching silence")

	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(5 * time.Minute),
	}
	require.NoError(t, ss.Set(sil2))
	require.NotEmpty(t, sil2.Id)

	require.True(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert silenced by matching silence")

	// One hour passes, silence expires.
	clock.Advance(time.Hour)
	now = ss.nowUTC()

	require.False(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert not silenced by expired silence")

	// Update silence to start in the future.
	err = ss.Set(&pb.Silence{
		Id:       sil2.Id,
		Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
		StartsAt: now.Add(time.Hour),
		EndsAt:   now.Add(3 * time.Hour),
	})
	require.NoError(t, err)

	require.False(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert not silenced by future silence")

	// Two hours pass, silence becomes active.
	clock.Advance(2 * time.Hour)
	now = ss.nowUTC()

	// Exposes issue #2426.
	require.True(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert silenced by activated silence")

	err = ss.Set(&pb.Silence{
		Matchers: []*pb.Matcher{{Name: "foo", Pattern: "b..", Type: pb.Matcher_REGEXP}},
		StartsAt: now.Add(time.Hour),
		EndsAt:   now.Add(3 * time.Hour),
	})
	require.NoError(t, err)

	// Note that issue #2426 doesn't apply anymore because we added a new silence.
	require.True(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert still silenced by activated silence")

	// Two hours pass, first silence expires, overlapping second silence becomes active.
	clock.Advance(2 * time.Hour)

	// Another variant of issue #2426 (overlapping silences).
	require.True(t, s.Mutes(model.LabelSet{"foo": "bar"}), "expected alert silenced by activated second silence")
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
		zeroTimestamp  = time.Time{}
		validTimestamp = now
	)
	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "",
		},
		{
			s: &pb.Silence{
				Id:        "some_id",
				Matchers:  []*pb.Matcher{},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "at least one matcher required",
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
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
				},
				StartsAt:  now,
				EndsAt:    now.Add(-time.Second),
				UpdatedAt: validTimestamp,
			},
			err: "end time must not be before start time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
				},
				StartsAt:  zeroTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero start timestamp",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
				},
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
			Silence:   &pb.Silence{Id: id, UpdatedAt: ts},
			ExpiresAt: exp,
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
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
							{Name: "label2", Pattern: "val.+", Type: pb.Matcher_REGEXP},
						},
						StartsAt:  now,
						EndsAt:    now,
						UpdatedAt: now,
					},
					ExpiresAt: now,
				},
				{
					Silence: &pb.Silence{
						Id: "4b1e760d-182c-4980-b873-c1a6827c9817",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						},
						StartsAt:  now.Add(time.Hour),
						EndsAt:    now.Add(2 * time.Hour),
						UpdatedAt: now,
					},
					ExpiresAt: now.Add(24 * time.Hour),
				},
				{
					Silence: &pb.Silence{
						Id: "3dfb2528-59ce-41eb-b465-f875a4e744a4",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_NOT_EQUAL},
							{Name: "label2", Pattern: "val.+", Type: pb.Matcher_NOT_REGEXP},
						},
						StartsAt:  now,
						EndsAt:    now,
						UpdatedAt: now,
					},
					ExpiresAt: now,
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

		require.Equal(t, in, out, "decoded data doesn't match encoded data")
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
