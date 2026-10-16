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
package silence

import (
	"time"

	"github.com/prometheus/alertmanager/silence/silencepb"
)

type SilenceState string

const (
	SilenceStateActive  SilenceState = "active"
	SilenceStateExpired SilenceState = "expired"
	SilenceStatePending SilenceState = "pending"
)

// CurrentState returns the SilenceState that a silence with the given start
// and end time would have right now.
func CurrentState(start, end time.Time) SilenceState {
	return StateAt(start, end, time.Now())
}

// StateFromProtoAt returns the silence state at the given time.
func StateFromProtoAt(sil *silencepb.Silence, at time.Time) SilenceState {
	return StateAt(sil.StartsAt.AsTime(), sil.EndsAt.AsTime(), at)
}

// StateAt returns the silence state at the given time.
// The start is inclusive. The end is exclusive.
func StateAt(start, end, at time.Time) SilenceState {
	if at.Before(start) {
		return SilenceStatePending
	}
	if at.Before(end) {
		return SilenceStateActive
	}
	return SilenceStateExpired
}
