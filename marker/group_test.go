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

package marker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupMarker_Muted(t *testing.T) {
	marker := NewGroupMarker()

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

func TestGroupMarker_DeleteByGroupKey(t *testing.T) {
	marker := NewGroupMarker()

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
