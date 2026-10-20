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
	"sort"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

// TestSilenceQueryAt evaluates query filters at a specific point in time.
// This test ensures that queries return the correct state of silences
// based on when they were evaluated, not when the query was executed.
func TestSilenceQueryAt(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: 24 * time.Hour})
		require.NoError(t, err)
		now := store.nowUTC()
		sil := newStateTestSilence(now.Add(time.Hour), now.Add(2*time.Hour))
		require.NoError(t, store.Set(t.Context(), sil))
		time.Sleep(3 * time.Hour)
		result, _, err := store.QueryAt(t.Context(), now.Add(time.Hour), QState(SilenceStateActive))
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, sil.Id, result[0].Id)
		active, _, err := store.Query(t.Context(), QState(SilenceStateActive))
		require.NoError(t, err)
		require.Empty(t, active)
	})
}

func TestQState(t *testing.T) {
	now := time.Now()

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
