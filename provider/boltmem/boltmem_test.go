// Copyright 2016 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/lic:wenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package boltmem

import (
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

func init() {
	pretty.CompareConfig.IncludeUnexported = true
}

func TestNotifiesSet(t *testing.T) {
	var (
		t0 = time.Now()
		// t1 = t0.Add(10 * time.Minute)
		// t2 = t0.Add(20 * time.Minute)
		// t3 = t0.Add(30 * time.Minute)
	)
	type query struct {
		recv     string
		fps      []model.Fingerprint
		expected []*types.NotifyInfo
	}
	var steps = []struct {
		insert  []*types.NotifyInfo
		queries []query
	}{
		{
			insert: []*types.NotifyInfo{
				{
					Alert:     30000,
					Receiver:  "receiver",
					Resolved:  false,
					Timestamp: t0,
				}, {
					Alert:     20000,
					Receiver:  "receiver",
					Resolved:  true,
					Timestamp: t0,
				}, {
					Alert:     10000,
					Receiver:  "receiver",
					Resolved:  true,
					Timestamp: t0,
				},
			},
			queries: []query{
				{
					recv: "receiver",
					fps:  []model.Fingerprint{30000, 30001, 20000, 10000},
					expected: []*types.NotifyInfo{
						{
							Alert:     30000,
							Receiver:  "receiver",
							Resolved:  false,
							Timestamp: t0,
						},
						nil, {
							Alert:     20000,
							Receiver:  "receiver",
							Resolved:  true,
							Timestamp: t0,
						}, {
							Alert:     10000,
							Receiver:  "receiver",
							Resolved:  true,
							Timestamp: t0,
						},
					},
				},
			},
		},
	}

	dir, err := ioutil.TempDir("", "notify_set_set")
	if err != nil {
		t.Fatal(err)
	}

	n, err := NewNotificationInfo(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range steps {
		if err := n.Set(step.insert...); err != nil {
			t.Fatalf("Insert failed: %s", err)
		}

		for _, q := range step.queries {
			res, err := n.Get(q.recv, q.fps...)
			if err != nil {
				t.Fatalf("Query failed: %s", err)
			}
			if !notifyInfoListEqual(res, q.expected) {
				t.Errorf("Unexpected query result")
				t.Fatalf(pretty.Compare(res, q.expected))
			}
		}
	}
}

func TestSilencesSet(t *testing.T) {
	var (
		t0 = time.Now()
		t1 = t0.Add(10 * time.Minute)
		t2 = t0.Add(20 * time.Minute)
		// t3 = t0.Add(30 * time.Minute)
	)

	var cases = []struct {
		insert *types.Silence
	}{
		{
			insert: types.NewSilence(&model.Silence{
				Matchers: []*model.Matcher{
					{Name: "key", Value: "val"},
				},
				StartsAt:  t0,
				EndsAt:    t2,
				CreatedAt: t1,
				CreatedBy: "user",
				Comment:   "test comment",
			}),
		},
	}

	dir, err := ioutil.TempDir("", "silences_test")
	if err != nil {
		t.Fatal(err)
	}

	silences, err := NewSilences(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		uid, err := silences.Set(c.insert)
		if err != nil {
			t.Fatalf("Insert failed: %s", err)
		}
		c.insert.ID = uid

		sil, err := silences.Get(uid)
		if err != nil {
			t.Fatalf("Getting failed: %s", err)
		}

		if silencesEqual(sil, c.insert) {
			t.Errorf("Unexpected silence")
			t.Fatalf(pretty.Compare(sil, c.insert))
		}
	}
}

func TestSilencesDelete(t *testing.T) {
	dir, err := ioutil.TempDir("", "silences_test")
	if err != nil {
		t.Fatal(err)
	}

	silences, err := NewSilences(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	uid, err := silences.Set(&types.Silence{
		Matchers: []*types.Matcher{
			{Name: "key", Value: "val"},
		},
		Silence: model.Silence{
			CreatedBy: "user",
			Comment:   "test comment",
		},
	})

	if err != nil {
		t.Fatalf("Insert failed: %s", err)
	}
	if err := silences.Del(uid); err != nil {
		t.Fatalf("Delete failed: %s", err)
	}

	if s, err := silences.Get(uid); err != provider.ErrNotFound {
		t.Fatalf("Expected 'not found' error but got: %v, %s", s, err)
	}
}

func TestSilencesAll(t *testing.T) {
	var (
		t0 = time.Now()
		t1 = t0.Add(10 * time.Minute)
		t2 = t0.Add(20 * time.Minute)
		t3 = t0.Add(30 * time.Minute)
	)

	insert := []*types.Silence{
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key", Value: "val"},
			},
			StartsAt:  t0,
			EndsAt:    t2,
			CreatedAt: t1,
			CreatedBy: "user",
			Comment:   "test comment",
		}),
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key", Value: "val"},
				{Name: "key2", Value: "val2.*", IsRegex: true},
			},
			StartsAt:  t1,
			EndsAt:    t2,
			CreatedAt: t1,
			CreatedBy: "user2",
			Comment:   "test comment",
		}),
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key", Value: "val"},
			},
			StartsAt:  t2,
			EndsAt:    t3,
			CreatedAt: t3,
			CreatedBy: "user",
			Comment:   "another test comment",
		}),
	}

	dir, err := ioutil.TempDir("", "silences_test")
	if err != nil {
		t.Fatal(err)
	}

	silences, err := NewSilences(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, sil := range insert {
		uid, err := silences.Set(sil)
		if err != nil {
			t.Fatalf("Insert failed: %s", err)
		}
		sil.ID = uid
	}

	res, err := silences.All()
	if err != nil {
		t.Fatalf("Retrieval failed: %s", err)
	}

	if silenceListEqual(res, insert) {
		t.Errorf("Unexpected result")
		t.Fatalf(pretty.Compare(res, insert))
	}
}

