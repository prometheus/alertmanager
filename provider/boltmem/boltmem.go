package boltmem

import (
	"encoding/binary"
	"path/filepath"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

// Alerts gives access to a set of alerts. All methods are goroutine-safe.
type Alerts struct {
	mtx    sync.RWMutex
	alerts map[model.Fingerprint]*types.Alert

	listeners map[int]chan *types.Alert
}

func NewAlerts() (*Alerts, error) {
	return nil, nil
}

// Subscribe returns an iterator over active alerts that have not been
// resolved and successfully notified about.
// They are not guaranteed to be in chronological order.
func (a *Alerts) Subscribe() provider.AlertIterator {
	return nil
}

// GetPending returns an iterator over all alerts that have
// pending notifications.
func (a *Alerts) GetPending() provider.AlertIterator {
	return nil
}

// Get returns the alert for a given fingerprint.
func (a *Alerts) Get(model.Fingerprint) (*types.Alert, error) {
	return nil, nil
}

// Put adds the given alert to the set.
func (a *Alerts) Put(...*types.Alert) error {
	return nil
}

// Silences gives access to silences. All methods are goroutine-safe.
type Silences struct {
	db *bolt.DB
}

func NewSilences(path string) (*Silences, error) {
	db, err := bolt.Open(filepath.Join(path, "silences.db"), 0666, nil)
	if err != nil {
		return nil, err
	}
	return &Silences{db: db}, nil
}

// The Silences provider must implement the Muter interface
// for all its silences. The data provider may have access to an
// optimized view of the data to perform this evaluation.
func (s *Silences) Mutes(lset model.LabelSet) bool {
	return false
}

// All returns all existing silences.
func (s *Silences) All() ([]*types.Silence, error) {
	return nil, nil
}

// Set a new silence.
func (s *Silences) Set(*types.Silence) (uint64, error) {
	return 0, nil
}

// Del removes a silence.
func (s *Silences) Del(uint64) error {
	return nil
}

// Get a silence associated with a fingerprint.
func (s *Silences) Get(uint64) (*types.Silence, error) {
	return nil, nil
}

// Notifies provides information about pending and successful
// notifications. All methods are goroutine-safe.
type Notifies struct {
	db *bolt.DB
}

func NewNotifies(path string) (*Notifies, error) {
	db, err := bolt.Open(filepath.Join(path, "notifies.db"), 0666, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bktNotifies)
		return err
	})
	return &Notifies{db: db}, err
}

var (
	bktNotifies = []byte("notifies")
)

func (n *Notifies) Get(recv string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	var res []*types.NotifyInfo

	err := n.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktNotifies)

		for _, fp := range fps {
			k := make([]byte, 16+len([]byte(recv)))
			binary.BigEndian.PutUint64(k, uint64(fp))
			copy(k[16:], []byte(recv))

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
func (n *Notifies) Set(ns ...*types.NotifyInfo) error {
	err := n.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktNotifies)

		for _, n := range ns {
			k := make([]byte, 16+len([]byte(n.Receiver)))
			binary.BigEndian.PutUint64(k, uint64(n.Alert))
			copy(k[16:], []byte(n.Receiver))

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
