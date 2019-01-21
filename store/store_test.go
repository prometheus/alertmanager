package store

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestSetGet(t *testing.T) {
	d := time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := NewAlerts(d)
	a.Run(ctx)
	alert := &types.Alert{
		UpdatedAt: time.Now(),
	}
	require.NoError(t, a.Set(alert))
	want := alert.Fingerprint()
	got, err := a.Get(want)

	require.NoError(t, err)
	require.Equal(t, want, got.Fingerprint())
}

func TestDelete(t *testing.T) {
	d := time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := NewAlerts(d)
	a.Run(ctx)
	alert := &types.Alert{
		UpdatedAt: time.Now(),
	}
	require.NoError(t, a.Set(alert))

	fp := alert.Fingerprint()

	err := a.Delete(fp)
	require.NoError(t, err)

	got, err := a.Get(fp)
	require.Nil(t, got)
	require.Equal(t, ErrNotFound, err)
}

func TestGC(t *testing.T) {
	now := time.Now()
	newAlert := func(key string, start, end time.Duration) *types.Alert {
		return &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{model.LabelName(key): "b"},
				StartsAt: now.Add(start * time.Minute),
				EndsAt:   now.Add(end * time.Minute),
			},
		}
	}
	active := []*types.Alert{
		newAlert("b", 10, 20),
		newAlert("c", -10, 10),
	}
	resolved := []*types.Alert{
		newAlert("a", -10, -5),
		newAlert("d", -10, -1),
	}
	s := NewAlerts(5 * time.Minute)
	var n int
	s.SetGCCallback(func(a []*types.Alert) {
		for range a {
			n++
		}
	})
	for _, alert := range append(active, resolved...) {
		require.NoError(t, s.Set(alert))
	}

	s.gc()

	for _, alert := range active {
		if _, err := s.Get(alert.Fingerprint()); err != nil {
			t.Errorf("alert %v should not have been gc'd", alert)
		}
	}
	for _, alert := range resolved {
		if _, err := s.Get(alert.Fingerprint()); err == nil {
			t.Errorf("alert %v should have been gc'd", alert)
		}
	}
	require.Equal(t, len(resolved), n)
}
