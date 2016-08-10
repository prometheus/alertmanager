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

package boltmem

import (
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

var (
	bktNotificationInfo = []byte("notification_info")
	bktSilences         = []byte("silences")
	// bktAlerts           = []byte("alerts")
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

// Silences gives access to silences. All methods are goroutine-safe.
type Silences struct {
	db *bolt.DB
	mk types.Marker

	mtx   sync.RWMutex
	cache map[uint64]*types.Silence
	keys  []uint64
}

// NewSilences creates a new Silences provider.
func NewSilences(path string, mk types.Marker) (*Silences, error) {
	db, err := bolt.Open(filepath.Join(path, "silences.db"), 0666, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktSilences)
		return err
	})
	if err != nil {
		return nil, err
	}
	s := &Silences{
		db:    db,
		mk:    mk,
		cache: map[uint64]*types.Silence{},
		keys:  make([]uint64, 0),
	}
	return s, s.initCache()
}

// Close the silences provider.
func (s *Silences) Close() error {
	return s.db.Close()
}

// The Silences provider must implement the Muter interface
// for all its silences. The data provider may have access to an
// optimized view of the data to perform this evaluation.
func (s *Silences) Mutes(lset model.LabelSet) bool {
	sils, err := s.All()
	if err != nil {
		log.Errorf("retrieving silences failed: %s", err)
		// In doubt, do not silence anything.
		return false
	}

	for _, sil := range sils {
		if sil.Mutes(lset) {
			s.mk.SetSilenced(lset.Fingerprint(), sil.ID)
			return true
		}
	}

	s.mk.SetSilenced(lset.Fingerprint())
	return false
}

const defaultPageSize uint64 = 25

// Query implements the Silences interface.
func (s *Silences) Query(n, o, uid uint64) ([]*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if uint64(len(s.keys)) < n {
		n = uint64(len(s.keys))
	}

	// TODO: This is the second time you're searching linearly through an
	// array. Do a binary search.
	var j int
	for i, id := range s.keys {
		if id == uid {
			// Since uid is the last uid from the previous request,
			// we want to move one index higher. This is the first
			// silence in the response.
			j = i + 1
			break
		}
	}

	pageStart := uint64(j) + (defaultPageSize * o)
	if pageStart > uint64(len(s.keys)) {
		return []*types.Silence{}, nil
	}

	res := make([]*types.Silence, n, n)

	i := 0
	for _, id := range s.keys[pageStart : pageStart+n] {
		// We control the cache and the key so they shouldn't ever be
		// out of sync, i.e. we don't need to worry about existence
		// checks on the cache

		// Make sure this res[i] business is kosher and won't index out
		// of bounds. Like with a test.
		res[i] = s.cache[id]
		i++
	}
	return res, nil
}

// All returns all existing silences.
func (s *Silences) All() ([]*types.Silence, error) {
	return s.Query(uint64(len(s.cache)), 0, 1)
}

func (s *Silences) initCache() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ms model.Silence
			if err := json.Unmarshal(v, &ms); err != nil {
				return err
			}
			// The ID is duplicated in the value and always equal
			// to the stored key.
			s.cache[ms.ID] = types.NewSilence(&ms)
			s.keys = append(s.keys, ms.ID)
		}

		return nil
	})
	return err
}

// Set a new silence.
func (s *Silences) Set(sil *types.Silence) (uint64, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var (
		uid uint64
		err error
	)
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)

		// Silences are immutable and we always create a new one.
		uid, err = b.NextSequence()
		if err != nil {
			return err
		}
		sil.ID = uid

		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, uid)

		msb, err := json.Marshal(sil.Silence)
		if err != nil {
			return err
		}
		return b.Put(k, msb)
	})
	if err != nil {
		return 0, err
	}
	s.cache[uid] = sil
	s.keys = append(s.keys, uid)
	return uid, nil
}

// Del removes a silence.
func (s *Silences) Del(uid uint64) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)

		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, uid)

		return b.Delete(k)
	})
	if err != nil {
		return err
	}
	delete(s.cache, uid)
	// TODO: Yes, do a binary search, but even for 5000 keys this will
	// still be pretty fast.
	var j int
	for i, id := range s.keys {
		if id == uid {
			j = i
			break
		}
	}
	s.keys = append(s.keys[:j], s.keys[j+1:]...)
	return nil
}

// Get a silence associated with a fingerprint.
func (s *Silences) Get(uid uint64) (*types.Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	sil, ok := s.cache[uid]
	if !ok {
		return nil, provider.ErrNotFound
	}
	return sil, nil
}

// NotificationInfo provides information about pending and successful
// notifications. All methods are goroutine-safe.
type NotificationInfo struct {
	db *bolt.DB
}

// NewNotification creates a new notification info provider.
func NewNotificationInfo(path string) (*NotificationInfo, error) {
	db, err := bolt.Open(filepath.Join(path, "notification_info.db"), 0666, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktNotificationInfo)
		return err
	})
	return &NotificationInfo{db: db}, err
}

// Close the notification information provider.
func (n *NotificationInfo) Close() error {
	return n.db.Close()
}

// Get notification information for alerts and the given receiver.
func (n *NotificationInfo) Get(recv string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	var res []*types.NotifyInfo

	err := n.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktNotificationInfo)

		for _, fp := range fps {
			k := make([]byte, 8+len([]byte(recv)))
			binary.BigEndian.PutUint64(k, uint64(fp))
			copy(k[8:], []byte(recv))

			v := b.Get(k)
			if v == nil {
				res = append(res, nil)
				continue
			}

			ni := &types.NotifyInfo{
				Alert:    fp,
				Receiver: recv,
				Resolved: v[0] == 1,
			}
			if err := ni.Timestamp.UnmarshalBinary(v[1:]); err != nil {
				return err
			}
			res = append(res, ni)
		}
		return nil
	})

	return res, err
}

// Set several notifies at once. All or none must succeed.
func (n *NotificationInfo) Set(ns ...*types.NotifyInfo) error {
	err := n.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktNotificationInfo)

		for _, n := range ns {
			k := make([]byte, 8+len([]byte(n.Receiver)))
			binary.BigEndian.PutUint64(k, uint64(n.Alert))
			copy(k[8:], []byte(n.Receiver))

			var v []byte
			if n.Resolved {
				v = []byte{1}
			} else {
				v = []byte{0}
			}
			tsb, err := n.Timestamp.MarshalBinary()
			if err != nil {
				return err
			}
			v = append(v, tsb...)

			if err := b.Put(k, v); err != nil {
				return err
			}
		}

		return nil
	})
	return err
}
