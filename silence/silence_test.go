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

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
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
		return &pb.MeshSilence{ExpiresAt: exp}
	}
	s.st = state{
		"1": newSilence(now),
		"2": newSilence(now.Add(-time.Second)),
		"3": newSilence(now.Add(time.Second)),
	}
	want := state{
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
		f, err := ioutil.TempFile("", "snapshot")
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

func TestSilencesSetSilence(t *testing.T) {
	s, err := New(Options{
		Retention: time.Minute,
	})
	require.NoError(t, err)

	now := utcNow()
	nowpb := now

	sil := &pb.Silence{
		Id:       "some_id",
		Matchers: []*pb.Matcher{{Name: "abc", Pattern: "def"}},
		StartsAt: nowpb,
		EndsAt:   nowpb,
	}

	want := state{
		"some_id": &pb.MeshSilence{
			Silence:   sil,
			ExpiresAt: now.Add(time.Minute),
		},
	}

	done := make(chan bool)
	s.broadcast = func(b []byte) {
		var e pb.MeshSilence
		r := bytes.NewReader(b)
		_, err := pbutil.ReadDelimited(r, &e)
		require.NoError(t, err)

		require.Equal(t, want["some_id"], &e)
		close(done)
	}

	// setSilence() is always called with s.mtx locked()
	go func() {
		s.mtx.Lock()
		require.NoError(t, s.setSilence(sil))
		s.mtx.Unlock()
	}()

	// GossipBroadcast is called in a goroutine.
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("GossipBroadcast was not called")
	}

	require.Equal(t, want, s.st, "Unexpected silence state")
}

func TestSilenceSet(t *testing.T) {
	s, err := New(Options{
		Retention: time.Hour,
	})
	require.NoError(t, err)

	now := utcNow()
	now1 := now
	s.now = func() time.Time { return now }

	// Insert silence with fixed start time.
	sil1 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		StartsAt: now.Add(2 * time.Minute),
		EndsAt:   now.Add(5 * time.Minute),
	}
	id1, err := s.Set(sil1)
	require.NoError(t, err)
	require.NotEqual(t, id1, "")

	want := state{
		id1: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id1,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  now1.Add(2 * time.Minute),
				EndsAt:    now1.Add(5 * time.Minute),
				UpdatedAt: now1,
			},
			ExpiresAt: now1.Add(5*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Insert silence with unset start time. Must be set to now.
	now = now.Add(time.Minute)
	now2 := now

	sil2 := &pb.Silence{
		Matchers: []*pb.Matcher{{Name: "a", Pattern: "b"}},
		EndsAt:   now.Add(1 * time.Minute),
	}
	id2, err := s.Set(sil2)
	require.NoError(t, err)
	require.NotEqual(t, id2, "")

	want = state{
		id1: want[id1],
		id2: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id2,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  now2,
				EndsAt:    now2.Add(1 * time.Minute),
				UpdatedAt: now2,
			},
			ExpiresAt: now2.Add(1*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Overwrite silence 2 with new end time.
	now = now.Add(time.Minute)
	now3 := now

	sil3 := cloneSilence(sil2)
	sil3.EndsAt = now.Add(100 * time.Minute)

	id3, err := s.Set(sil3)
	require.NoError(t, err)
	require.Equal(t, id2, id3)

	want = state{
		id1: want[id1],
		id2: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id2,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  now2,
				EndsAt:    now3.Add(100 * time.Minute),
				UpdatedAt: now3,
			},
			ExpiresAt: now3.Add(100*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Update silence 2 with new matcher expires it and creates a new one.
	now = now.Add(time.Minute)
	now4 := now

	sil4 := cloneSilence(sil3)
	sil4.Matchers = []*pb.Matcher{{Name: "a", Pattern: "c"}}

	id4, err := s.Set(sil4)
	require.NoError(t, err)
	require.NotEqual(t, id2, id4)

	want = state{
		id1: want[id1],
		id2: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id2,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  now2,
				EndsAt:    now4,
				UpdatedAt: now4,
			},
			ExpiresAt: now4.Add(s.retention),
		},
		id4: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id4,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "c"}},
				StartsAt:  now4,
				EndsAt:    now3.Add(100 * time.Minute),
				UpdatedAt: now4,
			},
			ExpiresAt: now3.Add(100*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")

	// Re-create expired silence.
	now = now.Add(time.Minute)
	now5 := now

	sil5 := cloneSilence(sil3)
	sil5.StartsAt = now
	sil5.EndsAt = now.Add(5 * time.Minute)

	id5, err := s.Set(sil5)
	require.NoError(t, err)
	require.NotEqual(t, id2, id4)

	want = state{
		id1: want[id1],
		id2: want[id2],
		id4: want[id4],
		id5: &pb.MeshSilence{
			Silence: &pb.Silence{
				Id:        id5,
				Matchers:  []*pb.Matcher{{Name: "a", Pattern: "b"}},
				StartsAt:  now5,
				EndsAt:    now5.Add(5 * time.Minute),
				UpdatedAt: now5,
			},
			ExpiresAt: now5.Add(5*time.Minute + s.retention),
		},
	}
	require.Equal(t, want, s.st, "unexpected state after silence creation")
}

func TestSilencesSetFail(t *testing.T) {
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
			err: ErrNotFound.Error(),
		}, {
			s:   &pb.Silence{}, // Silence without matcher.
			err: "silence invalid",
		},
	}
	for _, c := range cases {
		_, err := s.Set(c.s)
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
		res, err := s.query(c.q, time.Time{})
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
	now := utcNow()

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
	s, err := New(Options{})
	require.NoError(t, err)

	now := time.Now()
	s.now = func() time.Time { return now }

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

	require.NoError(t, s.expire("pending"))
	require.NoError(t, s.expire("active"))

	err = s.expire("expired")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already expired")

	sil, err := s.QueryOne(QIDs("pending"))
	require.NoError(t, err)
	require.Equal(t, &pb.Silence{
		Id:        "pending",
		Matchers:  []*pb.Matcher{m},
		StartsAt:  now,
		EndsAt:    now,
		UpdatedAt: now,
	}, sil)

	count, err = s.CountState(types.SilenceStatePending)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// Expiring a pending Silence should make the API return the
	// SilenceStateExpired Silence state.
	silenceState := types.CalcSilenceState(sil.StartsAt, sil.EndsAt)
	require.Equal(t, silenceState, types.SilenceStateExpired)

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
		now            = utcNow()
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
					&pb.Matcher{Name: "a", Pattern: "b"},
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
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    zeroTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero end timestamp",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					&pb.Matcher{Name: "a", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: zeroTimestamp,
			},
			err: "invalid zero update timestamp",
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

func TestStateMerge(t *testing.T) {
	now := utcNow()

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
