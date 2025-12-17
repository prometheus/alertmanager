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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestLogGC(t *testing.T) {
	// mockClock removed
	now := time.Now()
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
		// clock removed
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
	// mockClock removed
	now := time.Now()
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
		},
		// clock removed
		metrics:   newMetrics(nil),
		broadcast: func([]byte) {},
	}
	err := l.Delete(1)
	require.NoError(t, err, "unexpected delete error")

	expected := state{
		2: newFlushLog(now.Add(time.Second)),
	}
	require.Equal(t, expected, l.st, "unexpected state after garbage collection")
}

func TestLogSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	// mockClock removed
	now := time.Now().UTC()

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
	require.NoError(t, err)

	var calls atomic.Int32
	var wg sync.WaitGroup

	wg.Go(func() {
		l.Maintenance(100*time.Millisecond, f.Name(), stopc, func() (int64, error) {
			calls.Add(1)
			return 0, nil
		})
	})
	gosched()

	// Before the first tick, no maintenance executed.
	time.Sleep(99 * time.Millisecond)
	require.EqualValues(t, 0, calls.Load())

	// Tick once.
	time.Sleep(1 * time.Millisecond)
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
	// mockClock removed
	now := time.Now()

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
		a, b  state
		final state
	}{
		{
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
				6: newFlushLog(6, now, time.Time{}),                                  // zero expiration, should be deleted
			},
			final: state{
				1: newFlushLog(1, now, exp),
				2: newFlushLog(2, now, exp),
				3: newFlushLog(3, now.Add(time.Minute), exp),
				4: newFlushLog(4, now, exp),
			},
		},
	}

	for _, c := range cases {
		ca, cb := c.a.clone(), c.b.clone()

		res := c.a.clone()
		for _, e := range cb {
			res.merge(e, now)
		}
		require.Equal(t, c.final, res, "Merge result should match expectation")
		require.Equal(t, c.b, cb, "Merged state should remain unmodified")
		require.NotEqual(t, c.final, ca, "Merge should not change original state")
	}
}

func TestStateDataCoding(t *testing.T) {
	// Check whether encoding and decoding the data is symmetric.
	// mockClock removed
	now := time.Now().UTC()

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

	// no entry
	_, err = nl.Query(1)
	require.EqualError(t, err, "not found")

	now := time.Now()
	err = nl.Log(1, now, 0)
	require.NoError(t, err, "logging flush failed")

	entries, err := nl.Query(1)
	require.NoError(t, err, "querying flushlog failed")
	entry := entries[0]
	require.Equal(t, uint64(1), entry.GroupFingerprint, "unexpected group fingerprint")
	require.Equal(t, now, entry.Timestamp, "unexpected group fingerprint")
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
