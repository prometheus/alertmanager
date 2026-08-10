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
)

// markerKey defines a custom type with which a context is populated to
// avoid accidental collisions.
type markerKey int

const (
	keyAlertMarker markerKey = iota
)

// WithContext returns a copy of ctx carrying the given
// AlertMarker. Inhibitor and Silencer extract it from the context to
// write per-group alert status.
func WithContext(ctx context.Context, m AlertMarker) context.Context {
	return context.WithValue(ctx, keyAlertMarker, m)
}

// FromContext extracts the AlertMarker from ctx.
// If no marker is present, it returns (nil, false).
func FromContext(ctx context.Context) (AlertMarker, bool) {
	m, ok := ctx.Value(keyAlertMarker).(AlertMarker)
	return m, ok
}
