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

package silence

import (
	"sync"

	"github.com/prometheus/common/model"
)

type cacheEntry struct {
	activeIDs  []string
	pendingIDs []string
	version    int
}

func newCacheEntry(activeIDs, pendingIDs []string, version int) *cacheEntry {
	return &cacheEntry{
		activeIDs:  activeIDs,
		pendingIDs: pendingIDs,
		version:    version,
	}
}

func (e *cacheEntry) count() int {
	return len(e.activeIDs) + len(e.pendingIDs)
}

type cache struct {
	entries map[model.Fingerprint]*cacheEntry
	mu      sync.RWMutex
}

func (c *cache) delete(fp model.Fingerprint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, fp)
}

func (c *cache) get(fp model.Fingerprint) cacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry := cacheEntry{}
	if e, found := c.entries[fp]; found {
		entry.version = e.version
		entry.activeIDs = make([]string, len(e.activeIDs))
		copy(entry.activeIDs, e.activeIDs)
		entry.pendingIDs = make([]string, len(e.pendingIDs))
		copy(entry.pendingIDs, e.pendingIDs)
	}
	return entry
}

func (c *cache) set(fp model.Fingerprint, entry *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[fp] = entry
}
