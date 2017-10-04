package dispatch

import (
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
)

func newAPIAlert(labels model.LabelSet) APIAlert {
	return APIAlert{
		Alert: &model.Alert{
			Labels:   labels,
			StartsAt: time.Now().Add(1 * time.Minute),
			EndsAt:   time.Now().Add(1 * time.Hour),
		},
	}
}

func TestFilterLabels(t *testing.T) {

	var (
		a1 = newAPIAlert(model.LabelSet{
			"a": "v1",
			"b": "v2",
			"c": "v3",
		})
		a2 = newAPIAlert(model.LabelSet{
			"a": "v1",
			"b": "v2",
			"c": "v4",
		})
		a3 = newAPIAlert(model.LabelSet{
			"a": "v1",
			"b": "v2",
			"c": "v5",
		})
		a4 = newAPIAlert(model.LabelSet{
			"foo": "bar",
			"baz": "qux",
		})
		alertsSlices = []struct {
			in, want []APIAlert
		}{
			{
				in:   []APIAlert{a1, a2, a3},
				want: []APIAlert{a1, a2, a3},
			},
			{
				in:   []APIAlert{a1, a4},
				want: []APIAlert{a1},
			},
			{
				in:   []APIAlert{a4},
				want: []APIAlert{},
			},
		}
	)

	matcher, err := labels.NewMatcher(labels.MatchRegexp, "c", "v.*")
	if err != nil {
		t.Fatalf("error making matcher: %v", err)
	}
	matcher2, err := labels.NewMatcher(labels.MatchEqual, "a", "v1")
	if err != nil {
		t.Fatalf("error making matcher: %v", err)
	}

	matchers := []*labels.Matcher{matcher, matcher2}

	for _, alerts := range alertsSlices {
		got := []APIAlert{}
		for _, a := range alerts.in {
			if matchesFilterLabels(&a, matchers) {
				got = append(got, a)
			}
		}
		if !reflect.DeepEqual(got, alerts.want) {
			t.Fatalf("error: returned alerts do not match:\ngot  %v\nwant %v", got, alerts.want)
		}
	}
}

func TestAggrGroup(t *testing.T) {
	lset := model.LabelSet{
		"a": "v1",
		"b": "v2",
	}
	opts := &RouteOpts{
		Receiver:       "n1",
		GroupBy:        map[model.LabelName]struct{}{},
		GroupWait:      1 * time.Second,
		GroupInterval:  300 * time.Millisecond,
		RepeatInterval: 1 * time.Hour,
	}
	route := &Route{
		RouteOpts: *opts,
	}

	var (
		a1 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v3",
				},
				StartsAt: time.Now().Add(time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
			UpdatedAt: time.Now(),
		}
		a2 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v4",
				},
				StartsAt: time.Now().Add(-time.Hour),
				EndsAt:   time.Now().Add(2 * time.Hour),
			},
			UpdatedAt: time.Now(),
		}
		a3 = &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"a": "v1",
					"b": "v2",
					"c": "v5",
				},
				StartsAt: time.Now().Add(time.Minute),
				EndsAt:   time.Now().Add(5 * time.Minute),
			},
			UpdatedAt: time.Now(),
		}
	)

	var (
		last       = time.Now()
		current    = time.Now()
		lastCurMtx = &sync.Mutex{}
		alertsCh   = make(chan types.AlertSlice)
	)

	ntfy := func(ctx context.Context, alerts ...*types.Alert) bool {
		// Validate that the context is properly populated.
		if _, ok := notify.Now(ctx); !ok {
			t.Errorf("now missing")
		}
		if _, ok := notify.GroupKey(ctx); !ok {
			t.Errorf("group key missing")
		}
		if lbls, ok := notify.GroupLabels(ctx); !ok || !reflect.DeepEqual(lbls, lset) {
			t.Errorf("wrong group labels: %q", lbls)
		}
		if rcv, ok := notify.ReceiverName(ctx); !ok || rcv != opts.Receiver {
			t.Errorf("wrong receiver: %q", rcv)
		}
		if ri, ok := notify.RepeatInterval(ctx); !ok || ri != opts.RepeatInterval {
			t.Errorf("wrong repeat interval: %q", ri)
		}

		lastCurMtx.Lock()
		last = current
		current = time.Now()
		lastCurMtx.Unlock()

		alertsCh <- types.AlertSlice(alerts)

		return true
	}

	// Test regular situation where we wait for group_wait to send out alerts.
	ag := newAggrGroup(context.Background(), lset, route, nil)
	go ag.run(ntfy)

	ag.insert(a1)

	select {
	case <-time.After(2 * opts.GroupWait):
		t.Fatalf("expected initial batch after group_wait")

	case batch := <-alertsCh:
		if s := time.Since(last); s < opts.GroupWait {
			t.Fatalf("received batch to early after %v", s)
		}
		exp := types.AlertSlice{a1}
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}
	}

	for i := 0; i < 3; i++ {
		// New alert should come in after group interval.
		ag.insert(a3)

		select {
		case <-time.After(2 * opts.GroupInterval):
			t.Fatalf("expected new batch after group interval but received none")

		case batch := <-alertsCh:
			if s := time.Since(last); s < opts.GroupInterval {
				t.Fatalf("received batch to early after %v", s)
			}
			exp := types.AlertSlice{a1, a3}
			sort.Sort(batch)

			if !reflect.DeepEqual(batch, exp) {
				t.Fatalf("expected alerts %v but got %v", exp, batch)
			}
		}
	}

	ag.stop()

	// Add an alert that started more than group_interval in the past. We expect
	// immediate flushing.
	// Finally, set all alerts to be resolved. After successful notify the aggregation group
	// should empty itself.
	ag = newAggrGroup(context.Background(), lset, route, nil)
	go ag.run(ntfy)

	ag.insert(a1)
	ag.insert(a2)

	// a2 lies way in the past so the initial group_wait should be skipped.
	select {
	case <-time.After(opts.GroupWait / 2):
		t.Fatalf("expected immediate alert but received none")

	case batch := <-alertsCh:
		exp := types.AlertSlice{a1, a2}
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}
	}

	for i := 0; i < 3; i++ {
		// New alert should come in after group interval.
		ag.insert(a3)

		select {
		case <-time.After(2 * opts.GroupInterval):
			t.Fatalf("expected new batch after group interval but received none")

		case batch := <-alertsCh:
			lastCurMtx.Lock()
			s := time.Since(last)
			lastCurMtx.Unlock()
			if s < opts.GroupInterval {
				t.Fatalf("received batch to early after %v", s)
			}
			exp := types.AlertSlice{a1, a2, a3}
			sort.Sort(batch)

			if !reflect.DeepEqual(batch, exp) {
				t.Fatalf("expected alerts %v but got %v", exp, batch)
			}
		}
	}

	// Resolve all alerts, they should be removed after the next batch was sent.
	a1.EndsAt = time.Now()
	a2.EndsAt = time.Now()
	a3.EndsAt = time.Now()

	select {
	case <-time.After(2 * opts.GroupInterval):
		t.Fatalf("expected new batch after group interval but received none")

	case batch := <-alertsCh:
		if s := time.Since(last); s < opts.GroupInterval {
			t.Fatalf("received batch to early after %v", s)
		}
		exp := types.AlertSlice{a1, a2, a3}
		sort.Sort(batch)

		if !reflect.DeepEqual(batch, exp) {
			t.Fatalf("expected alerts %v but got %v", exp, batch)
		}

		if !ag.empty() {
			t.Fatalf("Expected aggregation group to be empty after resolving alerts: %v", ag)
		}
	}

	ag.stop()
}
