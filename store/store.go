// Copyright 2018 Prometheus Team
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

package store

import (
	"errors"
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

// ErrNotFound is returned if a Store cannot find the Alert.
var ErrNotFound = errors.New("alert not found")

// Alerts provides lock-coordinated to an in-memory map of alerts, keyed by
// their fingerprint.
type Alerts struct {
	sync.Mutex
	c map[model.Fingerprint]*types.Alert
}

// NewAlerts returns a new Alerts struct.
func NewAlerts() *Alerts {
	return &Alerts{
		c: make(map[model.Fingerprint]*types.Alert),
	}
}

// Get returns the Alert with the matching fingerprint, or an error if it is
// not found.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.Lock()
	defer a.Unlock()

	alert, prs := a.c[fp]
	if !prs {
		return nil, ErrNotFound
	}
	return alert, nil
}

// Set unconditionally sets the alert in memory.
func (a *Alerts) Set(alert *types.Alert) error {
	a.Lock()
	defer a.Unlock()

	a.c[alert.Fingerprint()] = alert
	return nil
}

func (a *Alerts) Delete(fp model.Fingerprint) {
	a.Lock()
	defer a.Unlock()
	delete(a.c, fp)
}

// DeleteIfNotModified deletes the slice of Alerts from the store if not
// modified.
func (a *Alerts) DeleteIfNotModified(alerts types.AlertSlice) error {
	a.Lock()
	defer a.Unlock()
	for _, alert := range alerts {
		fp := alert.Fingerprint()
		if other, ok := a.c[fp]; ok && alert.UpdatedAt == other.UpdatedAt {
			delete(a.c, fp)
		}
	}
	return nil
}

// DeleteResolved deletes all resolved alerts.
func (a *Alerts) DeleteResolved() {
	a.Lock()
	defer a.Unlock()
	for fp, alert := range a.c {
		if alert.Resolved() {
			delete(a.c, fp)
		}
	}
}

// List returns a slice of Alerts currently held in memory.
func (a *Alerts) List() []*types.Alert {
	a.Lock()
	defer a.Unlock()

	alerts := make([]*types.Alert, 0, len(a.c))
	for _, alert := range a.c {
		alerts = append(alerts, alert)
	}

	return alerts
}

// Empty returns true if the store is empty.
func (a *Alerts) Empty() bool {
	a.Lock()
	defer a.Unlock()

	return len(a.c) == 0
}