func testNewSilences(t *testing.T, dir string) *Silences {
	dir, err := ioutil.TempDir("", dir)
	if err != nil {
		t.Fatal(err)
	}

	silences, err := NewSilences(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return silences
}

func TestSilencesQuery(t *testing.T) {
	var (
		t0       = time.Now()
		silences = testNewSilences(t, "silences_test")
	)

	n := 500
	insert := make([]*types.Silence, n)
	for i := 0; i < n; i++ {
		insert[i] = createNewSilence(t, silences, t0, i)
	}

	pairs := []queryPair{
		queryPair{
			n:      10,
			offset: 0,
		},
		queryPair{
			n:      10,
			offset: 2,
		},
		queryPair{
			n:      100,
			offset: 4,
		},
	}

	for _, p := range pairs {
		res, err := silences.Query(p.n, p.offset)
		if err != nil {
			t.Fatalf("Retrieval failed: %s", err)
		}

		start := p.offset * defaultPageSize
		end := start + p.n
		if end > uint64(n) {
			t.Fatalf("your test data doesn't include the range you're requesting: insert[%d:%d] (max index %d)", start, end, n)
		}
		expected := append([]*types.Silence{}, insert[start:end]...)
		if silenceListEqual(res, expected) {
			t.Errorf("Unexpected result")
			t.Fatalf(pretty.Compare(res, expected))
		}
	}
}

func TestSilencesQueryTooManyRequested(t *testing.T) {
	var (
		t0       = time.Now()
		silences = testNewSilences(t, "silences_test")
	)

	n := 50
	insert := make([]*types.Silence, n)
	for i := 0; i < n; i++ {
		insert[i] = createNewSilence(t, silences, t0, i)
	}

	res, err := silences.Query(uint64(n*2), 0)
	if err != nil {
		t.Fatalf("Retrieval failed: %s", err)
	}

	if len(res) != n {
		t.Fatalf("incorrect silences length: wanted %d, got %d", n, len(res))
	}
}

func TestSilencesQueryTooHighOffset(t *testing.T) {
	var (
		t0       = time.Now()
		silences = testNewSilences(t, "silences_test")
	)

	n := 50
	insert := make([]*types.Silence, n)
	for i := 0; i < n; i++ {
		insert[i] = createNewSilence(t, silences, t0, i)
	}

	res, err := silences.Query(uint64(n*2), 20)
	if err != nil {
		t.Fatalf("Retrieval failed: %s", err)
	}

	if len(res) != 0 {
		t.Fatalf("incorrect silences length: wanted %d, got %d", n, len(res))
	}
}

func createNewSilence(t *testing.T, s *Silences, t0 time.Time, i int) *types.Silence {
	sil := types.NewSilence(&model.Silence{
		Matchers: []*model.Matcher{
			{Name: "key", Value: "val"},
		},
		StartsAt:  t0.Add(time.Duration(i) * time.Minute),
		EndsAt:    t0.Add((time.Duration(i) + 1) * time.Minute),
		CreatedAt: t0.Add(time.Duration(i) * time.Minute),
		CreatedBy: "user",
		Comment:   "another test comment",
	})
	uid, err := s.Set(sil)
	if err != nil {
		t.Fatalf("Insert failed: %s", err)
	}
	sil.ID = uid
	return sil
}

type queryPair struct {
	n, offset uint64
}

func alertsEqual(a1, a2 *types.Alert) bool {
	if !reflect.DeepEqual(a1.Labels, a2.Labels) {
		return false
	}
	if !reflect.DeepEqual(a1.Annotations, a2.Annotations) {
		return false
	}
	if a1.GeneratorURL != a2.GeneratorURL {
		return false
	}
	if !a1.StartsAt.Equal(a2.StartsAt) {
		return false
	}
	if !a1.EndsAt.Equal(a2.EndsAt) {
		return false
	}
	if !a1.UpdatedAt.Equal(a2.UpdatedAt) {
		return false
	}
	return a1.Timeout == a2.Timeout
}

func TestSilencesMutes(t *testing.T) {
	var (
		t0 = time.Now()
		t1 = t0.Add(10 * time.Minute)
		t2 = t0.Add(20 * time.Minute)
		t3 = t0.Add(30 * time.Minute)
	)

	// All silences are active for the time of the test. Time restriction
	// testing is covered for the Mutes() method of the Silence type.
	insert := []*types.Silence{
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key", Value: "val"},
			},
			StartsAt:  t0,
			EndsAt:    t2,
			CreatedAt: t1,
			CreatedBy: "user",
			Comment:   "test comment",
		}),
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key2", Value: "val2.*", IsRegex: true},
			},
			StartsAt:  t0,
			EndsAt:    t2,
			CreatedAt: t1,
			CreatedBy: "user2",
			Comment:   "test comment",
		}),
		types.NewSilence(&model.Silence{
			Matchers: []*model.Matcher{
				{Name: "key", Value: "val2"},
			},
			StartsAt:  t0,
			EndsAt:    t3,
			CreatedAt: t3,
			CreatedBy: "user",
			Comment:   "another test comment",
		}),
	}

	dir, err := ioutil.TempDir("", "silences_test")
	if err != nil {
		t.Fatal(err)
	}

	silences, err := NewSilences(dir, types.NewMarker())
	if err != nil {
		t.Fatal(err)
	}

	for _, sil := range insert {
		uid, err := silences.Set(sil)
		if err != nil {
			t.Fatalf("Insert failed: %s", err)
		}
		sil.ID = uid
	}

	tests := []struct {
		lset  model.LabelSet
		match bool
	}{
		{
			lset: model.LabelSet{
				"foo": "bar",
				"bar": "foo",
			},
			match: false,
		},
		{
			lset: model.LabelSet{
				"key": "val",
				"bar": "foo",
			},
			match: true,
		},
		{
			lset: model.LabelSet{
				"foo": "bar",
				"key": "val2",
			},
			match: true,
		},
		{
			lset: model.LabelSet{
				"key2": "bar",
				"bar":  ":$foo",
			},
			match: false,
		},
		{
			lset: model.LabelSet{
				"key2": "val2",
				"bar":  "foo",
			},
			match: true,
		},
		{
			lset: model.LabelSet{
				"key2": "val2 foo",
				"bar":  "foo",
			},
			match: true,
		},
	}

	for i, test := range tests {
		if b := silences.Mutes(test.lset); b != test.match {
			t.Errorf("Unexpected mute result: %d", i)
			t.Fatalf("Expected %v, got %v", test.match, b)
		} else {
			if _, wasSilenced := silences.mk.Silenced(test.lset.Fingerprint()); wasSilenced != b {
				t.Fatalf("Marker was not set correctly: %d", i)
			}
		}
	}
}

