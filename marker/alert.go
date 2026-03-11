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
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
)

// NewAlertMarker returns a new AlertMarker backed by an in-memory map.
func NewAlertMarker() AlertMarker {
	return &alertMarker{
		status: map[model.Fingerprint]*alertStatus{},
	}
}

// alertMarker is an in-memory implementation of AlertMarker.
type alertMarker struct {
	status map[model.Fingerprint]*alertStatus
	mtx    sync.RWMutex
}

// SetSilenced implements AlertMarker.
func (m *alertMarker) SetSilenced(fp model.Fingerprint, silencedBy []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	s, found := m.status[fp]
	if !found {
		s = &alertStatus{}
		m.status[fp] = s
	}
	s.SilencedBy = silencedBy
}

// SetInhibited implements AlertMarker.
func (m *alertMarker) SetInhibited(fp model.Fingerprint, inhibitedBy []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	s, found := m.status[fp]
	if !found {
		s = &alertStatus{}
		m.status[fp] = s
	}
	s.InhibitedBy = inhibitedBy
}

// Status implements AlertMarker.
func (m *alertMarker) Status(fp model.Fingerprint) alert.AlertStatus {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	status := alert.AlertStatus{
		State:       alert.AlertStateUnprocessed,
		SilencedBy:  []string{},
		InhibitedBy: []string{},
	}

	s, found := m.status[fp]
	if !found {
		return status
	}

	status.State = s.state()
	status.SilencedBy = s.SilencedBy
	status.InhibitedBy = s.InhibitedBy
	return status
}

// Delete implements AlertMarker.
func (m *alertMarker) Delete(alerts ...model.Fingerprint) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, alert := range alerts {
		delete(m.status, alert)
	}
}
