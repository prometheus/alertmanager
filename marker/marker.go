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
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
)

// AlertMarker tracks per-alert silenced/inhibited status within a single
// aggregation group. Each aggregation group owns its own instance.
// All methods are goroutine-safe.
type AlertMarker interface {
	// SetInhibited sets the inhibitedBy for the given fingerprint.
	// If inhibitedBy is empty, it clears the inhibitedBy.
	SetInhibited(fingerprint model.Fingerprint, inhibitedBy []string)

	// SetSilenced sets the silencedBy for the given fingerprint.
	// If silencedBy is empty, it clears the silencedBy.
	SetSilenced(fingerprint model.Fingerprint, silencedBy []string)

	// Status returns the AlertStatus for the given fingerprint.
	// If the fingerprint is not found, it returns an unknown status.
	Status(fingerprint model.Fingerprint) alert.AlertStatus

	// Delete removes markers for the given fingerprints.
	Delete(fingerprints ...model.Fingerprint)
}

// GroupMarker helps to mark groups as active or muted.
// All methods are goroutine-safe.
//
// TODO(grobinson): routeID is used in Muted and SetMuted because groupKey
// is not unique (see #3817). Once groupKey uniqueness is fixed routeID can
// be removed from the GroupMarker interface.
type GroupMarker interface {
	// Muted returns true if the group is muted, otherwise false. If the group
	// is muted then it also returns the names of the time intervals that muted
	// it.
	Muted(routeID, groupKey string) ([]string, bool)

	// SetMuted marks the group as muted, and sets the names of the time
	// intervals that mute it. If the list of names is nil or the empty slice
	// then the muted marker is removed.
	SetMuted(routeID, groupKey string, timeIntervalNames []string)

	// DeleteByGroupKey removes all markers for the GroupKey.
	DeleteByGroupKey(routeID, groupKey string)
}
