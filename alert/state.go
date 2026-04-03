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

package alert

// AlertState is used as part of AlertStatus.
type AlertState string

// Possible values for AlertState.
const (
	AlertStateUnprocessed AlertState = "unprocessed"
	AlertStateActive      AlertState = "active"
	AlertStateSuppressed  AlertState = "suppressed"
)

// Compare returns -1 if s has lower priority than other, 0 if equal,
// and 1 if higher. Priority order: suppressed > active > unprocessed.
func (s AlertState) Compare(other AlertState) int {
	p := func(st AlertState) int {
		switch st {
		case AlertStateSuppressed:
			return 2
		case AlertStateActive:
			return 1
		default:
			return 0
		}
	}
	switch a, b := p(s), p(other); {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
