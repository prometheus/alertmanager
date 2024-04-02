// Copyright 2015 Prometheus Team
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

package types

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/matchers/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// AlertState is used as part of AlertStatus.
type AlertState string

// Possible values for AlertState.
const (
	AlertStateUnprocessed AlertState = "unprocessed"
	AlertStateActive      AlertState = "active"
	AlertStateSuppressed  AlertState = "suppressed"
)

// AlertStatus stores the state of an alert and, as applicable, the IDs of
// silences silencing the alert and of other alerts inhibiting the alert. Note
// that currently, SilencedBy is supposed to be the complete set of the relevant
// silences while InhibitedBy may contain only a subset of the inhibiting alerts
// – in practice exactly one ID. (This somewhat confusing semantics might change
// in the future.)
type AlertStatus struct {
	State       AlertState `json:"state"`
	SilencedBy  []string   `json:"silencedBy"`
	InhibitedBy []string   `json:"inhibitedBy"`

	// For internal tracking, not exposed in the API.
	pendingSilences []string
	silencesVersion int
}

// GroupStatus stores the state of the group, and, as applicable, the fingerprints
// of all muted alerts.
type GroupStatus struct {
	// MutedAlerts contains the fingerprints of all muted alerts, and for each
	// muted alert the active or mute time intervals that muted it.
	MutedAlerts map[model.Fingerprint][]string `json:"mutedAlerts"`
}

// AlertMarker helps to mark alerts as silenced and/or inhibited.
// All methods are goroutine-safe.
type AlertMarker interface {
	// SetActiveOrSilenced replaces the previous SilencedBy by the provided IDs of
	// active and pending silences, including the version number of the
	// silences state. The set of provided IDs is supposed to represent the
	// complete set of relevant silences. If no active silence IDs are provided and
	// InhibitedBy is already empty, it sets the provided alert to AlertStateActive.
	// Otherwise, it sets the provided alert to AlertStateSuppressed.
	SetActiveOrSilenced(alert model.Fingerprint, version int, activeSilenceIDs, pendingSilenceIDs []string)
	// SetInhibited replaces the previous InhibitedBy by the provided IDs of
	// alerts. In contrast to SetActiveOrSilenced, the set of provided IDs is not
	// expected to represent the complete set of inhibiting alerts. (In
	// practice, this method is only called with one or zero IDs. However,
	// this expectation might change in the future. If no IDs are provided
	// and InhibitedBy is already empty, it sets the provided alert to
	// AlertStateActive. Otherwise, it sets the provided alert to
	// AlertStateSuppressed.
	SetInhibited(alert model.Fingerprint, alertIDs ...string)

	// Count alerts of the given state(s). With no state provided, count all
	// alerts.
	Count(...AlertState) int

	// Status of the given alert.
	Status(model.Fingerprint) AlertStatus
	// Delete the given alert.
	Delete(model.Fingerprint)

	// Various methods to inquire if the given alert is in a certain
	// AlertState. Silenced also returns all the active and pending
	// silences, while Inhibited may return only a subset of inhibiting
	// alerts. Silenced also returns the version of the silences state the
	// result is based on.
	Unprocessed(model.Fingerprint) bool
	Active(model.Fingerprint) bool
	Silenced(model.Fingerprint) (activeIDs, pendingIDs []string, version int, silenced bool)
	Inhibited(model.Fingerprint) ([]string, bool)
}

type GroupMarker interface {
	// Muted returns true if the alert is muted, otherwise false. If the alert
	// is muted then it also returns the names of the time intervals that muted
	// it. If the alert is not muted then the list is nil.
	Muted(groupKey string, fingerprint model.Fingerprint) ([]string, bool)

	// SetMuted marks the alert as muted, and sets the names of the time
	// intervals that mute it. If the list of names is nil or the empty slice
	// then the muted marker is removed.
	SetMuted(groupKey string, fingerprint model.Fingerprint, timeIntervalNames []string)
}

// NewMarker returns an instance of a AlertMarker implementation.
func NewMarker(r prometheus.Registerer) AlertMarker {
	m := &memMarker{
		alerts: map[model.Fingerprint]*AlertStatus{},
		groups: map[string]*GroupStatus{},
	}
	m.registerMetrics(r)
	return m
}

type memMarker struct {
	alerts map[model.Fingerprint]*AlertStatus
	groups map[string]*GroupStatus

	mtx sync.RWMutex
}

