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

	"github.com/prometheus/alertmanager/alert"
)

func TestSetGet(t *testing.T) {
	a := NewAlerts()
	alert := &alert.Alert{
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
		a1 := &alert.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(-time.Second),
		}
		require.NoError(t, a.Set(a1))

		// a1 should be deleted as it has not been modified.
		a.DeleteIfNotModified(alert.AlertSlice{a1}, false)
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
	})

	t.Run("modified alert should not be deleted", func(t *testing.T) {
		a := NewAlerts()
		a1 := &alert.Alert{
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
		a2 := &alert.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(-time.Second),
		}
		require.True(t, a2.UpdatedAt.Before(a1.UpdatedAt))
		a.DeleteIfNotModified(alert.AlertSlice{a2}, false)
		// a1 should not be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)

		// Make another copy of a1 that is older, but do not put it.
		// We want to make sure a2 is not deleted here either.
		a3 := &alert.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now().Add(time.Second),
		}
		require.True(t, a3.UpdatedAt.After(a1.UpdatedAt))
		a.DeleteIfNotModified(alert.AlertSlice{a3}, false)
		// a1 should not be deleted.
		got, err = a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)
	})

	t.Run("should not delete other alerts", func(t *testing.T) {
		a := NewAlerts()
		a1 := &alert.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
			},
			UpdatedAt: time.Now(),
		}
		a2 := &alert.Alert{
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
		require.NoError(t, a.DeleteIfNotModified(alert.AlertSlice{a1}, true))
		// a1 should be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.False(t, a.Destroyed())
		require.Nil(t, got)
		// a2 should not be deleted.
		got, err = a.Get(a2.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a2, got)
	})
}

func TestPerAlertLimitResolved(t *testing.T) {
	newAlert := func(instance string, resolved bool) *alert.Alert {
		end := time.Now().Add(time.Hour)
		if resolved {
			end = time.Now().Add(-time.Hour)
		}
		return &alert.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "Test", "instance": model.LabelValue(instance)},
				StartsAt: time.Now().Add(-2 * time.Hour),
				EndsAt:   end,
			},
			UpdatedAt: time.Now(),
		}
	}

	t.Run("resolved frees the slot held by an admitted firing alert", func(t *testing.T) {
		a := NewAlerts().WithPerAlertLimit(1)

		// The first firing alert is admitted and fills the bucket.
		require.NoError(t, a.Set(newAlert("server1", false)))

		// A second firing alert is limited while the bucket is full.
		require.ErrorIs(t, a.Set(newAlert("server2", false)), ErrLimited)

		// Resolving the admitted alert frees its slot and is stored.
		require.NoError(t, a.Set(newAlert("server1", true)))

		// The freed slot now admits another firing alert.
		require.NoError(t, a.Set(newAlert("server2", false)))
	})

	t.Run("resolved without an admitted firing counterpart is dropped", func(t *testing.T) {
		a := NewAlerts().WithPerAlertLimit(1)

		require.ErrorIs(t, a.Set(newAlert("ghost", true)), ErrLimited)
		require.Zero(t, a.Len())
	})

	t.Run("resolving twice drops the duplicate resolution", func(t *testing.T) {
		a := NewAlerts().WithPerAlertLimit(1)

		require.NoError(t, a.Set(newAlert("server1", false)))
		require.NoError(t, a.Set(newAlert("server1", true)))
		// The slot was already freed by the first resolution.
		require.ErrorIs(t, a.Set(newAlert("server1", true)), ErrLimited)
	})
}

func TestGC(t *testing.T) {
	now := time.Now()
	newAlert := func(key string, start, end time.Duration) *alert.Alert {
		return &alert.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{model.LabelName(key): "b"},
				StartsAt: now.Add(start * time.Minute),
				EndsAt:   now.Add(end * time.Minute),
			},
		}
	}
	active := []*alert.Alert{
		newAlert("b", 10, 20),
		newAlert("c", -10, 10),
	}
	resolved := []*alert.Alert{
		newAlert("a", -10, -5),
		newAlert("d", -10, -1),
	}
	s := NewAlerts()
	var (
		n           int
		done        = make(chan struct{})
		ctx, cancel = context.WithCancel(context.Background())
	)
	s.SetGCCallback(func(a []*alert.Alert) {
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
