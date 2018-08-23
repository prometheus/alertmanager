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

// Alerts implements Store using an in-memory map.
type Alerts struct {
	sync.Mutex
	c  map[model.Fingerprint]*types.Alert
	cb func([]*types.Alert)
}

// NewAlerts returns a new Alerts struct.
func NewAlerts(ctx context.Context, gcInterval time.Duration) *Alerts {
	a := &Alerts{
		c:  make(map[model.Fingerprint]*types.Alert),
		cb: func(_ []*types.Alert) {},
	}

	if gcInterval == 0 {
		gcInterval = time.Minute
	}

	go a.run(ctx, gcInterval)

	return a
}

// SetGCCallback implements Store.
func (a *Alerts) SetGCCallback(cb func([]*types.Alert)) {
	a.Lock()
	defer a.Unlock()

	a.cb = cb
}

func (a *Alerts) run(ctx context.Context, d time.Duration) {
	t := time.NewTicker(d)
	for {
		select {
		case <-ctx.Done():
			// cleanup
			return
		case <-t.C:
			a.gc()
		}
	}
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

// Get implements Store.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.Lock()
	defer a.Unlock()

	alert, prs := a.c[fp]
	if !prs {
		return nil, ErrNotFound
	}
	return alert, nil
}

// Set implements Store. It unconditionally sets the alert in memory.
func (a *Alerts) Set(alert *types.Alert) error {
	a.Lock()
	defer a.Unlock()

	a.c[alert.Fingerprint()] = alert
	return nil
}

// Delete implements Store.
func (a *Alerts) Delete(fp model.Fingerprint) error {
	a.Lock()
	defer a.Unlock()

	delete(a.c, fp)
	return nil
}

// List implements Store. It returns a buffered channel of all current Alerts.
// It should be entirely consumed.
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

// Count implements Store.
func (a *Alerts) Count() int {
	a.Lock()
	defer a.Unlock()

	return len(a.c)
}
