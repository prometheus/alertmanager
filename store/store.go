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
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/limit"
	"github.com/prometheus/alertmanager/types"
)

// ErrLimited is returned if a Store has reached the per-alert limit.
var ErrLimited = errors.New("alert limited")

// ErrNotFound is returned if a Store cannot find the Alert.
var ErrNotFound = errors.New("alert not found")

// ErrDestroyed is returned if a Store has been destroyed.
var ErrDestroyed = errors.New("alert store destroyed")

// Alerts provides lock-coordinated to an in-memory map of alerts, keyed by
// their fingerprint. Resolved alerts are removed from the map based on
// gcInterval. An optional callback can be set which receives a slice of all
// resolved alerts that have been removed.
type Alerts struct {
	sync.Mutex
	alerts        map[model.Fingerprint]*types.Alert
	gcCallback    func([]*types.Alert)
	limits        map[string]*limit.Bucket[model.Fingerprint]
	perAlertLimit int
	destroyed     bool
}

// NewAlerts returns a new Alerts struct.
func NewAlerts() *Alerts {
	a := &Alerts{
		alerts:        make(map[model.Fingerprint]*types.Alert),
		gcCallback:    func(_ []*types.Alert) {},
		perAlertLimit: 0,
	}

	return a
}

// WithPerAlertLimit sets the per-alert limit for the Alerts struct.
func (a *Alerts) WithPerAlertLimit(lim int) *Alerts {
	a.Lock()
	defer a.Unlock()

	a.limits = make(map[string]*limit.Bucket[model.Fingerprint])
	a.perAlertLimit = lim

	return a
}

// SetGCCallback sets a GC callback to be executed after each GC.
func (a *Alerts) SetGCCallback(cb func([]*types.Alert)) {
	a.Lock()
	defer a.Unlock()

	a.gcCallback = cb
}

// Run starts the GC loop. The interval must be greater than zero; if not, the function will panic.
// Note: This is only used by inhibitor currently and potentially can be removed later.
func (a *Alerts) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.GC()
		}
	}
}

// GC deletes resolved alerts and returns them.
func (a *Alerts) GC() (deleted []*types.Alert) {
	// Remove stale alert limit buckets.
	a.gcLimitBuckets()

	// Delete resolved alerts.
	deleted = a.gcAlerts()

	// Execute GC callback if needed.
	if len(deleted) > 0 {
		a.gcCallback(deleted)
	}

	return deleted
}

// gcAlerts deletes resolved alerts and returns a copy of them.
func (a *Alerts) gcAlerts() (deleted []*types.Alert) {
	a.Lock()
	defer a.Unlock()
	for fp, alert := range a.alerts {
		if alert.Resolved() {
			deleted = append(deleted, alert)
			delete(a.alerts, fp)
		}
	}
	return deleted
}

// gcLimitBuckets removes stale alert limit buckets.
func (a *Alerts) gcLimitBuckets() {
	a.Lock()
	defer a.Unlock()

	for alertName, bucket := range a.limits {
		if bucket.IsStale() {
			delete(a.limits, alertName)
		}
	}
}

// Get returns the Alert with the matching fingerprint, or an error if it is
// not found.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.Lock()
	defer a.Unlock()

	alert, prs := a.alerts[fp]
	if !prs {
		return nil, ErrNotFound
	}
	return alert, nil
}

// Set unconditionally sets the alert in memory.
func (a *Alerts) Set(alert *types.Alert) error {
	a.Lock()
	defer a.Unlock()

	if a.destroyed {
		return ErrDestroyed
	}

	fp := alert.Fingerprint()
	name := alert.Name()

	// Apply per alert limits if necessary
	if a.perAlertLimit > 0 {
		bucket, ok := a.limits[name]
		if !ok {
			bucket = limit.NewBucket[model.Fingerprint](a.perAlertLimit)
			a.limits[name] = bucket
		}
		if !bucket.Upsert(fp, alert.EndsAt) {
			return ErrLimited
		}
	}

	a.alerts[fp] = alert
	return nil
}

// DeleteIfNotModified deletes the slice of Alerts from the store if not
// modified.
func (a *Alerts) DeleteIfNotModified(alerts types.AlertSlice, destroyIfEmpty bool) error {
	a.Lock()
	defer a.Unlock()
	for _, alert := range alerts {
		fp := alert.Fingerprint()
		if other, ok := a.alerts[fp]; ok && alert.UpdatedAt.Equal(other.UpdatedAt) {
			delete(a.alerts, fp)
		}
	}

	// If the store is now empty, mark it as destroyed
	if len(a.alerts) == 0 && destroyIfEmpty {
		a.destroyed = true
	}

	return nil
}

// List returns a slice of Alerts currently held in memory.
func (a *Alerts) List() []*types.Alert {
	a.Lock()
	defer a.Unlock()

	alerts := make([]*types.Alert, 0, len(a.alerts))
	for _, alert := range a.alerts {
		alerts = append(alerts, alert)
	}

	return alerts
}

// Empty returns true if the store is empty.
func (a *Alerts) Empty() bool {
	a.Lock()
	defer a.Unlock()

	return len(a.alerts) == 0
}

// Empty returns true if the store is empty.
func (a *Alerts) Destroyed() bool {
	a.Lock()
	defer a.Unlock()

	return a.destroyed
}

// Len returns the number of alerts in the store.
func (a *Alerts) Len() int {
	a.Lock()
	defer a.Unlock()

	return len(a.alerts)
}
