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

package nflog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/prometheus/alertmanager/nflog/nflogpb"

	"github.com/coder/quartz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLogGC(t *testing.T) {
	mockClock := quartz.NewMock(t)
	now := mockClock.Now()
	// We only care about key names and expiration timestamps.
	newEntry := func(ts time.Time) *pb.MeshEntry {
		return &pb.MeshEntry{
			ExpiresAt: timestamppb.New(ts),
		}
	}

	l := &Log{
		st: state{
			"a1": newEntry(now),
			"a2": newEntry(now.Add(time.Second)),
			"a3": newEntry(now.Add(-time.Second)),
		},
		clock:   mockClock,
		metrics: newMetrics(prometheus.NewRegistry()),
	}
	n, err := l.GC()
	require.NoError(t, err, "unexpected error in garbage collection")
	require.Equal(t, 2, n, "unexpected number of removed entries")

	expected := state{
		"a2": newEntry(now.Add(time.Second)),
	}
	require.Equal(t, expected, l.st, "unexpected state after garbage collection")
}

func TestLogSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	mockClock := quartz.NewMock(t)
	now := mockClock.Now().UTC()

	cases := []struct {
		entries []*pb.MeshEntry
	}{
		{
			entries: []*pb.MeshEntry{
				{
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "abc", Integration: "test1", Idx: 1},
						GroupHash: []byte("126a8a51b9d1bbd07fddc65819a542c3"),
						Resolved:  false,
						Timestamp: timestamppb.New(now),
					},
					ExpiresAt: timestamppb.New(now),
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f8abce7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "def", Integration: "test2", Idx: 29},
						GroupHash: []byte("122c2331b9d1bbd07fddc65819a542c3"),
						Resolved:  true,
						Timestamp: timestamppb.New(now),
					},
					ExpiresAt: timestamppb.New(now),
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("aaaaaca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "ghi", Integration: "test3", Idx: 0},
						GroupHash: []byte("126a8a51b9d1bbd07fddc6e3e3e542c3"),
						Resolved:  false,
						Timestamp: timestamppb.New(now),
					},
					ExpiresAt: timestamppb.New(now),
				},
			},
		},
	}

	for _, c := range cases {
		f, err := os.CreateTemp(t.TempDir(), "snapshot")
		require.NoError(t, err, "creating temp file failed")

		l1 := &Log{
			st:      state{},
			metrics: newMetrics(nil),
		}
		// Setup internal state manually.
		for _, e := range c.entries {
			l1.st[stateKey(string(e.Entry.GroupKey), e.Entry.Receiver)] = e
		}
		_, err = l1.Snapshot(f)
		require.NoError(t, err, "creating snapshot failed")
		require.NoError(t, f.Close(), "closing snapshot file failed")

		f, err = os.Open(f.Name())
		require.NoError(t, err, "opening snapshot file failed")

		// Check again against new nlog instance.
		l2 := &Log{}
		err = l2.loadSnapshot(f)
		require.NoError(t, err, "error loading snapshot")

		for id, expected := range l1.st {
			actual, ok := l2.st[id]
			require.True(t, ok, "silence %s missing from decoded state", id)
			require.True(t, proto.Equal(expected, actual), "silence %s mismatch after decoding", id)
		}

		require.NoError(t, f.Close(), "closing snapshot file failed")
	}
}

func TestWithMaintenance_SupportsCustomCallback(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "snapshot")
	require.NoError(t, err, "creating temp file failed")
	stopc := make(chan struct{})
	reg := prometheus.NewPedanticRegistry()
	opts := Options{
		Metrics:      reg,
		SnapshotFile: f.Name(),
	}

	l, err := New(opts)
	clock := quartz.NewMock(t)
	l.clock = clock
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
	clock.Advance(99 * time.Millisecond)
	require.EqualValues(t, 0, calls.Load())

	// Tick once.
	clock.Advance(1 * time.Millisecond)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 5*time.Second, time.Second)

	// Stop the maintenance loop. We should get exactly one more execution of the maintenance func.
	close(stopc)
	wg.Wait()

	require.EqualValues(t, 2, calls.Load())
	// Check the maintenance metrics.
	require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
