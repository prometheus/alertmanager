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

package provider

import (
	"fmt"
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

var (
	ErrNotFound = fmt.Errorf("item not found")
)

// MemAlerts implements an Alerts provider based on in-memory data.
type MemAlerts struct {
	mtx       sync.RWMutex
	alerts    map[model.Fingerprint]*types.Alert
	listeners []chan *types.Alert
}

func NewMemAlerts() *MemAlerts {
	return &MemAlerts{
		alerts: map[model.Fingerprint]*types.Alert{},
	}
}

func (a *MemAlerts) IterActive() <-chan *types.Alert {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ch := make(chan *types.Alert)

	for _, alert := range a.alerts {
		ch <- alert
	}

	a.listeners = append(a.listeners, ch)

	return ch
}

func (a *MemAlerts) Put(alert *types.Alert) error {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	a.alerts[alert.Fingerprint()] = alert

	for _, ch := range a.listeners {
		ch <- alert
	}

	return nil
}

func (a *MemAlerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	if a, ok := a.alerts[fp]; ok {
		return a, nil
	}
	return nil, ErrNotFound
}
