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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/stretchr/testify/require"
)

func TestNlogGC(t *testing.T) {
	now := utcNow()
	// We only care about key names and expiration timestamps.
	newEntry := func(ts time.Time) *pb.MeshEntry {
		return &pb.MeshEntry{
			ExpiresAt: ts,
		}
	}

	l := &nlog{
		st: gossipData{
			"a1": newEntry(now),
			"a2": newEntry(now.Add(time.Second)),
			"a3": newEntry(now.Add(-time.Second)),
		},
		now:     func() time.Time { return now },
		metrics: newMetrics(nil),
	}
	n, err := l.GC()
	require.NoError(t, err, "unexpected error in garbage collection")
	require.Equal(t, 2, n, "unexpected number of removed entries")

	expected := gossipData{
		"a2": newEntry(now.Add(time.Second)),
	}
	require.Equal(t, l.st, expected, "unepexcted state after garbage collection")
}

func TestNlogSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	now := utcNow()

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
						Timestamp: now,
					},
					ExpiresAt: now,
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f8abce7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "def", Integration: "test2", Idx: 29},
						GroupHash: []byte("122c2331b9d1bbd07fddc65819a542c3"),
						Resolved:  true,
						Timestamp: now,
					},
					ExpiresAt: now,
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("aaaaaca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "ghi", Integration: "test3", Idx: 0},
						GroupHash: []byte("126a8a51b9d1bbd07fddc6e3e3e542c3"),
						Resolved:  false,
						Timestamp: now,
					},
					ExpiresAt: now,
				},
			},
		},
	}

	for _, c := range cases {
		f, err := ioutil.TempFile("", "snapshot")
		require.NoError(t, err, "creating temp file failed")

		l1 := &nlog{
			st:      gossipData{},
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
		l2 := &nlog{}
		err = l2.loadSnapshot(f)
		require.NoError(t, err, "error loading snapshot")
		require.Equal(t, l1.st, l2.st, "state after loading snapshot did not match snapshotted state")

		require.NoError(t, f.Close(), "closing snapshot file failed")
	}
}

func TestReplaceFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "replace_file")
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

	res, err := ioutil.ReadAll(ofr)
	require.NoError(t, err, "reading original file failed")
	require.Equal(t, "test", string(res), "unexpected file contents")
}

func TestGossipDataMerge(t *testing.T) {
	now := utcNow()

	// We only care about key names and timestamps for the
	// merging logic.
	newEntry := func(ts time.Time) *pb.MeshEntry {
		return &pb.MeshEntry{
			Entry: &pb.Entry{Timestamp: ts},
		}
	}
	cases := []struct {
		a, b         gossipData
		final, delta gossipData
	}{
		{
			a: gossipData{
				"a1": newEntry(now),
				"a2": newEntry(now),
				"a3": newEntry(now),
			},
			b: gossipData{
				"b1": newEntry(now),                   // new key, should be added
				"a2": newEntry(now.Add(-time.Minute)), // older timestamp, should be dropped
				"a3": newEntry(now.Add(time.Minute)),  // newer timestamp, should overwrite
			},
			final: gossipData{
				"a1": newEntry(now),
				"a2": newEntry(now),
				"a3": newEntry(now.Add(time.Minute)),
				"b1": newEntry(now),
			},
			delta: gossipData{
				"b1": newEntry(now),
				"a3": newEntry(now.Add(time.Minute)),
			},
		},
	}

	for _, c := range cases {
		ca, cb := c.a.clone(), c.b.clone()

		res := ca.Merge(cb)

		require.Equal(t, c.final, res, "Merge result should match expectation")
		require.Equal(t, c.final, ca, "Merge should apply changes to original state")
		require.Equal(t, c.b, cb, "Merged state should remain unmodified")

		ca, cb = c.a.clone(), c.b.clone()

		delta := ca.mergeDelta(cb)

		require.Equal(t, c.delta, delta, "Merge delta should match expectation")
		require.Equal(t, c.final, ca, "Merge should apply changes to original state")
		require.Equal(t, c.b, cb, "Merged state should remain unmodified")
	}
}

func TestGossipDataCoding(t *testing.T) {
	// Check whether encoding and decoding the data is symmetric.
	now := utcNow()

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
						Timestamp: now,
					},
					ExpiresAt: now,
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("d8e8fca2dc0f8abce7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "def", Integration: "test2", Idx: 29},
						GroupHash: []byte("122c2331b9d1bbd07fddc65819a542c3"),
						Resolved:  true,
						Timestamp: now,
					},
					ExpiresAt: now,
				}, {
					Entry: &pb.Entry{
						GroupKey:  []byte("aaaaaca2dc0f896fd7cb4cb0031ba249"),
						Receiver:  &pb.Receiver{GroupName: "ghi", Integration: "test3", Idx: 0},
						GroupHash: []byte("126a8a51b9d1bbd07fddc6e3e3e542c3"),
						Resolved:  false,
						Timestamp: now,
					},
					ExpiresAt: now,
				},
			},
		},
	}

	for _, c := range cases {
		// Create gossip data from input.
		in := gossipData{}
		for _, e := range c.entries {
			in[stateKey(string(e.Entry.GroupKey), e.Entry.Receiver)] = e
		}
		msg := in.Encode()
		require.Equal(t, 1, len(msg), "expected single message for input")

		out, err := decodeGossipData(msg[0])
		require.NoError(t, err, "decoding message failed")

		require.Equal(t, in, out, "decoded data doesn't match encoded data")
	}
}