func TestAlertsPut(t *testing.T) {
	dir, err := ioutil.TempDir("", "alerts_test")
	if err != nil {
		t.Fatal(err)
	}

	alerts, err := NewAlerts(dir)
	if err != nil {
		t.Fatal(err)
	}

	var (
		t0 = time.Now()
		t1 = t0.Add(10 * time.Minute)
	)

	insert := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:       model.LabelSet{"bar": "foo"},
				Annotations:  model.LabelSet{"foo": "bar"},
				StartsAt:     t0,
				EndsAt:       t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: t0,
			Timeout:   false,
		}, {
			Alert: model.Alert{
				Labels:       model.LabelSet{"bar": "foo2"},
				Annotations:  model.LabelSet{"foo": "bar2"},
				StartsAt:     t0,
				EndsAt:       t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: t0,
			Timeout:   false,
		}, {
			Alert: model.Alert{
				Labels:       model.LabelSet{"bar": "foo3"},
				Annotations:  model.LabelSet{"foo": "bar3"},
				StartsAt:     t0,
				EndsAt:       t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: t0,
			Timeout:   false,
		},
	}

	if err := alerts.Put(insert...); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	for i, a := range insert {
		res, err := alerts.Get(a.Fingerprint())
		if err != nil {
			t.Fatalf("retrieval error: %s", err)
		}
		if !alertsEqual(res, a) {
			t.Errorf("Unexpected alert: %d", i)
			t.Fatalf(pretty.Compare(res, a))
		}
	}
}

