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
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
)

// Deprecated: Use alert.Alert directly.
type Alert = alert.Alert

// Deprecated: Use alert.AlertSlice directly.
type AlertSlice = alert.AlertSlice

// Deprecated: Use alert.Alerts directly.
var Alerts = alert.Alerts

// Deprecated: Use alert.AlertState constants directly.
type AlertState = alert.AlertState

// Deprecated: Use alert.AlertStateActive directly.
const AlertStateActive AlertState = alert.AlertStateActive

// Deprecated: Use alert.AlertStateSuppressed directly.
const AlertStateSuppressed AlertState = alert.AlertStateSuppressed

// Deprecated: Use alert.AlertStateUnprocessed directly.
const AlertStateUnprocessed AlertState = alert.AlertStateUnprocessed

// Deprecated: Use alert.AlertStatus directly.
type AlertStatus = alert.AlertStatus

// groupStatus stores the state of the group, and, as applicable, the names
// of all active and mute time intervals that are muting it.
type groupStatus struct {
	// mutedBy contains the names of all active and mute time intervals that
	// are muting it.
	mutedBy []string
}

// AlertMarker helps to mark alerts as silenced and/or inhibited.
// All methods are goroutine-safe.
type AlertMarker interface {
	// SetActiveOrSilenced replaces the previous SilencedBy by the provided IDs of
	// active silences. The set of provided IDs is supposed to represent the
	// complete set of relevant silences. If no active silence IDs are provided and
	// InhibitedBy is already empty, it sets the provided alert to AlertStateActive.
	// Otherwise, it sets the provided alert to AlertStateSuppressed.
	SetActiveOrSilenced(alert model.Fingerprint, activeSilenceIDs []string)
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
	Delete(...model.Fingerprint)

	// Various methods to inquire if the given alert is in a certain
	// AlertState. Silenced also returns all the active silences,
	// while Inhibited may return only a subset of inhibiting alerts.
	Unprocessed(model.Fingerprint) bool
	Active(model.Fingerprint) bool
	Silenced(model.Fingerprint) (activeIDs []string, silenced bool)
	Inhibited(model.Fingerprint) ([]string, bool)
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

// NewMarker returns an instance of a AlertMarker implementation.
func NewMarker(r prometheus.Registerer) *MemMarker {
	m := &MemMarker{
		alerts: map[model.Fingerprint]*AlertStatus{},
		groups: map[string]*groupStatus{},
	}
	m.registerMetrics(r)
	return m
}

type MemMarker struct {
	alerts map[model.Fingerprint]*AlertStatus
	groups map[string]*groupStatus

	mtx sync.RWMutex
}

// Muted implements GroupMarker.
func (m *MemMarker) Muted(routeID, groupKey string) ([]string, bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	status, ok := m.groups[routeID+groupKey]
	if !ok {
		return nil, false
	}
	return status.mutedBy, len(status.mutedBy) > 0
}

// SetMuted implements GroupMarker.
func (m *MemMarker) SetMuted(routeID, groupKey string, timeIntervalNames []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	status, ok := m.groups[routeID+groupKey]
	if !ok {
		status = &groupStatus{}
		m.groups[routeID+groupKey] = status
	}
	status.mutedBy = timeIntervalNames
}

func (m *MemMarker) DeleteByGroupKey(routeID, groupKey string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.groups, routeID+groupKey)
}

func (m *MemMarker) registerMetrics(r prometheus.Registerer) {
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
func (m *MemMarker) Count(states ...AlertState) int {
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
func (m *MemMarker) SetActiveOrSilenced(alert model.Fingerprint, activeIDs []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	s, found := m.alerts[alert]
	if !found {
		s = &AlertStatus{}
		m.alerts[alert] = s
	}
	s.SilencedBy = activeIDs

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
func (m *MemMarker) SetInhibited(alert model.Fingerprint, ids ...string) {
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
func (m *MemMarker) Status(alert model.Fingerprint) AlertStatus {
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
func (m *MemMarker) Delete(alerts ...model.Fingerprint) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, alert := range alerts {
		delete(m.alerts, alert)
	}
}

// Unprocessed implements AlertMarker.
func (m *MemMarker) Unprocessed(alert model.Fingerprint) bool {
	return m.Status(alert).State == AlertStateUnprocessed
}

// Active implements AlertMarker.
func (m *MemMarker) Active(alert model.Fingerprint) bool {
	return m.Status(alert).State == AlertStateActive
}

// Inhibited implements AlertMarker.
func (m *MemMarker) Inhibited(alert model.Fingerprint) ([]string, bool) {
	s := m.Status(alert)
	return s.InhibitedBy,
		s.State == AlertStateSuppressed && len(s.InhibitedBy) > 0
}

// Silenced returns whether the alert for the given Fingerprint is in the
// Silenced state, any associated silence IDs, and the silences state version
// the result is based on.
func (m *MemMarker) Silenced(alert model.Fingerprint) (activeIDs []string, silenced bool) {
	s := m.Status(alert)
	return s.SilencedBy,
		s.State == AlertStateSuppressed && len(s.SilencedBy) > 0
}
