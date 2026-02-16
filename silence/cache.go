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

// cacheEntry stores the IDs of silences that match an alert and the version of the silences state the
// result is based on.
type cacheEntry struct {
	silenceIDs []string
	version    int
}

// newCacheEntry creates a new cacheEntry.
func newCacheEntry(version int, silenceIDs ...string) *cacheEntry {
	return &cacheEntry{
		silenceIDs: silenceIDs,
		version:    version,
	}
}

// count returns the number of silence IDs in the cacheEntry.
func (e *cacheEntry) count() int {
	return len(e.silenceIDs)
}

// cache stores the IDs of silences that match an alert and the version of the silences state the
// result is based on.
type cache struct {
	entries map[model.Fingerprint]*cacheEntry
	mtx     sync.RWMutex
}

// delete removes the cacheEntry for the given fingerprint.
func (c *cache) delete(fp model.Fingerprint) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	delete(c.entries, fp)
}

// get returns the cacheEntry for the given fingerprint.
// The returned entry is not a copy, so it should not be modified.
func (c *cache) get(fp model.Fingerprint) *cacheEntry {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	if e, found := c.entries[fp]; found {
		return e
	}
	return &cacheEntry{}
}

// set sets the cacheEntry for the given fingerprint.
func (c *cache) set(fp model.Fingerprint, entry *cacheEntry) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.entries[fp] = entry
}
