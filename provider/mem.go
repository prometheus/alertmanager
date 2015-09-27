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

type MemData struct {
	mtx      sync.RWMutex
	alerts   map[model.Fingerprint]*types.Alert
	notifies map[string]map[model.Fingerprint]*types.Notify
}

func NewMemData() *MemData {
	return &MemData{
		alerts:   map[model.Fingerprint]*types.Alert{},
		notifies: map[string]map[model.Fingerprint]*types.Notify{},
	}
}

type memAlertIterator struct {
	ch    <-chan *types.Alert
	close func()
}

func (ai memAlertIterator) Next() <-chan *types.Alert {
	return ai.ch
}

func (ai memAlertIterator) Err() error { return nil }
func (ai memAlertIterator) Close()     { ai.close() }

// MemAlerts implements an Alerts provider based on in-memory data.
type MemAlerts struct {
	data *MemData

	mtx       sync.RWMutex
	listeners []chan *types.Alert
}

func NewMemAlerts(data *MemData) *MemAlerts {
	return &MemAlerts{
		data: data,
	}
}

func (a *MemAlerts) IterActive() AlertIterator {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.data.mtx.Lock()
	defer a.data.mtx.Unlock()

	var alerts []*types.Alert
	for _, a := range a.data.alerts {
		if !a.Resolved() {
			alerts = append(alerts, a)
		}
	}

	ch := make(chan *types.Alert)

	go func() {
		for _, a := range alerts {
			ch <- a
		}
	}()

	i := len(a.listeners)
	a.listeners = append(a.listeners, ch)

	return memAlertIterator{
		ch: ch,
		close: func() {
			a.mtx.Lock()
			a.listeners = append(a.listeners[:i], a.listeners[i+1:]...)
			close(ch)
			a.mtx.Unlock()
		},
	}
}

func (a *MemAlerts) All() ([]*types.Alert, error) {
	a.data.mtx.RLock()
	defer a.data.mtx.RUnlock()

	var alerts []*types.Alert
	for _, a := range a.data.alerts {
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (a *MemAlerts) Put(alerts ...*types.Alert) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.data.mtx.Lock()
	defer a.data.mtx.Unlock()

	for _, alert := range alerts {
		a.data.alerts[alert.Fingerprint()] = alert

		for _, ch := range a.listeners {
			ch <- alert
		}
	}

	return nil
}

func (a *MemAlerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.data.mtx.RLock()
	defer a.data.mtx.RUnlock()

	if a, ok := a.data.alerts[fp]; ok {
		return a, nil
	}
	return nil, ErrNotFound
}

type MemNotifies struct {
	data *MemData
}

func NewMemNotifies(data *MemData) *MemNotifies {
	return &MemNotifies{data: data}
}

func (n *MemNotifies) Set(dest string, ns ...*types.Notify) error {
	n.data.mtx.Lock()
	defer n.data.mtx.Unlock()

	for _, notify := range ns {
		am, ok := n.data.notifies[dest]
		if !ok {
			am = map[model.Fingerprint]*types.Notify{}
			n.data.notifies[dest] = am
		}
		am[notify.Alert] = notify
	}
	return nil
}

func (n *MemNotifies) Get(dest string, fps ...model.Fingerprint) ([]*types.Notify, error) {
	n.data.mtx.RLock()
	defer n.data.mtx.RUnlock()

	ns, ok := n.data.notifies[dest]
	if !ok {
		return nil, nil
	}

	var res []*types.Notify
	for _, fp := range fps {
		res = append(res, ns[fp])
	}

	return res, nil
}

type MemSilences struct {
	mtx      sync.RWMutex
	silences map[model.Fingerprint]*types.Silence
}

func NewMemSilences() *MemSilences {
	return &MemSilences{
		silences: map[model.Fingerprint]*types.Silence{},
	}
}

func (s *MemSilences) Mutes(lset model.LabelSet) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, sil := range s.silences {
		if sil.Mutes(lset) {
			return true
		}
	}
	return false
}

func (s *MemSilences) All() ([]*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var sils []*types.Silence
	for _, sil := range s.silences {
		sils = append(sils, sil)
	}
	return sils, nil
}

func (s *MemSilences) Set(sil *types.Silence) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if sil.ID == 0 {
		sil.ID = model.Fingerprint(len(s.silences) + 1)
	}

	s.silences[sil.ID] = sil
	return nil
}

func (s *MemSilences) Del(id model.Fingerprint) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.silences, id)
	return nil
}

func (s *MemSilences) Get(id model.Fingerprint) (*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	sil, ok := s.silences[id]
	if !ok {
		return nil, ErrNotFound
	}
	return sil, nil
}
