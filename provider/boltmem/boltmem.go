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
	"fmt"
	"path/filepath"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

var (
	bktNotificationInfo = []byte("notification_info")
	bktSilences         = []byte("silences")
	bktAlerts           = []byte("alerts")
)

// Alerts gives access to a set of alerts. All methods are goroutine-safe.
type Alerts struct {
	db *bolt.DB

	mtx       sync.RWMutex
	listeners map[int]chan *types.Alert
	next      int
}

// NewAlerts returns a new alert provider.
func NewAlerts(path string) (*Alerts, error) {
	db, err := bolt.Open(filepath.Join(path, "alerts.db"), 0666, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktAlerts)
		return err
	})
	return &Alerts{
		db:        db,
		listeners: map[int]chan *types.Alert{},
		next:      0,
	}, err
}

// Close the alert provider.
func (a *Alerts) Close() error {
	return a.db.Close()
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
	var alerts []*types.Alert

	err := a.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktAlerts)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var a types.Alert
			if err := json.Unmarshal(v, &a); err != nil {
				return err
			}
			alerts = append(alerts, &a)
		}

		return nil
	})
	return alerts, err
}

// Get returns the alert for a given fingerprint.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	var alert types.Alert
	err := a.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktAlerts)

		fpb := make([]byte, 8)
		binary.BigEndian.PutUint64(fpb, uint64(fp))

		ab := b.Get(fpb)
		if ab == nil {
			return provider.ErrNotFound
		}

		return json.Unmarshal(ab, &alert)
	})
	return &alert, err
}

// Put adds the given alert to the set.
func (a *Alerts) Put(alerts ...*types.Alert) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	err := a.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktAlerts)

		for _, alert := range alerts {
			fp := make([]byte, 8)
			binary.BigEndian.PutUint64(fp, uint64(alert.Fingerprint()))

			ab := b.Get(fp)

			// Merge the alert with the existing one.
			if ab != nil {
				var old types.Alert
				if err := json.Unmarshal(ab, &old); err != nil {
					return fmt.Errorf("decoding alert failed: %s", err)
				}
				// Merge alerts if there is an overlap in activity range.
				if (alert.EndsAt.After(old.StartsAt) && alert.EndsAt.Before(old.EndsAt)) ||
					(alert.StartsAt.After(old.StartsAt) && alert.StartsAt.Before(old.EndsAt)) {
					alert = old.Merge(alert)
				}
			}

			ab, err := json.Marshal(alert)
			if err != nil {
				return fmt.Errorf("encoding alert failed: %s", err)
			}

			if err := b.Put(fp, ab); err != nil {
				return fmt.Errorf("writing alert failed: %s", err)
			}

			// Send the update to all subscribers.
			for _, ch := range a.listeners {
				ch <- alert
			}
		}
		return nil
	})

	return err
}

// Silences gives access to silences. All methods are goroutine-safe.
type Silences struct {
	db *bolt.DB
	mk types.Marker
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
	return &Silences{db: db, mk: mk}, err
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

// All returns all existing silences.
func (s *Silences) All() ([]*types.Silence, error) {
	var res []*types.Silence

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ms model.Silence
			if err := json.Unmarshal(v, &ms); err != nil {
				return err
			}
			ms.ID = binary.BigEndian.Uint64(k)

			if err := json.Unmarshal(v, &ms); err != nil {
				return err
			}

			res = append(res, types.NewSilence(&ms))
		}

		return nil
	})

	return res, err
}

// Set a new silence.
func (s *Silences) Set(sil *types.Silence) (uint64, error) {
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
	return uid, err
}

// Del removes a silence.
func (s *Silences) Del(uid uint64) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)

		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, uid)

		return b.Delete(k)
	})
	return err
}

// Get a silence associated with a fingerprint.
func (s *Silences) Get(uid uint64) (*types.Silence, error) {
	var sil *types.Silence

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktSilences)

		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, uid)

		v := b.Get(k)
		if v == nil {
			return provider.ErrNotFound
		}
		var ms model.Silence

		if err := json.Unmarshal(v, &ms); err != nil {
			return err
		}
		sil = types.NewSilence(&ms)

		return nil
	})
	return sil, err
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
