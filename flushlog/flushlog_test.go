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

package flushlog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pb "github.com/prometheus/alertmanager/flushlog/flushlogpb"

	"github.com/benbjohnson/clock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestLogGC(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()
	// We only care about key names and expiration timestamps.
	newFlushLog := func(ts time.Time) *pb.MeshFlushLog {
		return &pb.MeshFlushLog{
			ExpiresAt: ts,
		}
	}

	l := &FlushLog{
		st: state{
			1: newFlushLog(now),
			2: newFlushLog(now.Add(time.Second)),
			3: newFlushLog(now.Add(-time.Second)),
		},
		clock:   mockClock,
		metrics: newMetrics(nil),
	}
	n, err := l.GC()
	require.NoError(t, err, "unexpected error in garbage collection")
	require.Equal(t, 2, n, "unexpected number of removed entries")

	expected := state{
		2: newFlushLog(now.Add(time.Second)),
	}
	require.Equal(t, expected, l.st, "unexpected state after garbage collection")
}

func TestLogDelete(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()
	newFlushLog := func(fp uint64, ts, exp time.Time) *pb.MeshFlushLog {
		return &pb.MeshFlushLog{
			FlushLog: &pb.FlushLog{
				GroupFingerprint: fp,
				Timestamp:        ts,
			},
			ExpiresAt: exp,
		}
	}

	l := &FlushLog{
		st: state{
			1: newFlushLog(1, now, now.Add(time.Hour)),
			2: newFlushLog(2, now, now.Add(time.Second)),
		},
		clock:     mockClock,
		metrics:   newMetrics(nil),
		broadcast: func([]byte) {},
	}
	err := l.Delete(1)
	require.NoError(t, err, "unexpected delete error")

	// Tombstone retained in state with the original FlushLog.Timestamp and
	// ExpiresAt zeroed; entry 2 untouched.
	expected := state{
		1: newFlushLog(1, now, time.Time{}),
		2: newFlushLog(2, now, now.Add(time.Second)),
	}
	require.Equal(t, expected, l.st, "unexpected state after delete")
}

func TestLogSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	mockClock := clock.NewMock()
	now := mockClock.Now().UTC()

	cases := []struct {
		entries []*pb.MeshFlushLog
	}{
		{
			entries: []*pb.MeshFlushLog{
				{
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 1,
						Timestamp:        now,
					},
					ExpiresAt: now,
				},
				{
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 2,
						Timestamp:        now,
					},
					ExpiresAt: now,
				},
				{
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 3,
						Timestamp:        now,
					},
					ExpiresAt: now,
				},
			},
		},
	}

	for _, c := range cases {
		f, err := os.CreateTemp("", "snapshot")
		require.NoError(t, err, "creating temp file failed")

		l1 := &FlushLog{
			st:      state{},
			metrics: newMetrics(nil),
		}
		// Setup internal state manually.
		for i, e := range c.entries {
			l1.st[uint64(i+1)] = e
		}
		_, err = l1.Snapshot(f)
		require.NoError(t, err, "creating snapshot failed")
		require.NoError(t, f.Close(), "closing snapshot file failed")

		f, err = os.Open(f.Name())
		require.NoError(t, err, "opening snapshot file failed")

		// Check again against new nlog instance.
		l2 := &FlushLog{}
		err = l2.loadSnapshot(f)
		require.NoError(t, err, "error loading snapshot")
		require.Equal(t, l1.st, l2.st, "state after loading snapshot did not match snapshotted state")

		require.NoError(t, f.Close(), "closing snapshot file failed")
	}
}

func TestWithMaintenance_SupportsCustomCallback(t *testing.T) {
	f, err := os.CreateTemp("", "snapshot")
	require.NoError(t, err, "creating temp file failed")
	stopc := make(chan struct{})
	reg := prometheus.NewPedanticRegistry()
	opts := Options{
		Metrics:      reg,
		SnapshotFile: f.Name(),
	}

	l, err := New(opts)
	clock := clock.NewMock()
	l.clock = clock
	require.NoError(t, err)

	var calls atomic.Int32
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		l.Maintenance(100*time.Millisecond, f.Name(), stopc, func() (int64, error) {
			calls.Add(1)
			return 0, nil
		})
	}()
	gosched()

	// Before the first tick, no maintenance executed.
	clock.Add(99 * time.Millisecond)
	require.EqualValues(t, 0, calls.Load())

	// Tick once.
	clock.Add(1 * time.Millisecond)
	require.EqualValues(t, 1, calls.Load())

	// Stop the maintenance loop. We should get exactly one more execution of the maintenance func.
	close(stopc)
	wg.Wait()

	require.EqualValues(t, 2, calls.Load())
	// Check the maintenance metrics.
	require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
