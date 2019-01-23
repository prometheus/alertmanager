package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

var (
	// ErrNotFound is returned if a Store cannot find the Alert.
	ErrNotFound = errors.New("alert not found")
)

// Alerts provides lock-coordinated to an in-memory map of alerts, keyed by
// their fingerprint. Resolved alerts are removed from the map based on
// gcInterval. An optional callback can be set which receives a slice of all
// resolved alerts that have been removed.
type Alerts struct {
	gcInterval time.Duration

	sync.Mutex
	c  map[model.Fingerprint]*types.Alert
	cb func([]*types.Alert)
}

// NewAlerts returns a new Alerts struct.
func NewAlerts(gcInterval time.Duration) *Alerts {
	if gcInterval == 0 {
		gcInterval = time.Minute
	}

	a := &Alerts{
		c:          make(map[model.Fingerprint]*types.Alert),
		cb:         func(_ []*types.Alert) {},
		gcInterval: gcInterval,
	}

	return a
}

// SetGCCallback sets a GC callback to be executed after each GC.
func (a *Alerts) SetGCCallback(cb func([]*types.Alert)) {
	a.Lock()
	defer a.Unlock()

	a.cb = cb
}

// Run starts the GC loop.
func (a *Alerts) Run(ctx context.Context) {
	go func(t *time.Ticker) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.gc()
			}
		}
	}(time.NewTicker(a.gcInterval))
}

func (a *Alerts) gc() {
	a.Lock()
	defer a.Unlock()

	resolved := []*types.Alert{}
	for fp, alert := range a.c {
		if alert.Resolved() {
			delete(a.c, fp)
			resolved = append(resolved, alert)
		}
	}
	a.cb(resolved)
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

// Delete removes the Alert with the matching fingerprint from the store.
func (a *Alerts) Delete(fp model.Fingerprint) error {
	a.Lock()
	defer a.Unlock()

	delete(a.c, fp)
	return nil
}

// List returns a buffered channel of Alerts currently held in memory.
func (a *Alerts) List() <-chan *types.Alert {
	a.Lock()
	defer a.Unlock()

	c := make(chan *types.Alert, len(a.c))
	for _, alert := range a.c {
		c <- alert
	}
	close(c)

	return c
}

// Count returns the number of items within the store.
func (a *Alerts) Count() int {
	a.Lock()
	defer a.Unlock()

	return len(a.c)
}
