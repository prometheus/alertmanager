package template

import (
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPairNames(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"name1", "name2", "name3"}
	require.EqualValues(t, expected, pairs.Names())
}

func TestPairValues(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"value1", "value2", "value3"}
	require.EqualValues(t, expected, pairs.Values())
}

func TestKVRemove(t *testing.T) {
	kv := KV{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
		"key4": "val4",
	}

	kv = kv.Remove([]string{"key2", "key4"})

	expected := []string{"key1", "key3"}
	require.EqualValues(t, expected, kv.Names())
}

func TestAlertsFiring(t *testing.T) {
	alerts := Alerts{
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertResolved)},
	}

	for _, alert := range alerts.Firing() {
		if alert.Status != string(model.AlertFiring) {
			t.Errorf("unexpected status %q", alert.Status)
		}
	}
}

func TestAlertsResolved(t *testing.T) {
	alerts := Alerts{
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertResolved)},
	}

	for _, alert := range alerts.Resolved() {
		if alert.Status != string(model.AlertResolved) {
			t.Errorf("unexpected status %q", alert.Status)
		}
	}
}
