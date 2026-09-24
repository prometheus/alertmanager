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

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/labelset"
)

// Alert wraps a model.Alert with additional information relevant
// to internal of the Alertmanager.
// The type is never exposed to external communication and the
// embedded alert has to be sanitized beforehand.
type Alert struct {
	model.Alert

	// The authoritative timestamp.
	UpdatedAt time.Time
	Timeout   bool

	// The fingerprint of Labels, computed once by New. The labels must not
	// be modified after construction.
	fingerprint model.Fingerprint
}

// New returns an Alert wrapping the given model.Alert. All Alerts must be
// created through New so that derived internal state can be computed here.
// The labels must not be modified afterwards.
func New(a model.Alert, updatedAt time.Time, timeout bool) *Alert {
	return &Alert{
		Alert:       a,
		UpdatedAt:   updatedAt,
		Timeout:     timeout,
		fingerprint: a.Labels.Fingerprint(),
	}
}

// Fingerprint returns the fingerprint of the alert's labels, computed at
// construction time.
func (a *Alert) Fingerprint() model.Fingerprint {
	return a.fingerprint
}

// LabelSet returns the alert's labels together with its fingerprint, so that
// consumers which only need the labels do not hash them again.
func (a *Alert) LabelSet() labelset.LabelSet {
	return labelset.New(a.Labels, a.fingerprint)
}

// Merge merges the timespan of two alerts based and overwrites annotations
// based on the authoritative timestamp.  A new alert is returned, the labels
// are assumed to be equal.
func (a *Alert) Merge(o *Alert) *Alert {
	// Let o always be the younger alert.
	if o.UpdatedAt.Before(a.UpdatedAt) {
		return o.Merge(a)
	}

	res := *o

	// Always pick the earliest starting time.
	if a.StartsAt.Before(o.StartsAt) {
		res.StartsAt = a.StartsAt
	}

	if o.Resolved() {
		// The latest explicit resolved timestamp wins if both alerts are effectively resolved.
		if a.Resolved() && a.EndsAt.After(o.EndsAt) {
			res.EndsAt = a.EndsAt
		}
	} else {
		// A non-timeout timestamp always rules if it is the latest.
		if a.EndsAt.After(o.EndsAt) && !a.Timeout {
			res.EndsAt = a.EndsAt
		}
	}

	return &res
}

// Validate checks a model.Alert like model.Alert.Validate, but validates label
// names according to the matcher compatibility mode selected by the feature
// flags: classic mode rejects UTF-8 names, the other modes allow them.
// model.Alert.Validate follows the deprecated process-wide
// model.NameValidationScheme instead, which Alertmanager does not set.
func Validate(a *model.Alert) error {
	if a.StartsAt.IsZero() {
		return fmt.Errorf("start time missing")
	}
	if !a.EndsAt.IsZero() && a.EndsAt.Before(a.StartsAt) {
		return fmt.Errorf("start time must be before end time")
	}
	if len(a.Labels) == 0 {
		return fmt.Errorf("at least one label pair required")
	}
	if err := validateLs(a.Labels); err != nil {
		return fmt.Errorf("invalid label set: %w", err)
	}
	if err := validateLs(a.Annotations); err != nil {
		return fmt.Errorf("invalid annotations: %w", err)
	}
	return nil
}

// Validate overrides the same method in model.Alert so that label names are
// validated according to the configured matcher compatibility mode. See
// Validate.
func (a *Alert) Validate() error {
	return Validate(&a.Alert)
}

// AlertSlice is a sortable slice of Alerts.
type AlertSlice []*Alert

func (as AlertSlice) Less(i, j int) bool {
	// Look at labels.job, then labels.instance.
	for _, overrideKey := range [...]model.LabelName{"job", "instance"} {
		iVal, iOk := as[i].Labels[overrideKey]
		jVal, jOk := as[j].Labels[overrideKey]
		if !iOk && !jOk {
			continue
		}
		if !iOk {
			return false
		}
		if !jOk {
			return true
		}
		if iVal != jVal {
			return iVal < jVal
		}
	}
	return as[i].Labels.Before(as[j].Labels)
}
func (as AlertSlice) Swap(i, j int) { as[i], as[j] = as[j], as[i] }
func (as AlertSlice) Len() int      { return len(as) }

// LogValue implements slog.LogValuer. It returns a summary of alert counts per alertname,
// e.g. "MyAlert: 3, OtherAlert: 1".
func (as AlertSlice) LogValue() slog.Value {
	if len(as) == 0 {
		return slog.StringValue("")
	}

	counts := make(map[string]int, len(as))
	order := make([]string, 0, len(as))

	for _, a := range as {
		name := string(a.Labels[model.AlertNameLabel])
		if _, exists := counts[name]; !exists {
			order = append(order, name)
		}
		counts[name]++
	}

	var sb strings.Builder
	// Pre-size the builder to reduce re-allocations.
	// Rough guess: (avg name length + ": " + "count" + ", ") * unique alerts
	sb.Grow(len(order) * 20)

	for i, name := range order {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(name)
		sb.WriteString(": ")
		sb.WriteString(strconv.Itoa(counts[name]))
	}

	return slog.StringValue(sb.String())
}

// Alerts turns a sequence of internal alerts into a list of
// exposable model.Alert structures.
func Alerts(alerts ...*Alert) model.Alerts {
	res := make(model.Alerts, 0, len(alerts))
	for _, a := range alerts {
		v := a.Alert
		// If the end timestamp is not reached yet, do not expose it.
		if !a.Resolved() {
			v.EndsAt = time.Time{}
		}
		res = append(res, &v)
	}
	return res
}
