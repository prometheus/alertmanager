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
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/mesh"
)

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
		err := c.options.validate()
		if err == nil {
			if c.err != "" {
				t.Errorf("expected error containing %q but got none", c.err)
			}
			continue
		}
		if err != nil && c.err == "" {
			t.Errorf("unexpected error %q", err)
			continue
		}
		if !strings.Contains(err.Error(), c.err) {
			t.Errorf("expected error to contain %q but got %q", c.err, err)
		}
	}
}

func TestSilencesGC(t *testing.T) {
	s, err := New(Options{})
	require.NoError(t, err)

	now := utcNow()
	s.now = func() time.Time { return now }

	newSilence := func(exp time.Time) *pb.MeshSilence {
		return &pb.MeshSilence{ExpiresAt: mustTimeProto(exp)}
	}
	s.st = gossipData{
		"1": newSilence(now),
		"2": newSilence(now.Add(-time.Second)),
		"3": newSilence(now.Add(time.Second)),
	}
	want := gossipData{
		"3": newSilence(now.Add(time.Second)),
	}

	n, err := s.GC()
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, want, s.st)
}

func TestSilencesSnapshot(t *testing.T) {
	// Check whether storing and loading the snapshot is symmetric.
	now := utcNow()

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
						StartsAt:  mustTimeProto(now),
						EndsAt:    mustTimeProto(now),
						UpdatedAt: mustTimeProto(now),
					},
					ExpiresAt: mustTimeProto(now),
				},
				{
					Silence: &pb.Silence{
						Id: "4b1e760d-182c-4980-b873-c1a6827c9817",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						},
						StartsAt:  mustTimeProto(now.Add(time.Hour)),
						EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
						UpdatedAt: mustTimeProto(now),
					},
					ExpiresAt: mustTimeProto(now.Add(24 * time.Hour)),
				},
			},
		},
	}

	for _, c := range cases {
		f, err := ioutil.TempFile("", "snapshot")
		require.NoError(t, err, "creating temp file failed")

		s1 := &Silences{st: gossipData{}, metrics: newMetrics(nil)}
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
		s2 := &Silences{mc: matcherCache{}}
		err = s2.loadSnapshot(f)
		require.NoError(t, err, "error loading snapshot")
		require.Equal(t, s1.st, s2.st, "state after loading snapshot did not match snapshotted state")

		require.NoError(t, f.Close(), "closing snapshot file failed")
	}
}

type mockGossip struct {
	broadcast func(mesh.GossipData)
}

func (g *mockGossip) GossipBroadcast(d mesh.GossipData)         { g.broadcast(d) }
func (g *mockGossip) GossipUnicast(mesh.PeerName, []byte) error { panic("not implemented") }

func TestSilencesSetSilence(t *testing.T) {
	s, err := New(Options{
		Retention: time.Minute,
	})
	require.NoError(t, err)

	now := utcNow()
	nowpb := mustTimeProto(now)

	sil := &pb.Silence{
		Id:     "some_id",
		EndsAt: nowpb,
	}

	want := gossipData{
		"some_id": &pb.MeshSilence{
			Silence:   sil,
			ExpiresAt: mustTimeProto(now.Add(time.Minute)),
		},
	}

	var called bool
	s.gossip = &mockGossip{
		broadcast: func(d mesh.GossipData) {
			data, ok := d.(gossipData)
			require.True(t, ok, "gossip data of unknown type")
			require.Equal(t, want, data, "unexpected gossip broadcast data")

			called = true
		},
	}
	require.NoError(t, s.setSilence(sil))
	require.True(t, called, "GossipBroadcast was not called")
	require.Equal(t, want, s.st, "Unexpected silence state")
}

