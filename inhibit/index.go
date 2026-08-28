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
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

// index maps fingerprints of source alert equal labels to source alerts.
// For more info see comments on inhibitor and InhibitRule.
type index struct {
	mtx   sync.RWMutex
	items map[model.Fingerprint]*indexEntry
}

type indexEntry struct {
	alerts map[model.Fingerprint]indexedAlert

	any        *types.Alert
	sourceOnly *types.Alert
}

type indexedAlert struct {
	alert      *types.Alert
	sourceOnly bool
}

func newIndex() *index {
	return &index{
		items: make(map[model.Fingerprint]*indexEntry),
	}
}

func (c *index) Get(key model.Fingerprint, sourceOnly bool) (*types.Alert, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	entry, ok := c.items[key]
	if !ok {
		return nil, false
	}

	if sourceOnly {
		return entry.sourceOnly, entry.sourceOnly != nil
	}

	return entry.any, entry.any != nil
}

func (c *index) Set(key model.Fingerprint, alert *types.Alert, sourceOnly bool) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	entry, ok := c.items[key]
	if !ok {
		entry = &indexEntry{
			alerts: make(map[model.Fingerprint]indexedAlert),
		}
		c.items[key] = entry
	}

	if sameFingerprint(entry.any, alert) || sameFingerprint(entry.sourceOnly, alert) {
		entry.alerts[alert.Fingerprint()] = indexedAlert{
			alert:      alert,
			sourceOnly: sourceOnly,
		}
		entry.rebuild()
		return
	}

	entry.alerts[alert.Fingerprint()] = indexedAlert{
		alert:      alert,
		sourceOnly: sourceOnly,
	}

	if shouldReplaceIndexAlert(entry.any, alert) {
		entry.any = alert
	}
	if sourceOnly && shouldReplaceIndexAlert(entry.sourceOnly, alert) {
		entry.sourceOnly = alert
	}
}

func (c *index) Delete(key model.Fingerprint, alert *types.Alert) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	entry, ok := c.items[key]
	if !ok {
		return
	}

	fp := alert.Fingerprint()
	indexed, ok := entry.alerts[fp]
	if !ok {
		return
	}
	if indexed.alert != alert {
		return
	}

	delete(entry.alerts, fp)
	if len(entry.alerts) == 0 {
		delete(c.items, key)
		return
	}

	if entry.any == alert || entry.sourceOnly == alert {
		entry.rebuild()
	}
}

func (c *index) Len() int {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	return len(c.items)
}

func (e *indexEntry) rebuild() {
	e.any = nil
	e.sourceOnly = nil

	for _, indexed := range e.alerts {
		if shouldReplaceIndexAlert(e.any, indexed.alert) {
			e.any = indexed.alert
		}
		if indexed.sourceOnly && shouldReplaceIndexAlert(e.sourceOnly, indexed.alert) {
			e.sourceOnly = indexed.alert
		}
	}
}

func shouldReplaceIndexAlert(current, candidate *types.Alert) bool {
	if current == nil {
		return true
	}
	return current.ResolvedAt(candidate.EndsAt)
}

func sameFingerprint(a, b *types.Alert) bool {
	return a != nil && a.Fingerprint() == b.Fingerprint()
}
