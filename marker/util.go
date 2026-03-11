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
	"context"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
)

// alertMarkerKey is an unexported type used as context key for
// AlertMarker to avoid collisions with keys defined in other packages.
type alertMarkerKey struct{}

// WithAlertMarker returns a copy of ctx carrying the given
// AlertMarker. Inhibitor and Silencer extract it from the context to
// write per-group alert status.
func WithAlertMarker(ctx context.Context, m AlertMarker) context.Context {
	return context.WithValue(ctx, alertMarkerKey{}, m)
}

// GetAlertMarker extracts the AlertMarker from the context if present,
// otherwise returns a no-op marker.
func GetAlertMarker(ctx context.Context) AlertMarker {
	m, ok := ctx.Value(alertMarkerKey{}).(AlertMarker)
	if !ok {
		return noop
	}
	return m
}

// noop is a singleton instance of noopMarker.
var noop = &noopMarker{}

// NoopMarker is a marker that does nothing.
type noopMarker struct{}

func (noopMarker) SetInhibited(model.Fingerprint, []string) {}
func (noopMarker) SetSilenced(model.Fingerprint, []string)  {}
func (noopMarker) Status(model.Fingerprint) alert.AlertStatus {
	return alert.AlertStatus{State: alert.AlertStateUnprocessed}
}
func (noopMarker) Delete(...model.Fingerprint) {}
