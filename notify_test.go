package main

import (
	"reflect"
	"testing"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/types"
)

type recordNotifier struct {
	alerts []*types.Alert
}

func (n *recordNotifier) Notify(ctx context.Context, as ...*types.Alert) error {
	n.alerts = append(n.alerts, as...)
	return nil
}

func TestMutingNotifier(t *testing.T) {
	// Mute all label sets that have a "mute" key.
	muter := types.MuteFunc(func(lset model.LabelSet) bool {
		if _, ok := lset["mute"]; ok {
			return true
		}
		return false
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
