// Copyright 2017 Prometheus Team
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

package nflogpb

// IsFiringSubset returns whether the given subset is a subset of the alerts
// that were firing at the time of the last notification.
func (m *Entry) IsFiringSubset(subset map[uint64]struct{}) bool {
	firingSet := map[uint64]struct{}{}
	for i := range m.FiringAlerts {
		firingSet[m.FiringAlerts[i]] = struct{}{}
	}

	// make sure the alert is not resolved from peers already
	var resolvedSet = map[uint64]struct{}{}
	for i := range m.ResolvedAlerts {
		resolvedSet[m.ResolvedAlerts[i]] = struct{}{}
	}

	var exists = true
	for k := range subset {
		if _, ok := firingSet[k]; !ok {
			// check whether the entry is from cluster peer
			if !m.Remote {
				exists = false
				break
			}
			// check whether the alert is resolved from cluster peer
			if _, ok := resolvedSet[k]; !ok {
				exists = false
				break
			}
		}
	}
	return exists
}

// IsResolvedSubset returns whether the given subset is a subset of the alerts
// that were resolved at the time of the last notification.
func (m *Entry) IsResolvedSubset(subset map[uint64]struct{}) bool {
	set := map[uint64]struct{}{}
	for i := range m.ResolvedAlerts {
		set[m.ResolvedAlerts[i]] = struct{}{}
	}

	return isSubset(set, subset)
}

func isSubset(set, subset map[uint64]struct{}) bool {
	for k := range subset {
		_, exists := set[k]
		if !exists {
			return false
		}
	}

	return true
}