# HELP alertmanager_nflog_maintenance_errors_total How many maintenances were executed for the notification log that failed.
# TYPE alertmanager_nflog_maintenance_errors_total counter
alertmanager_nflog_maintenance_errors_total 0
# HELP alertmanager_nflog_maintenance_total How many maintenances were executed for the notification log.
# TYPE alertmanager_nflog_maintenance_total counter
alertmanager_nflog_maintenance_total 2
`), "alertmanager_nflog_maintenance_total", "alertmanager_nflog_maintenance_errors_total"))
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
	mockClock := quartz.NewMock(t)
	now := mockClock.Now()

	// We only care about key names and timestamps for the
	// merging logic.
	newEntry := func(name string, ts, exp time.Time) *pb.MeshEntry {
		return &pb.MeshEntry{
			Entry: &pb.Entry{
				Timestamp: timestamppb.New(ts),
				GroupKey:  []byte("key"),
				Receiver: &pb.Receiver{
					GroupName:   name,
					Idx:         1,
					Integration: "integr",
				},
			},
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
				"key:a1/integr/1": newEntry("a1", now, exp),
				"key:a2/integr/1": newEntry("a2", now, exp),
				"key:a3/integr/1": newEntry("a3", now, exp),
			},
			b: state{
				"key:b1/integr/1": newEntry("b1", now, exp),                                          // new key, should be added
				"key:b2/integr/1": newEntry("b2", now.Add(-time.Minute), now.Add(-time.Millisecond)), // new key, expired, should not be added
				"key:a2/integr/1": newEntry("a2", now.Add(-time.Minute), exp),                        // older timestamp, should be dropped
				"key:a3/integr/1": newEntry("a3", now.Add(time.Minute), exp),                         // newer timestamp, should overwrite
			},
			final: state{
				"key:a1/integr/1": newEntry("a1", now, exp),
				"key:a2/integr/1": newEntry("a2", now, exp),
				"key:a3/integr/1": newEntry("a3", now.Add(time.Minute), exp),
				"key:b1/integr/1": newEntry("b1", now, exp),
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
	mockClock := quartz.NewMock(t)
	now := mockClock.Now().UTC()

	cases := []struct {
		entries []*pb.MeshEntry
	}{
		{
			entries: []*pb.MeshEntry{
				{
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "abc", Integration: "test1", Idx: 1},
						GroupHash: []byte("126a8a51b9d1bbd07fddc65819a542c3"),
						Resolved:  false,
						Timestamp: timestamppb.New(now),
					},
					ExpiresAt: timestamppb.New(now),
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f8abce7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "def", Integration: "test2", Idx: 29},
						GroupHash: []byte("122c2331b9d1bbd07fddc65819a542c3"),
						Resolved:  true,
						Timestamp: timestamppb.New(now),
					},
					ExpiresAt: timestamppb.New(now),
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("aaaaaca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "ghi", Integration: "test3", Idx: 0},
						GroupHash: []byte("126a8a51b9d1bbd07fddc6e3e3e542c3"),
						Resolved:  false,
						Timestamp: timestamppb.New(now),
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
			in[stateKey(string(e.Entry.GroupKey), e.Entry.Receiver)] = e
		}
		msg, err := in.MarshalBinary()
		require.NoError(t, err)

		out, err := decodeState(bytes.NewReader(msg))
		require.NoError(t, err, "decoding message failed")

		for id, expected := range in {
			actual, ok := out[id]
			require.True(t, ok, "silence %s missing from decoded state", id)
			require.True(t, proto.Equal(expected, actual), "silence %s mismatch after decoding", id)
		}
	}
}

func TestQuery(t *testing.T) {
	opts := Options{Metrics: prometheus.NewRegistry(), Retention: time.Second}
	nl, err := New(opts)
	if err != nil {
		require.NoError(t, err, "constructing nflog failed")
	}

	recv := new(pb.Receiver)

	// no key param
	_, err = nl.Query(QGroupKey("key"))
	require.EqualError(t, err, "no query parameters specified")

	// no recv param
	_, err = nl.Query(QReceiver(recv))
	require.EqualError(t, err, "no query parameters specified")

	// no entry
	_, err = nl.Query(QGroupKey("nonexistentkey"), QReceiver(recv))
	require.EqualError(t, err, "not found")

	// existing entry
	firingAlerts := []uint64{1, 2, 3}
	resolvedAlerts := []uint64{4, 5}

	err = nl.Log(recv, "key", firingAlerts, resolvedAlerts, nil, 0)
	require.NoError(t, err, "logging notification failed")

	entries, err := nl.Query(QGroupKey("key"), QReceiver(recv))
	require.NoError(t, err, "querying nflog failed")
	entry := entries[0]
	require.Equal(t, firingAlerts, entry.FiringAlerts)
	require.Equal(t, resolvedAlerts, entry.ResolvedAlerts)
}

func TestStateDecodingError(t *testing.T) {
	// Check whether decoding copes with erroneous data.
	s := state{"": &pb.MeshEntry{}}

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