func alertListEqual(a1, a2 []*types.Alert) bool {
	if len(a1) != len(a2) {
		return false
	}
	for i, a := range a1 {
		if !alertsEqual(a, a2[i]) {
			return false
		}
	}
	return true
}

func silencesEqual(s1, s2 *types.Silence) bool {
	if !reflect.DeepEqual(s1.Matchers, s2.Matchers) {
		return false
	}
	if !reflect.DeepEqual(s1.Silence.Matchers, s2.Silence.Matchers) {
		return false
	}
	if !s1.StartsAt.Equal(s2.EndsAt) {
		return false
	}
	if !s1.EndsAt.Equal(s2.EndsAt) {
		return false
	}
	if !s1.CreatedAt.Equal(s2.CreatedAt) {
		return false
	}
	return s1.Comment == s2.Comment && s1.CreatedBy == s2.CreatedBy
}

func silenceListEqual(s1, s2 []*types.Silence) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, s := range s1 {
		if !silencesEqual(s, s2[i]) {
			return false
		}
	}
	return true
}

func notifyInfoEqual(n1, n2 *types.NotifyInfo) bool {
	// nil is a sentinel value and thus part of comparisons.
	if n1 == nil || n2 == nil {
		return n1 == nil && n2 == nil
	}
	if n1.Alert != n2.Alert {
		return false
	}
	if n1.Receiver != n2.Receiver {
		return false
	}
	if !n1.Timestamp.Equal(n2.Timestamp) {
		return false
	}
	return n1.Resolved == n2.Resolved
}

func notifyInfoListEqual(n1, n2 []*types.NotifyInfo) bool {

	if len(n1) != len(n2) {
		return false
	}
	for i, n := range n1 {
		if !notifyInfoEqual(n, n2[i]) {
			return false
		}
	}
	return true
}
