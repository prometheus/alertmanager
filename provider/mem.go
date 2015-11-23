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
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

// MemData contains the data backing MemAlerts and MemNotifies.
type MemData struct {
	mtx      sync.RWMutex
	alerts   map[model.Fingerprint]*types.Alert
	notifies map[string]map[model.Fingerprint]*types.NotifyInfo
}

// NewMemData contains an empty but initialized MemData instance.
func NewMemData() *MemData {
	return &MemData{
		alerts:   map[model.Fingerprint]*types.Alert{},
		notifies: map[string]map[model.Fingerprint]*types.NotifyInfo{},
	}
}

// MemAlerts implements an Alerts provider based on in-memory data.
type MemAlerts struct {
	data *MemData

	mtx       sync.RWMutex
	listeners map[int]chan *types.Alert
	next      int
}

// NewMemAlerts returns a new MemAlerts based on the provided data.
func NewMemAlerts(data *MemData) *MemAlerts {
	return &MemAlerts{
		data:      data,
		listeners: map[int]chan *types.Alert{},
	}
}

// Subscribe implements the Alerts interface.
func (a *MemAlerts) Subscribe() AlertIterator {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.data.mtx.Lock()
	defer a.data.mtx.Unlock()

	var (
		alerts = a.getPending()
		ch     = make(chan *types.Alert, 200)
		done   = make(chan struct{})
	)

	i := a.next
	a.next++

	a.listeners[i] = ch

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

	return alertIterator{
		ch:   ch,
		done: done,
	}
}

// GetPending implements the Alerts interface.
func (a *MemAlerts) GetPending() AlertIterator {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.data.mtx.Lock()
	defer a.data.mtx.Unlock()

	var (
		alerts = a.getPending()
		ch     = make(chan *types.Alert, 200)
		done   = make(chan struct{})
	)

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

	return alertIterator{
		ch:   ch,
		done: done,
	}
}

func (a *MemAlerts) getPending() []*types.Alert {
	// Get fingerprints for all alerts that have pending notifications.
	over := map[model.Fingerprint]struct{}{}
	for _, ns := range a.data.notifies {
		for fp, notify := range ns {
			if notify.Resolved {
				over[fp] = struct{}{}
			}
		}
	}

	// All alerts that have pending notifications are part of the
	// new scubscription.
	var alerts []*types.Alert
	for _, a := range a.data.alerts {
		if _, ok := over[a.Fingerprint()]; !ok {
			alerts = append(alerts, a)
		}
	}

	return alerts
}

// Put implements the Alerts interface.
func (a *MemAlerts) Put(alerts ...*types.Alert) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.data.mtx.Lock()
	defer a.data.mtx.Unlock()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		// Merge the alert with the existant one.
		if old, ok := a.data.alerts[fp]; ok {
			alert = old.Merge(alert)
		}

		a.data.alerts[fp] = alert

		for _, ch := range a.listeners {
			ch <- alert
		}
	}

	return nil
}

// Get implements the Alerts interface.
func (a *MemAlerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.data.mtx.RLock()
	defer a.data.mtx.RUnlock()

	if a, ok := a.data.alerts[fp]; ok {
		return a, nil
	}
	return nil, ErrNotFound
}

// MemNotifies implements a Notifies provider based on in-memory data.
type MemNotifies struct {
	data *MemData
}

// NewMemNotifies returns a new MemNotifies based on the provided data.
func NewMemNotifies(data *MemData) *MemNotifies {
	return &MemNotifies{data: data}
}

// Set implements the Notifies interface.
func (n *MemNotifies) Set(ns ...*types.NotifyInfo) error {
	n.data.mtx.Lock()
	defer n.data.mtx.Unlock()

	for _, notify := range ns {
		if notify == nil {
			continue
		}
		am, ok := n.data.notifies[notify.Receiver]
		if !ok {
			am = map[model.Fingerprint]*types.NotifyInfo{}
			n.data.notifies[notify.Receiver] = am
		}
		am[notify.Alert] = notify
	}
	return nil
}

// Get implements the Notifies interface.
func (n *MemNotifies) Get(dest string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	n.data.mtx.RLock()
	defer n.data.mtx.RUnlock()

	res := make([]*types.NotifyInfo, len(fps))

	ns, ok := n.data.notifies[dest]
	if !ok {
		return res, nil
	}

	for i, fp := range fps {
		res[i] = ns[fp]
	}

	return res, nil
}

// MemSilences implements a Silences provider based on in-memory data.
type MemSilences struct {
	mtx      sync.RWMutex
	silences map[uint64]*model.Silence
}

// NewMemSilences returns a new MemSilences.
func NewMemSilences() *MemSilences {
	return &MemSilences{
		silences: map[uint64]*model.Silence{},
	}
}

// Mutes implements the Muter interface.
func (s *MemSilences) Mutes(lset model.LabelSet) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, sil := range s.silences {
		if types.NewSilence(sil).Mutes(lset) {
			return true
		}
	}
	return false
}

// All implements the Silences interface.
func (s *MemSilences) All() ([]*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var sils []*types.Silence
	for _, sil := range s.silences {
		sils = append(sils, types.NewSilence(sil))
	}
	return sils, nil
}

// Set impelements the Silences interface.
func (s *MemSilences) Set(sil *types.Silence) (uint64, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if sil.ID == 0 {
		sil.ID = uint64(len(s.silences) + 1)
	} else {
		if _, ok := s.silences[sil.ID]; !ok {
			return 0, ErrNotFound
		}
	}

	s.silences[sil.ID] = &sil.Silence
	return sil.ID, nil
}

// Del implements the Silences interface.
func (s *MemSilences) Del(id uint64) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.silences, id)
	return nil
}

// Get implements the Silences interface.
func (s *MemSilences) Get(id uint64) (*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	sil, ok := s.silences[id]
	if !ok {
		return nil, ErrNotFound
	}
	return types.NewSilence(sil), nil
}