// Muted implements GroupMarker.
func (m *memMarker) Muted(groupKey string, fingerprint model.Fingerprint) ([]string, bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	groupStatus, ok := m.groups[groupKey]
	if !ok {
		return nil, false
	}
	timeIntervalsNames, ok := groupStatus.MutedAlerts[fingerprint]
	if !ok {
		return nil, false
	}
	return timeIntervalsNames, ok
}

// SetMuted implements GroupMarker.
func (m *memMarker) SetMuted(groupKey string, fingerprint model.Fingerprint, timeIntervalNames []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	groupStatus, ok := m.groups[groupKey]
	if !ok {
		groupStatus = &GroupStatus{
			MutedAlerts: make(map[model.Fingerprint][]string),
		}
		m.groups[groupKey] = groupStatus
	}
	muted := groupStatus.MutedAlerts
	muted[fingerprint] = timeIntervalNames
	groupStatus.MutedAlerts = muted
}

func (m *memMarker) registerMetrics(r prometheus.Registerer) {
	newMarkedAlertMetricByState := func(st AlertState) prometheus.GaugeFunc {
		return prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name:        "alertmanager_marked_alerts",
				Help:        "How many alerts by state are currently marked in the Alertmanager regardless of their expiry.",
				ConstLabels: prometheus.Labels{"state": string(st)},
			},
			func() float64 {
				return float64(m.Count(st))
			},
		)
	}

	alertsActive := newMarkedAlertMetricByState(AlertStateActive)
	alertsSuppressed := newMarkedAlertMetricByState(AlertStateSuppressed)
	alertStateUnprocessed := newMarkedAlertMetricByState(AlertStateUnprocessed)

	r.MustRegister(alertsActive)
	r.MustRegister(alertsSuppressed)
	r.MustRegister(alertStateUnprocessed)
}

// Count implements AlertMarker.
func (m *memMarker) Count(states ...AlertState) int {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	if len(states) == 0 {
		return len(m.alerts)
	}

	var count int
	for _, status := range m.alerts {
		for _, state := range states {
			if status.State == state {
				count++
			}
		}
	}
	return count
}

// SetActiveOrSilenced implements AlertMarker.
func (m *memMarker) SetActiveOrSilenced(alert model.Fingerprint, version int, activeIDs, pendingIDs []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	s, found := m.alerts[alert]
	if !found {
		s = &AlertStatus{}
		m.alerts[alert] = s
	}
	s.SilencedBy = activeIDs
	s.pendingSilences = pendingIDs
	s.silencesVersion = version

	// If there are any silence or alert IDs associated with the
	// fingerprint, it is suppressed. Otherwise, set it to
	// AlertStateActive.
	if len(activeIDs) == 0 && len(s.InhibitedBy) == 0 {
		s.State = AlertStateActive
		return
	}

	s.State = AlertStateSuppressed
}

// SetInhibited implements AlertMarker.
func (m *memMarker) SetInhibited(alert model.Fingerprint, ids ...string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	s, found := m.alerts[alert]
	if !found {
		s = &AlertStatus{}
		m.alerts[alert] = s
	}
	s.InhibitedBy = ids

	// If there are any silence or alert IDs associated with the
	// fingerprint, it is suppressed. Otherwise, set it to
	// AlertStateActive.
	if len(ids) == 0 && len(s.SilencedBy) == 0 {
		s.State = AlertStateActive
		return
	}

	s.State = AlertStateSuppressed
}

// Status implements AlertMarker.
func (m *memMarker) Status(alert model.Fingerprint) AlertStatus {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	if s, found := m.alerts[alert]; found {
		return *s
	}
	return AlertStatus{
		State:       AlertStateUnprocessed,
		SilencedBy:  []string{},
		InhibitedBy: []string{},
	}
}

// Delete implements AlertMarker.
func (m *memMarker) Delete(alert model.Fingerprint) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	delete(m.alerts, alert)
}

// Unprocessed implements AlertMarker.
func (m *memMarker) Unprocessed(alert model.Fingerprint) bool {
	return m.Status(alert).State == AlertStateUnprocessed
}

// Active implements AlertMarker.
func (m *memMarker) Active(alert model.Fingerprint) bool {
	return m.Status(alert).State == AlertStateActive
}

// Inhibited implements AlertMarker.
func (m *memMarker) Inhibited(alert model.Fingerprint) ([]string, bool) {
	s := m.Status(alert)
	return s.InhibitedBy,
		s.State == AlertStateSuppressed && len(s.InhibitedBy) > 0
}

