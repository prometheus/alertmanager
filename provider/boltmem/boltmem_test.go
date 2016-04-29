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

	n, err := NewNotifies(dir)
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
			if !reflect.DeepEqual(res, q.expected) {
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

		// Use pretty.Compare instead of reflect.DeepEqual because it
		// falsely evaluates to false.
		if len(pretty.Compare(sil, c.insert)) > 0 {
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
