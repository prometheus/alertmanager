// Copyright Prometheus Team
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

package dispatch

import (
	"sync"

	"go.uber.org/atomic"

	"github.com/prometheus/common/model"
)

// routeGroups is a map of routes to fingerprintGroups.
// It is a nested map implementation which avoid lock contention.
// The outer map is protected by a single mutex.
// The inner maps are protected by dedicated mutexes.
// The shared atomic counter is used to track the number of nested groups.
// Limits are shared between all groups.
// // Each branch of the map can hold its own R(W)Lock.
type routeGroups struct {
	mu          sync.RWMutex
	routeGroups map[*Route]*fingerprintGroups
	groupsNum   *atomic.Int64
	limits      Limits
}

// AddRoute adds a new route to the map, initializing the inner maps if needed.
// If the route already exists, it returns the existing fingerprintGroups.
func (rg *routeGroups) AddRoute(route *Route) *fingerprintGroups {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	if rg.routeGroups == nil {
		rg.routeGroups = make(map[*Route]*fingerprintGroups)
	}
	if rg.routeGroups[route] == nil {
		rg.routeGroups[route] = &fingerprintGroups{
			aggrGroups: make(map[model.Fingerprint]*aggrGroup),
			groupsNum:  rg.groupsNum,
			limits:     rg.limits,
		}
	}
	return rg.routeGroups[route]
}

// GetRoute returns the fingerprintGroups for the given route.
func (rg *routeGroups) GetRoute(route *Route) *fingerprintGroups {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return rg.routeGroups[route]
}

// Range iterates over the routeGroups.
func (rg *routeGroups) Range(fn func(*Route, *fingerprintGroups) bool) {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	for route, groups := range rg.routeGroups {
		if !fn(route, groups) {
			break
		}
	}
}

// fingerprintGroups is a map of fingerprints to aggregation groups.
// It is protected by a dedicated RW mutex.
// It inherits the shared atomic counter from the parent routeGroups to track the number of total groups.
// It inherits the limits from the parent routeGroups.
type fingerprintGroups struct {
	mu         sync.RWMutex
	aggrGroups map[model.Fingerprint]*aggrGroup
	groupsNum  *atomic.Int64
	limits     Limits
}

// LimitReached checks if the number of groups has reached the limit.
func (fg *fingerprintGroups) LimitReached() bool {
	if limit := fg.limits.MaxNumberOfAggregationGroups(); limit > 0 && fg.groupsNum.Load() >= int64(limit) {
		return true
	}
	return false
}

// AddGroup adds a new aggregation group to the map, initializing the inner maps if needed.
// If the group already exists, it returns the existing aggregation group.
func (fg *fingerprintGroups) AddGroup(fp model.Fingerprint, ag *aggrGroup) (group *aggrGroup, count int64, limit int) {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	if fg.aggrGroups == nil {
		fg.aggrGroups = make(map[model.Fingerprint]*aggrGroup)
	}
	// Check if we've reached the rate limit before creating a new group.
	if fg.LimitReached() {
		return nil, fg.groupsNum.Load(), fg.limits.MaxNumberOfAggregationGroups()
	}
	fg.aggrGroups[fp] = ag
	fg.groupsNum.Add(1)
	return ag, fg.groupsNum.Load(), fg.limits.MaxNumberOfAggregationGroups()
}

// RemoveGroup removes an aggregation group from the map.
func (fg *fingerprintGroups) RemoveGroup(fp model.Fingerprint) {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	delete(fg.aggrGroups, fp)
	fg.groupsNum.Sub(1)
}

// GetGroup returns an aggregation group by fingerprint.
func (fg *fingerprintGroups) GetGroup(fp model.Fingerprint) *aggrGroup {
	fg.mu.RLock()
	defer fg.mu.RUnlock()
	return fg.aggrGroups[fp]
}

// Range iterates over the fingerprintGroups.
func (fg *fingerprintGroups) Range(fn func(model.Fingerprint, *aggrGroup) bool) {
	fg.mu.RLock()
	defer fg.mu.RUnlock()
	for fp, ag := range fg.aggrGroups {
		if !fn(fp, ag) {
			break
		}
	}
}