func TestSilenceCreate(t *testing.T) {
	s, err := New(Options{
		Retention: time.Hour,
	})
	require.NoError(t, err)

	now := utcNow()
	s.now = func() time.Time { return now }

	// Insert silence with fixed start time.
	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: mustTimeProto(now.Add(2 * time.Minute)),
		EndsAt:   mustTimeProto(now.Add(5 * time.Minute)),
	}
	id1, err := s.Create(sil1)
	require.NoError(t, err)
	require.NotEqual(t, id1, "")

	want := gossipData{
		id1: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id1,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  mustTimeProto(now.Add(2 * time.Minute)),
				EndsAt:    mustTimeProto(now.Add(5 * time.Minute)),
				UpdatedAt: mustTimeProto(now),
			},
			ExpiresAt: mustTimeProto(now.Add(5*time.Minute + s.retention)),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Insert silence with unset start time. Must be set to now.
	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		EndsAt:   mustTimeProto(now.Add(1 * time.Minute)),
	}
	id2, err := s.Create(sil2)
	require.NoError(t, err)
	require.NotEqual(t, id2, "")

	want = gossipData{
		id1: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id1,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  mustTimeProto(now.Add(2 * time.Minute)),
				EndsAt:    mustTimeProto(now.Add(5 * time.Minute)),
				UpdatedAt: mustTimeProto(now),
			},
			ExpiresAt: mustTimeProto(now.Add(5*time.Minute + s.retention)),
		},
		id2: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id2,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  mustTimeProto(now),
				EndsAt:    mustTimeProto(now.Add(1 * time.Minute)),
				UpdatedAt: mustTimeProto(now),
			},
			ExpiresAt: mustTimeProto(now.Add(1*time.Minute + s.retention)),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

}

func TestSilencesCreateFail(t *testing.T) {
	s, err := New(Options{})
	require.NoError(t, err)

	now := utcNow()
	s.now = func() time.Time { return now }

	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s:   &pb.Silence{Id: "some_id"},
			err: "unexpected ID in new silence",
		}, {
			s:   &pb.Silence{StartsAt: mustTimeProto(now.Add(-time.Minute))},
			err: "new silence must not start in the past",
		}, {
			s:   &pb.Silence{}, // Silence without matcher.
			err: "invalid silence",
		},
	}
	for _, c := range cases {
		_, err := s.Create(c.s)
		if err == nil {
			if c.err != "" {
				t.Errorf("expected error containing %q but got none", c.err)
			}
			continue
		}
		if err != nil && c.err == "" {
			t.Errorf("unexpected error %q", err)
			continue
		}
		if !strings.Contains(err.Error(), c.err) {
			t.Errorf("expected error to contain %q but got %q", c.err, err)
		}
	}
}

func TestQState(t *testing.T) {
	now := utcNow()

	cases := []struct {
		sil    *pb.Silence
		states []SilenceState
		keep   bool
	}{
		{
			sil: &pb.Silence{
				StartsAt: mustTimeProto(now.Add(time.Minute)),
				EndsAt:   mustTimeProto(now.Add(time.Hour)),
			},
			states: []SilenceState{StateActive, StateExpired},
			keep:   false,
		},
		{
			sil: &pb.Silence{
				StartsAt: mustTimeProto(now.Add(time.Minute)),
				EndsAt:   mustTimeProto(now.Add(time.Hour)),
			},
			states: []SilenceState{StatePending},
			keep:   true,
		},
		{
			sil: &pb.Silence{
				StartsAt: mustTimeProto(now.Add(time.Minute)),
				EndsAt:   mustTimeProto(now.Add(time.Hour)),
			},
			states: []SilenceState{StateExpired, StatePending},
			keep:   true,
		},
	}
	for i, c := range cases {
		q := &query{}
		QState(c.states...)(q)
		f := q.filters[0]

		keep, err := f(c.sil, nil, mustTimeProto(now))
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
					{Name: "job", Pattern: "test", Type: pb.Matcher_EQUAL},
					{Name: "method", Pattern: "POST", Type: pb.Matcher_EQUAL},
				},
			},
			drop: false,
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
					{Name: "path", Pattern: "/user/.+", Type: pb.Matcher_REGEXP},
					{Name: "path", Pattern: "/nothing/.+", Type: pb.Matcher_REGEXP},
				},
			},
			drop: false,
		},
	}
	for _, c := range cases {
		drop, err := f(c.sil, &Silences{mc: matcherCache{}}, nil)
		require.NoError(t, err)
		require.Equal(t, c.drop, drop, "unexpected filter result")
	}
}

