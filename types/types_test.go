// Copyright 2015 Prometheus Team
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

package types //nolint:revive

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestMemMarker_Muted(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)

	// No groups should be muted.
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Mark the group as muted because it's the weekend.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Other groups should not be marked as muted.
	timeIntervalNames, isMuted = marker.Muted("route1", "group2")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Other routes should not be marked as muted either.
	timeIntervalNames, isMuted = marker.Muted("route2", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// The group is no longer muted.
	marker.SetMuted("route1", "group1", nil)
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestMemMarker_DeleteByGroupKey(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)

	// Mark the group and check that it is muted.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for a different group key. The group should
	// still be muted.
	marker.DeleteByGroupKey("route1", "group2")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for the correct group key. The group should
	// no longer be muted.
	marker.DeleteByGroupKey("route1", "group1")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestMemMarker_Count(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)
	now := time.Now()

	states := []AlertState{AlertStateSuppressed, AlertStateActive, AlertStateUnprocessed}
	countByState := func(state AlertState) int {
		return marker.Count(state)
	}

	countTotal := func() int {
		var count int
		for _, s := range states {
			count += countByState(s)
		}
		return count
	}

	require.Equal(t, 0, countTotal())

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "suppressed"},
	}
	a3 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(-1 * time.Minute),
		Labels:   model.LabelSet{"test": "resolved"},
	}

	// Insert an active alert.
	marker.SetActiveOrSilenced(a1.Fingerprint(), nil)
	require.Equal(t, 1, countByState(AlertStateActive))
	require.Equal(t, 1, countTotal())

	// Insert a silenced alert.
	marker.SetActiveOrSilenced(a2.Fingerprint(), []string{"1"})
	require.Equal(t, 1, countByState(AlertStateSuppressed))
	require.Equal(t, 2, countTotal())

	// Insert a resolved silenced alert - it'll count as suppressed.
	marker.SetActiveOrSilenced(a3.Fingerprint(), []string{"1"})
	require.Equal(t, 2, countByState(AlertStateSuppressed))
	require.Equal(t, 3, countTotal())

	// Remove the silence from a3 - it'll count as active.
	marker.SetActiveOrSilenced(a3.Fingerprint(), nil)
	require.Equal(t, 2, countByState(AlertStateActive))
	require.Equal(t, 3, countTotal())
}

type fakeRegisterer struct {
	registeredCollectors []prometheus.Collector
}

func (r *fakeRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (r *fakeRegisterer) MustRegister(c ...prometheus.Collector) {
	r.registeredCollectors = append(r.registeredCollectors, c...)
}

func (r *fakeRegisterer) Unregister(prometheus.Collector) bool {
	return false
}

func TestNewMarkerRegistersMetrics(t *testing.T) {
	fr := fakeRegisterer{}
	NewMarker(&fr)

	if len(fr.registeredCollectors) == 0 {
		t.Error("expected NewMarker to register metrics on the given registerer")
	}
}
