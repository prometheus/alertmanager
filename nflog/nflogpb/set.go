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
	set := map[uint64]struct{}{}
	for i := range m.FiringAlerts {
		set[m.FiringAlerts[i]] = struct{}{}
	}

	return isSubset(set, subset)
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

// NotifiedFiringAlerts returns the firing alerts the receiver was shown at the
// last notification: those that were firing and not muted.
func (m *Entry) NotifiedFiringAlerts() map[uint64]struct{} {
	return setOf(m.FiringAlerts, m.MutedAlerts)
}

// NotifiedResolvedAlerts returns the resolved alerts the receiver was shown at
// the last notification.
func (m *Entry) NotifiedResolvedAlerts() map[uint64]struct{} {
	return setOf(m.ResolvedAlerts, m.MutedAlerts)
}

// FiringAlertSet returns the alerts that were firing at the last notification,
// muted ones included.
func (m *Entry) FiringAlertSet() map[uint64]struct{} {
	return setOf(m.FiringAlerts, nil)
}

// IsSubset returns whether every member of subset is a member of set.
func IsSubset(set, subset map[uint64]struct{}) bool {
	return isSubset(set, subset)
}

// setOf turns hashes into a set, leaving out the ones in exclude.
func setOf(hashes, exclude []uint64) map[uint64]struct{} {
	excluded := make(map[uint64]struct{}, len(exclude))
	for _, h := range exclude {
		excluded[h] = struct{}{}
	}

	set := make(map[uint64]struct{}, len(hashes))
	for _, h := range hashes {
		if _, ok := excluded[h]; ok {
			continue
		}
		set[h] = struct{}{}
	}
	return set
}