func TestSilencesQuery(t *testing.T) {
	s, err := New(Options{})
	require.NoError(t, err)

	s.st = gossipData{
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
					func(sil *pb.Silence, _ *Silences, _ *timestamp.Timestamp) (bool, error) {
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
					func(sil *pb.Silence, _ *Silences, _ *timestamp.Timestamp) (bool, error) {
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
		res, err := s.query(c.q, nil)
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

func TestSilenceSetTimeRange(t *testing.T) {
	now := utcNow()

	cases := []struct {
		sil        *pb.Silence
		start, end *timestamp.Timestamp
		err        string
	}{
		// Bad arguments.
		{
			sil:   &pb.Silence{},
			start: mustTimeProto(now),
			end:   mustTimeProto(now.Add(-time.Minute)),
			err:   "end time must not be before start time",
		},
		// Expired silence.
		{
			sil: &pb.Silence{
				StartsAt: mustTimeProto(now.Add(-time.Hour)),
				EndsAt:   mustTimeProto(now.Add(-time.Second)),
			},
			start: mustTimeProto(now),
			end:   mustTimeProto(now),
			err:   "expired silence must not be modified",
		},
		// Pending silences.
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(-time.Minute)),
			end:   mustTimeProto(now.Add(time.Hour)),
			err:   "start time cannot be set into the past",
		},
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(time.Minute)),
			end:   mustTimeProto(now.Add(time.Minute)),
		},
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now), // set to exactly start now.
			end:   mustTimeProto(now.Add(2 * time.Hour)),
		},
		// Active silences.
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(-time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(-time.Minute)),
			end:   mustTimeProto(now.Add(2 * time.Hour)),
			err:   "start time of active silence cannot be modified",
		},
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(-time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(-time.Hour)),
			end:   mustTimeProto(now.Add(-time.Second)),
			err:   "end time cannot be set into the past",
		},
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(-time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(-time.Hour)),
			end:   mustTimeProto(now),
		},
		{
			sil: &pb.Silence{
				StartsAt:  mustTimeProto(now.Add(-time.Hour)),
				EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
				UpdatedAt: mustTimeProto(now.Add(-time.Hour)),
			},
			start: mustTimeProto(now.Add(-time.Hour)),
			end:   mustTimeProto(now.Add(3 * time.Hour)),
		},
	}
	for _, c := range cases {
		origSilence := cloneSilence(c.sil)

		sil, err := silenceSetTimeRange(c.sil, mustTimeProto(now), c.start, c.end)
		if err == nil {
			if c.err != "" {
				t.Errorf("expected error containing %q but got none", c.err)
			}
			// The original silence must not have been modified.
			require.Equal(t, origSilence, c.sil, "original silence illegally modified")

			require.Equal(t, sil.StartsAt, c.start)
			require.Equal(t, sil.EndsAt, c.end)
			require.Equal(t, sil.UpdatedAt, mustTimeProto(now))
			continue
		}
		if err != nil && c.err == "" {
			t.Errorf("unexpected error %q", err)
			continue
		}
		if !strings.Contains(err.Error(), c.err) {
			t.Errorf("expected error to contain %q but got %q", c.err, err)
		}

	}
}

func TestValidateMatcher(t *testing.T) {
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
				Name:    "00",
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
				Pattern: "\xff",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label value",
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
		err := validateMatcher(c.m)
		if err == nil {
			if c.err != "" {
				t.Errorf("expected error containing %q but got none", c.err)
			}
			continue
		}
		if err != nil && c.err == "" {
			t.Errorf("unexpected error %q", err)
			continue
		}
		if !strings.Contains(err.Error(), c.err) {
			t.Errorf("expected error to contain %q but got %q", c.err, err)
		}
	}
}

