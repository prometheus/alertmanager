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

func TestDelete(t *testing.T) {
	a := NewAlerts()
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

func TestResolved(t *testing.T) {
	a := NewAlerts()

	now := time.Now()
	require.NoError(t, a.SetOrReplaceResolved(makeAlert("a", now, -2, 10)))
	resolved := makeAlert("a", now, -2, -1)
	require.NoError(t, a.SetOrReplaceResolved(resolved))
	require.NoError(t, a.SetOrReplaceResolved(resolved))
	a.gc()
	require.ErrorIs(t, a.SetOrReplaceResolved(resolved), ErrNotFound)

	require.ErrorIs(t, a.DeleteIfResolved(resolved.Fingerprint()), ErrNotFound)
	require.NoError(t, a.SetOrReplaceResolved(makeAlert("a", now, -2, 10)))
	require.ErrorIs(t, a.DeleteIfResolved(resolved.Fingerprint()), ErrNotResolved)
	require.NoError(t, a.SetOrReplaceResolved(resolved))
	require.NoError(t, a.DeleteIfResolved(resolved.Fingerprint()))
	_, err := a.Get(resolved.Fingerprint())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGC(t *testing.T) {
	now := time.Now()

	newAlert := func(key string, start, end time.Duration) *types.Alert {
		return makeAlert(key, now, start, end)
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
	s.SetGCCallback(func(a []*types.Alert) {
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

func makeAlert(key string, now time.Time, start, end time.Duration) *types.Alert {
	return &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{model.LabelName(key): "b"},
			StartsAt: now.Add(start * time.Minute),
			EndsAt:   now.Add(end * time.Minute),
		},
	}
}