// Silenced returns whether the alert for the given Fingerprint is in the
// Silenced state, any associated silence IDs, and the silences state version
// the result is based on.
func (m *memMarker) Silenced(alert model.Fingerprint) (activeIDs, pendingIDs []string, version int, silenced bool) {
	s := m.Status(alert)
	return s.SilencedBy, s.pendingSilences, s.silencesVersion,
		s.State == AlertStateSuppressed && len(s.SilencedBy) > 0
}

// MultiError contains multiple errors and implements the error interface. Its
// zero value is ready to use. All its methods are goroutine safe.
type MultiError struct {
	mtx    sync.Mutex
	errors []error
}

// Add adds an error to the MultiError.
func (e *MultiError) Add(err error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.errors = append(e.errors, err)
}

// Len returns the number of errors added to the MultiError.
func (e *MultiError) Len() int {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	return len(e.errors)
}

// Errors returns the errors added to the MuliError. The returned slice is a
// copy of the internal slice of errors.
func (e *MultiError) Errors() []error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	return append(make([]error, 0, len(e.errors)), e.errors...)
}

func (e *MultiError) Error() string {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	es := make([]string, 0, len(e.errors))
	for _, err := range e.errors {
		es = append(es, err.Error())
	}
	return strings.Join(es, "; ")
}

// Alert wraps a model.Alert with additional information relevant
// to internal of the Alertmanager.
// The type is never exposed to external communication and the
// embedded alert has to be sanitized beforehand.
type Alert struct {
	model.Alert

	// The authoritative timestamp.
	UpdatedAt time.Time
	Timeout   bool
}

func validateLs(ls model.LabelSet) error {
	for ln, lv := range ls {
		if !compat.IsValidLabelName(ln) {
			return fmt.Errorf("invalid name %q", ln)
		}
		if !lv.IsValid() {
			return fmt.Errorf("invalid value %q", lv)
		}
	}
	return nil
}

// Validate overrides the same method in model.Alert to allow UTF-8 labels.
// This can be removed once prometheus/common has support for UTF-8.
func (a *Alert) Validate() error {
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

// A Muter determines whether a given label set is muted. Implementers that
// maintain an underlying AlertMarker are expected to update it during a call of
// Mutes.
type Muter interface {
	Mutes(model.LabelSet) bool
}

// A TimeMuter determines if the time is muted by one or more active or mute
// time intervals. If the time is muted, it returns true and the names of the
// time intervals that muted it. Otherwise, it returns false and a nil slice.
type TimeMuter interface {
	Mutes(timeIntervalNames []string, now time.Time) (bool, []string, error)
}

// A MuteFunc is a function that implements the Muter interface.
type MuteFunc func(model.LabelSet) bool

// Mutes implements the Muter interface.
func (f MuteFunc) Mutes(lset model.LabelSet) bool { return f(lset) }

// A Silence determines whether a given label set is muted.
type Silence struct {
	// A unique identifier across all connected instances.
	ID string `json:"id"`
	// A set of matchers determining if a label set is affected
	// by the silence.
	Matchers labels.Matchers `json:"matchers"`

	// Time range of the silence.
	//
	// * StartsAt must not be before creation time
	// * EndsAt must be after StartsAt
	// * Deleting a silence means to set EndsAt to now
	// * Time range must not be modified in different ways
	//
	// TODO(fabxc): this may potentially be extended by
	// creation and update timestamps.
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`

	// The last time the silence was updated.
	UpdatedAt time.Time `json:"updatedAt"`

	// Information about who created the silence for which reason.
	CreatedBy string `json:"createdBy"`
	Comment   string `json:"comment,omitempty"`

	Status SilenceStatus `json:"status"`
}

// Expired return if the silence is expired
// meaning that both StartsAt and EndsAt are equal.
func (s *Silence) Expired() bool {
	return s.StartsAt.Equal(s.EndsAt)
}

// SilenceStatus stores the state of a silence.
type SilenceStatus struct {
	State SilenceState `json:"state"`
}

// SilenceState is used as part of SilenceStatus.
type SilenceState string

// Possible values for SilenceState.
const (
	SilenceStateExpired SilenceState = "expired"
	SilenceStateActive  SilenceState = "active"
	SilenceStatePending SilenceState = "pending"
)

// CalcSilenceState returns the SilenceState that a silence with the given start
// and end time would have right now.
func CalcSilenceState(start, end time.Time) SilenceState {
	current := time.Now()
	if current.Before(start) {
		return SilenceStatePending
	}
	if current.Before(end) {
		return SilenceStateActive
	}
	return SilenceStateExpired
}