func TestValidateSilence(t *testing.T) {
	var (
		now              = utcNow()
		invalidTimestamp = &timestamp.Timestamp{Nanos: 1 << 30}
		validTimestamp   = mustTimeProto(now)
	)
	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "",
		},
		{
			s: &pb.Silence{
				Id: "",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "ID missing",
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
					&pb.Matcher{Name: "a", Pattern: "b"},
					&pb.Matcher{Name: "00", Pattern: "b"},
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
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  mustTimeProto(now),
				EndsAt:    mustTimeProto(now.Add(-time.Second)),
				UpdatedAt: validTimestamp,
			},
			err: "end time must not be before start time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  invalidTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid start time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    invalidTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid end time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: invalidTimestamp,
			},
			err: "invalid update timestamp",
		},
	}
	for _, c := range cases {
		err := validateSilence(c.s)
		if err == nil {
			if c.err != "" {
				t.Errorf("expected error containing %q but got none", c.err)
			}
			continue
		}
		if err != nil && c.err == "" {
			t.Errorf("unexpected error %q", err)
			continue
		}
		if !strings.Contains(err.Error(), c.err) {
			t.Errorf("expected error to contain %q but got %q", c.err, err)
		}
	}
}

func TestGossipDataMerge(t *testing.T) {
	now := utcNow()

	// We only care about key names and timestamps for the
	// merging logic.
	newSilence := func(ts time.Time) *pb.MeshSilence {
		return &pb.MeshSilence{
			Silence: &pb.Silence{UpdatedAt: mustTimeProto(ts)},
		}
	}
	cases := []struct {
		a, b         gossipData
		final, delta gossipData
	}{
		{
			a: gossipData{
				"a1": newSilence(now),
				"a2": newSilence(now),
				"a3": newSilence(now),
			},
			b: gossipData{
				"b1": newSilence(now),                   // new key, should be added
				"a2": newSilence(now.Add(-time.Minute)), // older timestamp, should be dropped
				"a3": newSilence(now.Add(time.Minute)),  // newer timestamp, should overwrite
			},
			final: gossipData{
				"a1": newSilence(now),
				"a2": newSilence(now),
				"a3": newSilence(now.Add(time.Minute)),
				"b1": newSilence(now),
			},
			delta: gossipData{
				"b1": newSilence(now),
				"a3": newSilence(now.Add(time.Minute)),
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
						StartsAt:  mustTimeProto(now),
						EndsAt:    mustTimeProto(now),
						UpdatedAt: mustTimeProto(now),
					},
					ExpiresAt: mustTimeProto(now),
				},
				{
					Silence: &pb.Silence{
						Id: "4b1e760d-182c-4980-b873-c1a6827c9817",
						Matchers: []*pb.Matcher{
							{Name: "label1", Pattern: "val1", Type: pb.Matcher_EQUAL},
						},
						StartsAt:  mustTimeProto(now.Add(time.Hour)),
						EndsAt:    mustTimeProto(now.Add(2 * time.Hour)),
						UpdatedAt: mustTimeProto(now),
					},
					ExpiresAt: mustTimeProto(now.Add(24 * time.Hour)),
				},
			},
		},
	}

	for _, c := range cases {
		// Create gossip data from input.
		in := gossipData{}
		for _, e := range c.entries {
			in[e.Silence.Id] = e
		}
		msg := in.Encode()
		require.Equal(t, 1, len(msg), "expected single message for input")

		out, err := decodeGossipData(msg[0])
		require.NoError(t, err, "decoding message failed")

		require.Equal(t, in, out, "decoded data doesn't match encoded data")
	}

}

func TestProtoBefore(t *testing.T) {
	now := utcNow()
	nowpb, err := ptypes.TimestampProto(now)
	require.NoError(t, err)

	cases := []struct {
		ts     time.Time
		before bool
	}{
		{
			ts:     now.Add(time.Second),
			before: true,
		}, {
			ts:     now.Add(-time.Second),
			before: false,
		}, {
			ts:     now.Add(time.Nanosecond),
			before: true,
		}, {
			ts:     now.Add(-time.Nanosecond),
			before: false,
		}, {
			ts:     now,
			before: false,
		},
	}

	for _, c := range cases {
		tspb, err := ptypes.TimestampProto(c.ts)
		require.NoError(t, err)

		res := protoBefore(nowpb, tspb)
		require.Equal(t, c.before, res, "protoBefore returned unexpected result")
	}
}

func mustTimeProto(ts time.Time) *timestamp.Timestamp {
	pt, err := ptypes.TimestampProto(ts)
	if err != nil {
		panic(err)
	}
	return pt
}
