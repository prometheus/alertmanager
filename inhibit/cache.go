// Copyright The Prometheus Authors
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

package inhibit

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
)

// cache contains the runtime state of the inhibit rule.
type cache struct {
	equal map[model.LabelName]struct{}

	mtx sync.RWMutex
	// alerts is the map of alerts that match the source matchers of the inhibit rule.
	alerts map[model.Fingerprint]*alert.Alert
	// index is a map of equal label fingerprint to the set of source alert fingerprints stored
	// in the cache.
	index map[model.Fingerprint]model.FingerprintSet
}

func newCache(equal map[model.LabelName]struct{}) *cache {
	return &cache{
		equal:  equal,
		alerts: make(map[model.Fingerprint]*alert.Alert),
		index:  make(map[model.Fingerprint]model.FingerprintSet),
	}
}

// fingerprintEquals returns the fingerprint of the equal labels of the given label set.
func (c *cache) fingerprintEquals(lset model.LabelSet) model.Fingerprint {
	equalSet := make(model.LabelSet, len(c.equal))
	for n := range c.equal {
		equalSet[n] = lset[n]
	}
	return equalSet.Fingerprint()
}

// set adds or replaces the given source alert.
func (c *cache) set(a *alert.Alert) {
	fp := a.Fingerprint()
	eq := c.fingerprintEquals(a.Labels)

	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.alerts[fp] = a
	set, ok := c.index[eq]
	if !ok {
		set = model.FingerprintSet{}
		c.index[eq] = set
	}
	set[fp] = struct{}{}
}

// find returns the fingerprint of a cached source alert that shares the equal
// labels of lset, is active at now, and satisfies match.
func (c *cache) find(lset model.LabelSet, now time.Time, match func(*alert.Alert) bool) (model.Fingerprint, bool) {
	eq := c.fingerprintEquals(lset)

	c.mtx.RLock()
	defer c.mtx.RUnlock()

	for fp := range c.index[eq] {
		a := c.alerts[fp]
		if a.ResolvedAt(now) {
			continue
		}
		if !match(a) {
			continue
		}
		return fp, true
	}

	return model.Fingerprint(0), false
}

func (c *cache) run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.gc()
		}
	}
}

func (c *cache) gc() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	for fp, a := range c.alerts {
		if !a.Resolved() {
			continue
		}
		delete(c.alerts, fp)

		eq := c.fingerprintEquals(a.Labels)
		set := c.index[eq]
		delete(set, fp)
		if len(set) == 0 {
			delete(c.index, eq)
		}
	}
}
