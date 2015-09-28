package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
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

func TestRoutedNotifier(t *testing.T) {
	notifiers := map[string]Notifier{
		"1": &recordNotifier{},
		"2": &recordNotifier{},
		"3": &recordNotifier{},
	}
	notifierOpts := map[string]*config.NotificationConfig{
		"1": &config.NotificationConfig{
			SendResolved:   false,
			RepeatInterval: 10000,
		},
		"2": &config.NotificationConfig{
			SendResolved:   true,
			RepeatInterval: 20000,
		},
		"3": &config.NotificationConfig{
			SendResolved:   false,
			RepeatInterval: 30000,
		},
	}
	routed := &routedNotifier{
		notifiers:    notifiers,
		notifierOpts: notifierOpts,
	}

	for _, route := range []string{"3", "2", "1"} {
		var (
			ctx   = context.WithValue(context.Background(), notifyName, route)
			alert = &types.Alert{
				Labels: model.LabelSet{"route": model.LabelValue(route)},
			}
		)
		err := routed.Notify(ctx, alert)
		if err != nil {
			t.Fatal(err)
		}

		rn := routed.notifiers[route].(*recordNotifier)
		if len(rn.alerts) != 1 && alert != rn.alerts[0] {
			t.Fatalf("Expeceted alert %v, got %v", alert, rn.alerts)
		}

		// The context handed down the chain must be populated with the
		// necessary information of the notification config.
		name, ok := rn.ctx.Value(notifyName).(string)
		if !ok || name != route {
			t.Fatalf("Expected name %q, got %q", name, route)
		}

		repeatInterval, ok := rn.ctx.Value(notifyRepeatInterval).(time.Duration)
		if ri := notifierOpts[route].RepeatInterval; !ok || repeatInterval != time.Duration(ri) {
			t.Fatalf("Expected repeat interval %q, got %q", ri, repeatInterval)
		}

		sendResolved, ok := rn.ctx.Value(notifySendResolved).(bool)
		if sr := notifierOpts[route].SendResolved; !ok || sendResolved != sr {
			t.Fatalf("Expected send resolved %q, got %q", sr, sendResolved)
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
	muteNotifer := mutingNotifier{
		notifier: record,
		Muter:    muter,
	}

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
		inAlerts = append(inAlerts, &types.Alert{Labels: lset})
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
