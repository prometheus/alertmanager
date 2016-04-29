package boltmem

import (
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

func init() {
	pretty.CompareConfig.IncludeUnexported = false
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
