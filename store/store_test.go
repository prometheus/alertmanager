// Copyright 2018 Prometheus Team
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

package store

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/types"
)

func TestSetGet(t *testing.T) {
	a := NewAlerts()
	alert := &types.Alert{
		UpdatedAt: time.Now(),
	}
	require.NoError(t, a.Set(alert))
	want := alert.Fingerprint()
	got, err := a.Get(want)

	require.NoError(t, err)
	require.Equal(t, want, got.Fingerprint())
}

func TestDeleteIfNotModified(t *testing.T) {
	t.Run("unmodified alert should be deleted", func(t *testing.T) {
		a := NewAlerts()
		a1 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(-time.Second),
		}
		require.NoError(t, a.Set(a1))

		// a1 should be deleted as it has not been modified.
		deleted := a.DeleteIfNotModified(types.AlertSlice{a1})
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
		require.Equal(t, []model.Fingerprint{a1.Fingerprint()}, deleted)
	})

	t.Run("modified alert should not be deleted", func(t *testing.T) {
		a := NewAlerts()
		a1 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now(),
		}
		require.NoError(t, a.Set(a1))

		// Make a copy of a1 that is older, but do not put it.
		// We want to make sure a1 is not deleted.
		a2 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(-time.Second),
		}
		require.True(t, a2.UpdatedAt.Before(a1.UpdatedAt))
		deleted := a.DeleteIfNotModified(types.AlertSlice{a2})
		// a1 should not be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)
		require.Empty(t, deleted)

		// Make another copy of a1 that is older, but do not put it.
		// We want to make sure a2 is not deleted here either.
		a3 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(time.Second),
		}
		require.True(t, a3.UpdatedAt.After(a1.UpdatedAt))
		deleted = a.DeleteIfNotModified(types.AlertSlice{a3})
		// a1 should not be deleted.
		got, err = a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)
		require.Empty(t, deleted)
	})

	t.Run("should not delete other alerts", func(t *testing.T) {
		a := NewAlerts()
		a1 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now(),
		}
		a2 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"bar": "baz",
				},
			},
			UpdatedAt: time.Now(),
		}
		require.NoError(t, a.Set(a1))
		require.NoError(t, a.Set(a2))

		// Deleting a1 should not delete a2.
		deleted := a.DeleteIfNotModified(types.AlertSlice{a1})
		// a1 should be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
		require.Equal(t, []model.Fingerprint{a1.Fingerprint()}, deleted)
		// a2 should not be deleted.
		got, err = a.Get(a2.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a2, got)
	})
}

func TestDeleteIfStale(t *testing.T) {
	const ttl = time.Hour
	now := time.Now()
	staleTime := now.Add(-2 * ttl)
	freshTime := now.Add(ttl)

	newAlert := func(key, val string, updatedAt time.Time) *types.Alert {
		return &types.Alert{
			Alert:     model.Alert{Labels: model.LabelSet{model.LabelName(key): model.LabelValue(val)}},
			UpdatedAt: updatedAt,
		}
	}

	t.Run("stale alert is deleted", func(t *testing.T) {
		a := NewAlerts()
		alert := newAlert("foo", "bar", staleTime)
		require.NoError(t, a.Set(alert))

		deleted := a.DeleteIfStale(types.AlertSlice{alert}, ttl)

		_, err := a.Get(alert.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Equal(t, []model.Fingerprint{alert.Fingerprint()}, deleted)
	})

	t.Run("fresh alert is kept", func(t *testing.T) {
		a := NewAlerts()
		alert := newAlert("foo", "bar", freshTime)
		require.NoError(t, a.Set(alert))

		deleted := a.DeleteIfStale(types.AlertSlice{alert}, ttl)

		got, err := a.Get(alert.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, alert, got)
		require.Empty(t, deleted)
	})

	t.Run("modified alert is not deleted even if stale", func(t *testing.T) {
		a := NewAlerts()
		old := newAlert("foo", "bar", staleTime)
		newer := newAlert("foo", "bar", freshTime)
		require.NoError(t, a.Set(newer))

		deleted := a.DeleteIfStale(types.AlertSlice{old}, ttl)

		got, err := a.Get(newer.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, newer, got)
		require.Empty(t, deleted)
	})

	t.Run("only stale alerts are deleted, fresh ones are kept", func(t *testing.T) {
		a := NewAlerts()
		stale := newAlert("stale", "yes", staleTime)
		fresh := newAlert("fresh", "yes", freshTime)
		require.NoError(t, a.Set(stale))
		require.NoError(t, a.Set(fresh))

		deleted := a.DeleteIfStale(types.AlertSlice{stale, fresh}, ttl)

		_, err := a.Get(stale.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		got, err := a.Get(fresh.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, fresh, got)
		require.Equal(t, []model.Fingerprint{stale.Fingerprint()}, deleted)
	})
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
	s := NewAlerts()
	var (
		n           int
		done        = make(chan struct{})
		ctx, cancel = context.WithCancel(context.Background())
	)
	s.SetGCCallback(func(a []types.Alert) {
		n += len(a)
		if n >= len(resolved) {
			cancel()
		}
	})
	for _, alert := range append(active, resolved...) {
		require.NoError(t, s.Set(alert))
	}
	go func() {
		s.Run(ctx, 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
		break
	case <-time.After(1 * time.Second):
		t.Fatal("garbage collection didn't complete in time")
	}

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
	require.Len(t, resolved, n)
}