# HELP alertmanager_flushlog_maintenance_errors_total How many maintenances were executed for the flush log that failed.
# TYPE alertmanager_flushlog_maintenance_errors_total counter
alertmanager_flushlog_maintenance_errors_total 0
# HELP alertmanager_flushlog_maintenance_total How many maintenances were executed for the flush log.
# TYPE alertmanager_flushlog_maintenance_total counter
alertmanager_flushlog_maintenance_total 2
`), "alertmanager_flushlog_maintenance_total", "alertmanager_flushlog_maintenance_errors_total"))
}

func TestReplaceFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "replace_file")
	require.NoError(t, err, "creating temp dir failed")

	origFilename := filepath.Join(dir, "testfile")

	of, err := os.Create(origFilename)
	require.NoError(t, err, "creating file failed")

	nf, err := openReplace(origFilename)
	require.NoError(t, err, "opening replacement file failed")

	_, err = nf.Write([]byte("test"))
	require.NoError(t, err, "writing replace file failed")

	require.NotEqual(t, nf.Name(), of.Name(), "replacement file must have different name while editing")
	require.NoError(t, nf.Close(), "closing replacement file failed")
	require.NoError(t, of.Close(), "closing original file failed")

	ofr, err := os.Open(origFilename)
	require.NoError(t, err, "opening original file failed")
	defer ofr.Close()

	res, err := io.ReadAll(ofr)
	require.NoError(t, err, "reading original file failed")
	require.Equal(t, "test", string(res), "unexpected file contents")
}

func TestStateMerge(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()

	// We only care about key names and timestamps for the
	// merging logic.
	newFlushLog := func(fp uint64, ts, exp time.Time) *pb.MeshFlushLog {
		return &pb.MeshFlushLog{
			FlushLog: &pb.FlushLog{
				GroupFingerprint: fp,
				Timestamp:        ts,
			},
			ExpiresAt: exp,
		}
	}

	exp := now.Add(time.Minute)

	cases := []struct {
		name  string
		a, b  state
		final state
	}{
		{
			name: "multiple operations",
			a: state{
				1: newFlushLog(1, now, exp),
				2: newFlushLog(2, now, exp),
				3: newFlushLog(3, now, exp),
				6: newFlushLog(6, now, exp),
			},
			b: state{
				2: newFlushLog(2, now.Add(-time.Minute), exp),                        // older timestamp, should be dropped
				3: newFlushLog(3, now.Add(time.Minute), exp),                         // newer timestamp, should overwrite
				4: newFlushLog(4, now, exp),                                          // new key, should be added
				5: newFlushLog(5, now.Add(-time.Minute), now.Add(-time.Millisecond)), // new key, expired, should not be added
				6: newFlushLog(6, now.Add(time.Minute), time.Time{}),                 // tombstone, overwrites entry as tombstone
			},
			final: state{
				1: newFlushLog(1, now, exp),
				2: newFlushLog(2, now, exp),
				3: newFlushLog(3, now.Add(time.Minute), exp),
				4: newFlushLog(4, now, exp),
				6: newFlushLog(6, now.Add(time.Minute), time.Time{}), // tombstone retained
			},
		},
		{
			name: "marks tombstone when expiration is zero and timestamp is after previous entry",
			a: state{
				1: newFlushLog(1, now, exp),
			},
			b: state{
				1: newFlushLog(1, now.Add(time.Minute), time.Time{}),
			},
			final: state{
				1: newFlushLog(1, now.Add(time.Minute), time.Time{}),
			},
		},
		{
			name: "doesn't tombstone when timestamp is before previous entry",
			a: state{
				1: newFlushLog(1, now, exp),
			},
			b: state{
				1: newFlushLog(1, now.Add(time.Minute*-1), time.Time{}),
			},
			final: state{
				1: newFlushLog(1, now, exp),
			},
		},
		{
			name: "doesn't resurrect entry after tombstone with same timestamp",
			a: state{
				1: newFlushLog(1, now, time.Time{}), // tombstone already in state
			},
			b: state{
				1: newFlushLog(1, now, exp), // stale refresh broadcast — same Timestamp T
			},
			final: state{
				1: newFlushLog(1, now, time.Time{}), // tombstone preserved
			},
		},
		{
			name: "newer flush overrides tombstone",
			a: state{
				1: newFlushLog(1, now, time.Time{}),
			},
			b: state{
				1: newFlushLog(1, now.Add(time.Minute), exp), // newer flush — Timestamp T2 > T1
			},
			final: state{
				1: newFlushLog(1, now.Add(time.Minute), exp),
			},
		},
		{
			name: "older entry doesn't override tombstone with same timestamp",
			a: state{
				1: newFlushLog(1, now, time.Time{}),
			},
			b: state{
				1: newFlushLog(1, now.Add(-time.Minute), exp),
			},
			final: state{
				1: newFlushLog(1, now, time.Time{}),
			},
		},
		{
			name: "stores tombstone with no prev to reject future stale refreshes",
			a:    state{},
			b: state{
				1: newFlushLog(1, now, time.Time{}),
			},
			final: state{
				1: newFlushLog(1, now, time.Time{}),
			},
		},
		{
			name: "newer tombstone overrides older tombstone",
			a: state{
				1: newFlushLog(1, now, time.Time{}),
			},
			b: state{
				1: newFlushLog(1, now.Add(time.Minute), time.Time{}),
			},
			final: state{
				1: newFlushLog(1, now.Add(time.Minute), time.Time{}),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ca, cb := c.a.clone(), c.b.clone()

			res := c.a.clone()
			for _, e := range cb {
				res.merge(e, now)
			}
			require.Equal(t, c.final, res, "Merge result should match expectation")
			require.Equal(t, c.b, cb, "Merged state should remain unmodified")
			require.Equal(t, c.a, ca, "Merge should not change original state")
		})
	}
}

// TestStateMergeGossipPropagation asserts that merge() returns true/false in
// the right cases so that ping-pong gossip on tombstones doesn't occur.
func TestStateMergeGossipPropagation(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()

	newFlushLog := func(fp uint64, ts, exp time.Time) *pb.MeshFlushLog {
		return &pb.MeshFlushLog{
			FlushLog: &pb.FlushLog{
				GroupFingerprint: fp,
				Timestamp:        ts,
			},
			ExpiresAt: exp,
		}
	}
	exp := now.Add(time.Minute)

	cases := []struct {
		name   string
		prev   state
		in     *pb.MeshFlushLog
		merged bool
	}{
		{
			name:   "tombstone over entry with same timestamp: merged once",
			prev:   state{1: newFlushLog(1, now, exp)},
			in:     newFlushLog(1, now, time.Time{}),
			merged: true,
		},
		{
			name:   "tombstone equal to stored tombstone: not re-gossiped",
			prev:   state{1: newFlushLog(1, now, time.Time{})},
			in:     newFlushLog(1, now, time.Time{}),
			merged: false,
		},
		{
			name:   "older tombstone vs stored tombstone: not re-gossiped",
			prev:   state{1: newFlushLog(1, now, time.Time{})},
			in:     newFlushLog(1, now.Add(-time.Minute), time.Time{}),
			merged: false,
		},
		{
			name:   "stale refresh after tombstone: not re-gossiped",
			prev:   state{1: newFlushLog(1, now, time.Time{})},
			in:     newFlushLog(1, now, exp),
			merged: false,
		},
		{
			name:   "newer flush after tombstone: merged",
			prev:   state{1: newFlushLog(1, now, time.Time{})},
			in:     newFlushLog(1, now.Add(time.Minute), exp),
			merged: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.prev.clone()
			got := s.merge(c.in, now)
			require.Equal(t, c.merged, got, "unexpected merge return value")
		})
	}
}

func TestStateDataCoding(t *testing.T) {
	// Check whether encoding and decoding the data is symmetric.
	mockClock := clock.NewMock()
	now := mockClock.Now().UTC()

	cases := []struct {
		entries []*pb.MeshFlushLog
	}{
		{
			entries: []*pb.MeshFlushLog{
				{
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 1,
						Timestamp:        now,
					},
					ExpiresAt: now,
				}, {
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 2,
						Timestamp:        now,
					},
					ExpiresAt: now,
				}, {
					FlushLog: &pb.FlushLog{
						GroupFingerprint: 3,
						Timestamp:        now,
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
			in[e.FlushLog.GroupFingerprint] = e
		}
		msg, err := in.MarshalBinary()
		require.NoError(t, err)

		out, err := decodeState(bytes.NewReader(msg))
		require.NoError(t, err, "decoding message failed")

		require.Equal(t, in, out, "decoded data doesn't match encoded data")
	}
}

func TestQuery(t *testing.T) {
	opts := Options{Retention: time.Second}
	nl, err := New(opts)
	if err != nil {
		require.NoError(t, err, "constructing flushlog failed")
	}
	clock := clock.NewMock()
	// logTS := clock.Now()
	nl.clock = clock

	// no entry
	_, err = nl.Query(1)
	require.EqualError(t, err, "not found")

	now := time.Now()
	err = nl.Log(1, now, now, 0)
	require.NoError(t, err, "logging flush failed")

	entries, err := nl.Query(1)
	require.NoError(t, err, "querying flushlog failed")
	entry := entries[0]
	require.Equal(t, uint64(1), entry.GroupFingerprint, "unexpected group fingerprint")
	require.Equal(t, now, entry.Timestamp, "unexpected group fingerprint")
}

// TestQuery_Tombstone asserts that Query treats a tombstone-marked entry as
// not-found so callers can't observe a phantom flushlog entry.
func TestQuery_Tombstone(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()

	l := &FlushLog{
		clock:   mockClock,
		metrics: newMetrics(nil),
		st: state{
			1: &pb.MeshFlushLog{
				FlushLog: &pb.FlushLog{
					GroupFingerprint: 1,
					Timestamp:        now,
				},
				ExpiresAt: time.Time{},
			},
		},
	}
	_, err := l.Query(1)
	require.ErrorIs(t, err, ErrNotFound, "Query must hide tombstones from callers")
}

// TestLog_AfterTombstone asserts that calling Log() when the previous entry
// is a tombstone produces a fresh entry with the new flushTime and a valid
// ExpiresAt — i.e. the tombstone doesn't sink the new flush via the
// closeToExpiry / Timestamp-carry-over branch.
func TestLog_AfterTombstone(t *testing.T) {
	mockClock := clock.NewMock()
	t1 := mockClock.Now()
	mockClock.Add(time.Hour)
	t2 := mockClock.Now()

	var broadcasts [][]byte
	l := &FlushLog{
		clock:     mockClock,
		retention: 24 * time.Hour,
		logger:    nil,
		metrics:   newMetrics(nil),
		broadcast: func(b []byte) { broadcasts = append(broadcasts, b) },
		st: state{
			1: &pb.MeshFlushLog{
				FlushLog: &pb.FlushLog{
					GroupFingerprint: 1,
					Timestamp:        t1,
				},
				ExpiresAt: time.Time{}, // tombstone
			},
		},
	}

	err := l.Log(1, t2, t2.Add(time.Hour), 0)
	require.NoError(t, err)

	got, ok := l.st[1]
	require.True(t, ok, "expected fresh entry in state after Log post-tombstone")
	require.False(t, got.ExpiresAt.IsZero(), "expected real entry, got tombstone")
	require.Equal(t, t2, got.FlushLog.Timestamp, "expected fresh Timestamp (t2), not tombstone's Timestamp (t1)")
	require.Len(t, broadcasts, 1, "expected exactly one broadcast")
}

// TestQuery_PointerStability asserts that a *pb.FlushLog returned by
// Query is not mutated by a subsequent Delete. Query releases the read
// lock before the caller dereferences fields; mutating the in-map
// pointer (rather than replacing it) would race on time.Time's
// multi-word value.
func TestQuery_PointerStability(t *testing.T) {
	mockClock := clock.NewMock()
	t1 := mockClock.Now()

	l := &FlushLog{
		clock:     mockClock,
		retention: 24 * time.Hour,
		metrics:   newMetrics(nil),
		broadcast: func([]byte) {},
		st: state{
			1: &pb.MeshFlushLog{
				FlushLog: &pb.FlushLog{
					GroupFingerprint: 1,
					Timestamp:        t1,
				},
				ExpiresAt: t1.Add(time.Hour),
			},
		},
	}

	entries, err := l.Query(1)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	held := entries[0]
	require.Equal(t, t1, held.Timestamp)

	// Advance the clock and delete. This would bump the in-map Timestamp
	// to the new now if Delete mutated the existing pointer in place.
	mockClock.Add(100 * time.Hour)
	require.NoError(t, l.Delete(1))

	require.Equal(t, t1, held.Timestamp, "pointer returned by Query must not be mutated by Delete")
}

// TestLogDelete_LongLivedEntry asserts that Delete bumps the tombstone's
// Timestamp to the deletion time when the underlying entry has been
// refreshed for longer than retention. Without the bump, the tombstone
// would be swept by GC immediately (Timestamp + retention is already in
// the past), opening a resurrection window for in-flight refresh
// broadcasts.
func TestLogDelete_LongLivedEntry(t *testing.T) {
	mockClock := clock.NewMock()
	t1 := mockClock.Now()
	retention := 24 * time.Hour
	mockClock.Add(100 * time.Hour)
	deletionTime := mockClock.Now()

	l := &FlushLog{
		clock:     mockClock,
		retention: retention,
		metrics:   newMetrics(nil),
		broadcast: func([]byte) {},
		st: state{
			1: &pb.MeshFlushLog{
				FlushLog: &pb.FlushLog{
					GroupFingerprint: 1,
					Timestamp:        t1, // original flush, well outside retention
				},
				ExpiresAt: deletionTime.Add(retention), // recently refreshed
			},
		},
	}

	err := l.Delete(1)
	require.NoError(t, err)

	tomb, ok := l.st[1]
	require.True(t, ok, "tombstone must be retained in state")
	require.True(t, tomb.ExpiresAt.IsZero(), "ExpiresAt must be zero")
	require.Equal(t, deletionTime, tomb.FlushLog.Timestamp, "Timestamp must be bumped to deletion time")

	// GC right after delete must not sweep the tombstone.
	n, err := l.GC()
	require.NoError(t, err)
	require.Equal(t, 0, n, "tombstone must not be swept immediately after delete")
	require.Contains(t, l.st, uint64(1))

	// Advance past retention from deletion; tombstone is now eligible for sweep.
	mockClock.Add(retention + time.Second)
	n, err = l.GC()
	require.NoError(t, err)
	require.Equal(t, 1, n, "tombstone must be swept once retention since delete has elapsed")
	require.NotContains(t, l.st, uint64(1))
}

// TestLogGC_Tombstones asserts GC retains tombstones until
// FlushLog.Timestamp + retention has passed, then sweeps them.
func TestLogGC_Tombstones(t *testing.T) {
	mockClock := clock.NewMock()
	now := mockClock.Now()
	retention := time.Hour

	entry := func(fp uint64, ts, exp time.Time) *pb.MeshFlushLog {
		return &pb.MeshFlushLog{
			FlushLog: &pb.FlushLog{
				GroupFingerprint: fp,
				Timestamp:        ts,
			},
			ExpiresAt: exp,
		}
	}

	l := &FlushLog{
		clock:     mockClock,
		retention: retention,
		metrics:   newMetrics(nil),
		st: state{
			1: entry(1, now, now.Add(time.Second)),                    // entry, ExpiresAt still in future — keep
			2: entry(2, now.Add(-2*time.Hour), now.Add(-time.Second)), // entry, expired — sweep
			3: entry(3, now, time.Time{}),                             // tombstone, Timestamp+retention in future — keep
			4: entry(4, now.Add(-2*retention), time.Time{}),           // tombstone, Timestamp+retention in past — sweep
		},
	}

	n, err := l.GC()
	require.NoError(t, err)
	require.Equal(t, 2, n, "expected two entries swept (one expired entry, one expired tombstone)")
	require.Contains(t, l.st, uint64(1), "fresh entry must survive GC")
	require.Contains(t, l.st, uint64(3), "fresh tombstone must survive GC")
	require.NotContains(t, l.st, uint64(2), "expired entry must be swept")
	require.NotContains(t, l.st, uint64(4), "expired tombstone must be swept")
}

func TestStateDecodingError(t *testing.T) {
	// Check whether decoding copes with erroneous data.
	s := state{1: &pb.MeshFlushLog{}}

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
