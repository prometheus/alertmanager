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

package kafka

import (
	"sort"
	"strings"
)

// BrokerList returns a deterministic representation of the broker
// list suitable for use in metric label values: brokers sorted
// alphabetically and joined with commas.  Callers that want a fuller
// identifier (e.g. "kafka:<brokers>/<topic>") compose around this
// helper.
func BrokerList(brokers []string) string {
	sorted := append([]string(nil), brokers...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// BrokerListsEqual reports whether two broker lists are
// content-equal, ignoring order.  Useful for hot-reload diffing where
// reordering a YAML broker list is semantically a no-op.
func BrokerListsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
