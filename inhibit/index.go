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
)

// index contains map of fingerprints to sets of fingerprints.
// The keys are fingerprints of the equal labels of source alerts.
// The values are the fingerprints of all source alerts sharing those equal labels.
// For more info see comments on inhibitor and InhibitRule.
type index struct {
	mtx   sync.RWMutex
	items map[model.Fingerprint]model.FingerprintSet
}

func newIndex() *index {
	return &index{
		items: make(map[model.Fingerprint]model.FingerprintSet),
	}
}

// Get returns a copy of the fingerprints indexed under key.
func (c *index) Get(key model.Fingerprint) ([]model.Fingerprint, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	set, ok := c.items[key]
	if !ok {
		return nil, false
	}
	fps := make([]model.Fingerprint, 0, len(set))
	for fp := range set {
		fps = append(fps, fp)
	}
	return fps, true
}

func (c *index) Add(key, value model.Fingerprint) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	set, ok := c.items[key]
	if !ok {
		set = model.FingerprintSet{}
		c.items[key] = set
	}
	set[value] = struct{}{}
}

// Delete removes value from the set of fingerprints indexed under key,
// removing the key entirely once the set is empty.
func (c *index) Delete(key, value model.Fingerprint) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	set, ok := c.items[key]
	if !ok {
		return
	}
	delete(set, value)
	if len(set) == 0 {
		delete(c.items, key)
	}
}

func (c *index) Len() int {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	return len(c.items)
}
