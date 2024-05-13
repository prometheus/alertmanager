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
		a.DeleteIfNotModified(types.AlertSlice{a1})
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
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
		a.DeleteIfNotModified(types.AlertSlice{a2})
		// a1 should not be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)

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
		a.DeleteIfNotModified(types.AlertSlice{a3})
		// a1 should not be deleted.
		got, err = a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)
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
		require.NoError(t, a.DeleteIfNotModified(types.AlertSlice{a1}))
		// a1 should be deleted.
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
		// a2 should not be deleted.
		got, err = a.Get(a2.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a2, got)
	})
}

func TestDeleteResolved(t *testing.T) {
	t.Run("active alert should not be deleted", func(t *testing.T) {
		a := NewAlerts()
		a1 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(5 * time.Minute),
			},
		}
		require.NoError(t, a.Set(a1))
		a.DeleteResolved()
		// a1 should not have been deleted.
		got, err := a.Get(a1.Fingerprint())
		require.NoError(t, err)
		require.Equal(t, a1, got)
	})

	t.Run("resolved alert should not be deleted", func(t *testing.T) {
		a := NewAlerts()
		a1 := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"foo": "bar",
				},
				StartsAt: time.Now().Add(-5 * time.Minute),
				EndsAt:   time.Now().Add(-time.Second),
			},
		}
		require.NoError(t, a.Set(a1))
		a.DeleteResolved()
		// a1 should have been deleted.
		got, err := a.Get(a1.Fingerprint())
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, got)
	})
}
