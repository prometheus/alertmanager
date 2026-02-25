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

import "sync"

// groupStatus stores the state of the group, and, as applicable, the names
// of all active and mute time intervals that are muting it.
type groupStatus struct {
	// mutedBy contains the names of all active and mute time intervals that
	// are muting it.
	mutedBy []string
}

// NewGroupMarker returns an instance of a GroupMarker implementation.
func NewGroupMarker() GroupMarker {
	return &groupMarker{
		groups: map[string]*groupStatus{},
	}
}

type groupMarker struct {
	groups map[string]*groupStatus

	mtx sync.RWMutex
}

// Muted implements GroupMarker.
func (m *groupMarker) Muted(routeID, groupKey string) ([]string, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	status, ok := m.groups[routeID+groupKey]
	if !ok {
		return nil, false
	}
	return status.mutedBy, len(status.mutedBy) > 0
}

// SetMuted implements GroupMarker.
func (m *groupMarker) SetMuted(routeID, groupKey string, timeIntervalNames []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	status, ok := m.groups[routeID+groupKey]
	if !ok {
		status = &groupStatus{}
		m.groups[routeID+groupKey] = status
	}
	status.mutedBy = timeIntervalNames
}

func (m *groupMarker) DeleteByGroupKey(routeID, groupKey string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.groups, routeID+groupKey)
}
