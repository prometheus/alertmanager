// Copyright 2016 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/lic:wenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mem

import (
	"sync"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

// Alerts gives access to a set of alerts. All methods are goroutine-safe.
type Alerts struct {
	mtx    sync.RWMutex
	alerts map[model.Fingerprint]*types.Alert
	stopGC chan struct{}

	listeners map[int]chan *types.Alert
	next      int
}

// NewAlerts returns a new alert provider.
func NewAlerts(path string) (*Alerts, error) {
	a := &Alerts{
		alerts:    map[model.Fingerprint]*types.Alert{},
		stopGC:    make(chan struct{}),
		listeners: map[int]chan *types.Alert{},
		next:      0,
	}
	go a.runGC()

	return a, nil
}

func (a *Alerts) runGC() {
	for {
		select {
		case <-a.stopGC:
			return
		case <-time.After(30 * time.Minute):
		}

		a.mtx.Lock()

		for fp, alert := range a.alerts {
			// As we don't persist alerts, we no longer consider them after
			// they are resolved. Alerts waiting for resolved notifications are
			// held in memory in aggregation groups redundantly.
			if alert.EndsAt.Before(time.Now()) {
				delete(a.alerts, fp)
			}
		}

		a.mtx.Unlock()
	}
}

// Close the alert provider.
func (a *Alerts) Close() error {
	close(a.stopGC)
	return nil
}

// Subscribe returns an iterator over active alerts that have not been
// resolved and successfully notified about.
// They are not guaranteed to be in chronological order.
func (a *Alerts) Subscribe() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)
	alerts, err := a.getPending()

	a.mtx.Lock()
	i := a.next
	a.next++
	a.listeners[i] = ch
	a.mtx.Unlock()

	go func() {
		defer func() {
			a.mtx.Lock()
			delete(a.listeners, i)
			close(ch)
			a.mtx.Unlock()
		}()

		for _, a := range alerts {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}

		<-done
	}()

	return provider.NewAlertIterator(ch, done, err)
}

// GetPending returns an iterator over all alerts that have
// pending notifications.
func (a *Alerts) GetPending() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)

	alerts, err := a.getPending()

	go func() {
		defer close(ch)

		for _, a := range alerts {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}
	}()

	return provider.NewAlertIterator(ch, done, err)
}

func (a *Alerts) getPending() ([]*types.Alert, error) {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	res := make([]*types.Alert, 0, len(a.alerts))

	for _, alert := range a.alerts {
		res = append(res, alert)
	}

	return res, nil
}

// Get returns the alert for a given fingerprint.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	alert, ok := a.alerts[fp]
	if !ok {
		return nil, provider.ErrNotFound
	}
	return alert, nil
}

// Put adds the given alert to the set.
func (a *Alerts) Put(alerts ...*types.Alert) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		if old, ok := a.alerts[fp]; ok {
			// Merge alerts if there is an overlap in activity range.
			if (alert.EndsAt.After(old.StartsAt) && alert.EndsAt.Before(old.EndsAt)) ||
				(alert.StartsAt.After(old.StartsAt) && alert.StartsAt.Before(old.EndsAt)) {
				alert = old.Merge(alert)
			}
		}

		a.alerts[fp] = alert

		for _, ch := range a.listeners {
			ch <- alert
		}
	}

	return nil
}
