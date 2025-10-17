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

// index contains map of fingerprints to fingerprints.
// The keys are fingerprints of the equal labels of source alerts.
// The values are fingerprints of the source alerts.
// For more info see comments on inhibitor and InhibitRule.
type index struct {
	mtx   sync.RWMutex
	items map[model.Fingerprint]model.Fingerprint
}

func newIndex() *index {
	return &index{
		items: make(map[model.Fingerprint]model.Fingerprint),
	}
}

func (c *index) Get(key model.Fingerprint) (model.Fingerprint, bool) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	fp, ok := c.items[key]
	return fp, ok
}

func (c *index) Set(key, value model.Fingerprint) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.items[key] = value
}

func (c *index) Delete(key model.Fingerprint) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.items, key)
}
