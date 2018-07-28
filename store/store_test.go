package store

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/stretchr/testify/require"
)

func TestSetGet(t *testing.T) {
	d := time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := NewAlerts(ctx, d)
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

	a := NewAlerts(ctx, d)
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
