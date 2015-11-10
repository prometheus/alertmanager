// Copyright 2015 Prometheus Team
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

package notify

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

type recordNotifier struct {
	ctx    context.Context
	alerts []*types.Alert
}

func (n *recordNotifier) Notify(ctx context.Context, as ...*types.Alert) error {
	n.ctx = ctx
	n.alerts = append(n.alerts, as...)
	return nil
}

type failNotifier struct{}

func (n *failNotifier) Notify(ctx context.Context, as ...*types.Alert) error {
	return fmt.Errorf("some error")
}

func TestDedupingNotifier(t *testing.T) {
	var (
		record   = &recordNotifier{}
		notifies = provider.NewMemNotifies(provider.NewMemData())
		deduper  = Dedup(notifies, record)
		ctx      = context.Background()
	)
	now := time.Now()

	ctx = WithReceiver(ctx, "name")
	ctx = WithRepeatInterval(ctx, time.Duration(100*time.Minute))
	ctx = WithSendResolved(ctx, true)
	ctx = WithNow(ctx, now)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "0"},
			},
		}, {
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "1"},
				EndsAt: now.Add(-5 * time.Minute),
			},
		}, {
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "2"},
				EndsAt: now.Add(-9 * time.Minute),
			},
		}, {
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "3"},
				EndsAt: now.Add(-10 * time.Minute),
			},
		}, {
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "4"},
			},
		}, {
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "5"},
			},
		},
	}

	var fps []model.Fingerprint
	for _, a := range alerts {
		fps = append(fps, a.Fingerprint())
	}

	nsBefore := []*types.NotifyInfo{
		// The first a new alert starting now.
		nil,
		// The second alert was not previously notified about and
		// is already resolved.
		nil,
		// The third alert is an attempt to resolve a previously
		// firing alert.
		{
			Alert:     fps[2],
			Receiver:  "name",
			Resolved:  false,
			Timestamp: now.Add(-10 * time.Minute),
		},
		// The fourth alert is an attempt to resolve an alert again
		// even though the previous notification succeeded.
		{
			Alert:     fps[3],
			Receiver:  "name",
			Resolved:  true,
			Timestamp: now.Add(-10 * time.Minute),
		},
		// The fifth alert resends a previously successful notification
		// that was longer than ago than the repeat interval.
		{
			Alert:     fps[4],
			Receiver:  "name",
			Resolved:  false,
			Timestamp: now.Add(-110 * time.Minute),
		},
		// The sixth alert is a firing again after being resolved before.
		{
			Alert:     fps[5],
			Receiver:  "name",
			Resolved:  true,
			Timestamp: now.Add(3 * time.Minute),
		},
	}

	if err := notifies.Set(nsBefore...); err != nil {
		t.Fatalf("Setting notifies failed: %s", err)
	}

	deduper.notifier = &failNotifier{}
	if err := deduper.Notify(ctx, alerts...); err == nil {
		t.Fatalf("Fail notifier did not fail")
	}
	// After a failing notify the notifies data must be unchanged.
	nsCur, err := notifies.Get("name", fps...)
	if err != nil {
		t.Fatalf("Error getting notify info: %s", err)
	}
	if !reflect.DeepEqual(nsBefore, nsCur) {
		t.Fatalf("Notify info data has changed unexpectedly")
	}

	deduper.notifier = record
	if err := deduper.Notify(ctx, alerts...); err != nil {
		t.Fatalf("Notify failed: %s", err)
	}

	alertsExp := []*types.Alert{
		alerts[0],
		alerts[2],
		alerts[4],
		alerts[5],
	}

	nsAfter := []*types.NotifyInfo{
		{
			Alert:    fps[0],
			Receiver: "name",
			Resolved: false,
		},
		nil,
		{
			Alert:    fps[2],
			Receiver: "name",
			Resolved: true,
		},
		nsBefore[3],
		{
			Alert:    fps[4],
			Receiver: "name",
			Resolved: false,
		},
		{
			Alert:    fps[5],
			Receiver: "name",
			Resolved: false,
		},
	}

	if !reflect.DeepEqual(record.alerts, alertsExp) {
		t.Fatalf("Expected alerts %v, got %v", alertsExp, record.alerts)
	}
	nsCur, err = notifies.Get("name", fps...)
	if err != nil {
		t.Fatalf("Error getting notifies: %s", err)
	}

	for i, after := range nsAfter {
		cur := nsCur[i]

		// Hack correct timestamps back in if they are sane.
		if cur != nil && after.Timestamp.IsZero() {
			if cur.Timestamp.Before(now) {
				t.Fatalf("Wrong timestamp for notify %v", cur)
			}
			after.Timestamp = cur.Timestamp
		}

		if !reflect.DeepEqual(after, cur) {
			t.Errorf("Unexpected notifies, expected: %v, got: %v", after, cur)
		}
	}
}

func TestRoutedNotifier(t *testing.T) {
	router := Router{
		"1": &recordNotifier{},
		"2": &recordNotifier{},
		"3": &recordNotifier{},
	}

	for _, route := range []string{"3", "2", "1"} {
		var (
			ctx   = WithReceiver(context.Background(), route)
			alert = &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{"route": model.LabelValue(route)},
				},
			}
		)
		err := router.Notify(ctx, alert)
		if err != nil {
			t.Fatal(err)
		}

		rn := router[route].(*recordNotifier)
		if len(rn.alerts) != 1 && alert != rn.alerts[0] {
			t.Fatalf("Expeceted alert %v, got %v", alert, rn.alerts)
		}
	}
}

func TestMutingNotifier(t *testing.T) {
	// Mute all label sets that have a "mute" key.
	muter := types.MuteFunc(func(lset model.LabelSet) bool {
		_, ok := lset["mute"]
		return ok
	})

	record := &recordNotifier{}
	muteNotifer := Mute(muter, record)

	in := []model.LabelSet{
		{},
		{"test": "set"},
		{"mute": "me"},
		{"foo": "bar", "test": "set"},
		{"foo": "bar", "mute": "me"},
		{},
		{"not": "muted"},
	}
	out := []model.LabelSet{
		{},
		{"test": "set"},
		{"foo": "bar", "test": "set"},
		{},
		{"not": "muted"},
	}

	var inAlerts []*types.Alert
	for _, lset := range in {
		inAlerts = append(inAlerts, &types.Alert{
			Alert: model.Alert{Labels: lset},
		})
	}

	if err := muteNotifer.Notify(nil, inAlerts...); err != nil {
		t.Fatalf("Notifying failed: %s", err)
	}

	var got []model.LabelSet
	for _, a := range record.alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
}
