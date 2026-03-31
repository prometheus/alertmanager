// Copyright The Prometheus Authors
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
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUndeliveredTracker_NewDefaultGCTTL(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"zero uses one hour", 0, time.Hour},
		{"negative uses one hour", -time.Minute, time.Hour},
		{"positive unchanged", 30 * time.Minute, 30 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewUndeliveredTracker(tc.in)
			require.Equal(t, tc.want, tr.gcTTL)
		})
	}
}

func TestUndeliveredTracker_NoteFailureKeepsFirstFailTime(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(7 * time.Minute)
	tr := NewUndeliveredTracker(time.Hour)

	tr.NoteFailure("k", t0)
	require.False(t, tr.ShouldAbandon("k", 10*time.Minute, t1))

	tr.NoteFailure("k", t1)
	require.True(t, tr.ShouldAbandon("k", 5*time.Minute, t1), "first failure time should stay at t0")
}

func TestUndeliveredTracker_ShouldAbandon(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		now          time.Time
		abandonAfter time.Duration
		want         bool
	}{
		{
			name:         "unknown key",
			now:          t0,
			abandonAfter: time.Nanosecond,
			want:         false,
		},
		{
			name:         "below threshold",
			now:          t0.Add(4 * time.Minute),
			abandonAfter: 5 * time.Minute,
			want:         false,
		},
		{
			name:         "at threshold",
			now:          t0.Add(5 * time.Minute),
			abandonAfter: 5 * time.Minute,
			want:         true,
		},
		{
			name:         "above threshold",
			now:          t0.Add(6 * time.Minute),
			abandonAfter: 5 * time.Minute,
			want:         true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trFresh := NewUndeliveredTracker(time.Hour)
			key := "k"
			if tc.name == "unknown key" {
				key = "missing"
			} else {
				trFresh.NoteFailure("k", t0)
			}
			got := trFresh.ShouldAbandon(key, tc.abandonAfter, tc.now)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestUndeliveredTracker_Clear(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	tr := NewUndeliveredTracker(time.Hour)
	tr.NoteFailure("k", t0)
	require.True(t, tr.ShouldAbandon("k", time.Minute, t0.Add(2*time.Minute)))

	tr.Clear("k")
	require.False(t, tr.ShouldAbandon("k", time.Nanosecond, t0.Add(time.Hour)))

	tr.Clear("nonexistent")
}

func TestUndeliveredTracker_GCRemovesStaleEntries(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	gcTTL := 10 * time.Minute
	tr := NewUndeliveredTracker(gcTTL)

	tr.NoteFailure("stale", t0)
	// lastSeen for "stale" is t0; at t0+15m cutoff is t0+5m, so entry is removed on next op.
	tr.NoteFailure("fresh", t0.Add(15*time.Minute))

	require.False(t, tr.ShouldAbandon("stale", time.Nanosecond, t0.Add(15*time.Minute)))
	require.False(t, tr.ShouldAbandon("fresh", time.Hour, t0.Add(15*time.Minute)))
}

func TestUndeliveredTracker_ShouldAbandonRefreshesLastSeen(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	// Long gcTTL so the initial entry is not removed by gc inside ShouldAbandon before lastSeen is refreshed.
	tr := NewUndeliveredTracker(time.Hour)

	tr.NoteFailure("k", t0)
	require.False(t, tr.ShouldAbandon("k", time.Hour, t0.Add(15*time.Minute)))

	tr.NoteFailure("other", t0.Add(20*time.Minute))
	require.True(t, tr.ShouldAbandon("k", time.Minute, t0.Add(21*time.Minute)),
		"firstFail should stay at t0 after ShouldAbandon refreshed lastSeen")
}

func TestUndeliveredTracker_Concurrent(t *testing.T) {
	tr := NewUndeliveredTracker(time.Hour)
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := string(rune('a' + id%8))
			tr.NoteFailure(key, t0)
			tr.ShouldAbandon(key, 30*time.Minute, t0.Add(31*time.Minute))
			tr.Clear(key)
		}(i)
	}
	wg.Wait()
}

func TestUndeliveredTracker_MarkAbandonedAndSuppress(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	tr := NewUndeliveredTracker(time.Hour)
	firing := []uint64{3, 1, 2}

	tr.MarkAbandoned("k", firing, t0)
	require.False(t, tr.ShouldAbandon("k", time.Nanosecond, t0.Add(time.Hour)))

	require.True(t, tr.ShouldSuppressAbandoned("k", []uint64{2, 3, 1}, t0.Add(time.Minute)))
	require.False(t, tr.ShouldSuppressAbandoned("k", []uint64{1, 2}, t0.Add(2*time.Minute)))
}

func TestUndeliveredTracker_ResetIfFiringChanged(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	tr := NewUndeliveredTracker(time.Hour)
	tr.MarkAbandoned("k", []uint64{1, 2}, t0)

	tr.ResetIfFiringChanged("k", []uint64{1, 2}, t0.Add(time.Minute))
	require.True(t, tr.ShouldSuppressAbandoned("k", []uint64{2, 1}, t0.Add(2*time.Minute)))

	tr.ResetIfFiringChanged("k", []uint64{1, 2, 3}, t0.Add(3*time.Minute))
	require.False(t, tr.ShouldSuppressAbandoned("k", []uint64{1, 2, 3}, t0.Add(4*time.Minute)))
	require.False(t, tr.ShouldAbandon("k", time.Nanosecond, t0.Add(4*time.Minute)))
}

func TestUndeliveredTracker_GCRemovesAbandonedEntry(t *testing.T) {
	t0 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	gcTTL := 5 * time.Minute
	tr := NewUndeliveredTracker(gcTTL)
	tr.MarkAbandoned("k", []uint64{1}, t0)

	tr.NoteFailure("other", t0.Add(6*time.Minute))
	require.False(t, tr.ShouldSuppressAbandoned("k", []uint64{1}, t0.Add(6*time.Minute)))
}

func TestUndeliveredTracker_SortedFiringCopy(t *testing.T) {
	got := sortedFiringCopy([]uint64{9, 1, 5})
	require.True(t, slices.IsSorted(got))
	require.Equal(t, []uint64{1, 5, 9}, got)
}
