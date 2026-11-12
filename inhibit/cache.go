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
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// constraint is one equality requirement of a rule: the value of label src
// on the source alert must equal the value of label tgt on the target label
// set (missing labels compare as empty). slot is the synthetic name under
// which the constraint's value enters the index key: identity constraints
// (src == tgt) use the label name, pair constraints their source label name
// with a "#2", "#3", ... suffix on collision. Equal-only rules keep the
// historical index keys byte-identical. Suffixes are internal; they never
// surface in alerts or events.
type constraint struct {
	slot model.LabelName
	src  model.LabelName
	tgt  model.LabelName
}

// constraintsFor builds the ordered constraint list: equal labels first,
// then pairs, in config order.
func constraintsFor(equal []model.LabelName, pairs []amcommoncfg.LabelPair) []constraint {
	constraints := make([]constraint, 0, len(equal)+len(pairs))
	usedSlots := make(map[model.LabelName]struct{}, len(equal)+len(pairs))

	for _, name := range equal {
		// A duplicated equal entry re-uses its slot and cannot collide.
		constraints = append(constraints, constraint{slot: name, src: name, tgt: name})
		usedSlots[name] = struct{}{}
	}

	for _, p := range pairs {
		src := string(p.SourceLabel)
		slot := p.SourceLabel
		for n := 2; ; n++ {
			if _, used := usedSlots[slot]; !used {
				break
			}
			slot = model.LabelName(src + "#" + strconv.Itoa(n))
		}
		usedSlots[slot] = struct{}{}
		constraints = append(constraints, constraint{slot: slot, src: p.SourceLabel, tgt: p.TargetLabel})
	}

	return constraints
}

// cache holds the runtime state of an inhibit rule; all mutable state is
// guarded by the single RWMutex (never held across callbacks).
type cache struct {
	// Ordered constraint list; see constraint.
	constraints []constraint

	mtx sync.RWMutex
	// alerts is the map of alerts that match the source matchers of the inhibit rule.
	alerts map[model.Fingerprint]*alert.Alert
	// Index-key fingerprint to the source alert fingerprints stored under
	// it (see fingerprintSource).
	index map[model.Fingerprint]model.FingerprintSet
}

func newCache(constraints []constraint) *cache {
	return &cache{
		constraints: constraints,
		alerts:      make(map[model.Fingerprint]*alert.Alert),
		index:       make(map[model.Fingerprint]model.FingerprintSet),
	}
}

// fingerprintSource projects a source alert's labels onto the index key.
func (c *cache) fingerprintSource(lset model.LabelSet) model.Fingerprint {
	key := make(model.LabelSet, len(c.constraints))
	for _, ct := range c.constraints {
		key[ct.slot] = lset[ct.src]
	}
	return key.Fingerprint()
}

// fingerprintTarget projects a target label set onto the index key for
// probing.
func (c *cache) fingerprintTarget(lset model.LabelSet) model.Fingerprint {
	key := make(model.LabelSet, len(c.constraints))
	for _, ct := range c.constraints {
		key[ct.slot] = lset[ct.tgt]
	}
	return key.Fingerprint()
}

// set adds or replaces the given source alert.
func (c *cache) set(a *alert.Alert) {
	fp := a.Fingerprint()
	key := c.fingerprintSource(a.Labels)

	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.alerts[fp] = a
	set, ok := c.index[key]
	if !ok {
		set = model.FingerprintSet{}
		c.index[key] = set
	}
	set[fp] = struct{}{}
}

// find returns a cached source alert whose projected key matches the target
// label set, is active at now, and satisfies match. Projected keys are equal
// iff every constraint holds.
func (c *cache) find(lset model.LabelSet, now time.Time, match func(*alert.Alert) bool) (model.Fingerprint, bool) {
	key := c.fingerprintTarget(lset)

	c.mtx.RLock()
	defer c.mtx.RUnlock()

	for fp := range c.index[key] {
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

		// Re-derive the key the way set does, or entries get orphaned.
		key := c.fingerprintSource(a.Labels)
		set := c.index[key]
		delete(set, fp)
		if len(set) == 0 {
			delete(c.index, key)
		}
	}
}
