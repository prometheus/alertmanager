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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func newTestCache() *cache {
	return &cache{entries: map[model.Fingerprint]*cacheEntry{}}
}

func TestCacheEntryCount(t *testing.T) {
	tests := []struct {
		name     string
		entry    *cacheEntry
		expected int
	}{
		{
			name:     "zero for no silence IDs",
			entry:    newCacheEntry(1),
			expected: 0,
		},
		{
			name:     "one entry",
			entry:    newCacheEntry(2, "a"),
			expected: 1,
		},
		{
			name:     "multiple entries",
			entry:    newCacheEntry(3, "a", "b", "c"),
			expected: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.entry.count())
		})
	}
}

func TestNewCacheEntry(t *testing.T) {
	e := newCacheEntry(42, "s1", "s2")

	require.Equal(t, []string{"s1", "s2"}, e.silenceIDs)
	require.Equal(t, 42, e.version)
}

func TestCacheSetAndGet(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	// Get on empty cache returns zero-value entry.
	entry := c.get(fp)
	require.Equal(t, 0, entry.count())
	require.Equal(t, 0, entry.version)

	// Set and retrieve.
	c.set(fp, newCacheEntry(5, "s1"))
	entry = c.get(fp)
	require.Equal(t, []string{"s1"}, entry.silenceIDs)
	require.Equal(t, 5, entry.version)
}

func TestCacheOverwrite(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	c.set(fp, newCacheEntry(1, "s1"))
	c.set(fp, newCacheEntry(2, "s2", "s3"))

	entry := c.get(fp)
	require.Equal(t, []string{"s2", "s3"}, entry.silenceIDs)
	require.Equal(t, 2, entry.version)
}

func TestCacheDelete(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	c.set(fp, newCacheEntry(1, "s1"))
	before := c.get(fp)
	require.Positive(t, before.count())

	c.delete(fp)

	entry := c.get(fp)
	require.Equal(t, 0, entry.count())
	require.Equal(t, 0, entry.version)
}

func TestCacheDeleteNonExistent(t *testing.T) {
	c := newTestCache()

	// Deleting a key that doesn't exist should not panic.
	require.NotPanics(t, func() {
		c.delete(model.Fingerprint(999))
	})
}

func TestCacheDeleteIsolation(t *testing.T) {
	c := newTestCache()
	fp1 := model.Fingerprint(1)
	fp2 := model.Fingerprint(2)

	c.set(fp1, newCacheEntry(1, "s1"))
	c.set(fp2, newCacheEntry(2, "s2"))

	c.delete(fp1)

	// fp1 should be gone.
	entry1 := c.get(fp1)
	require.Equal(t, 0, entry1.count())
	// fp2 should be untouched.
	entry2 := c.get(fp2)
	require.Equal(t, []string{"s2"}, entry2.silenceIDs)
}

func TestCacheMultipleFingerprints(t *testing.T) {
	c := newTestCache()

	for i := range 100 {
		fp := model.Fingerprint(i)
		c.set(fp, newCacheEntry(i, "s"))
	}

	for i := range 100 {
		fp := model.Fingerprint(i)
		entry := c.get(fp)
		require.Equal(t, 1, entry.count())
		require.Equal(t, i, entry.version)
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)
	c.set(fp, newCacheEntry(0, "initial"))

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent readers.
	for range goroutines {
		wg.Go(func() {
			for range 100 {
				_ = c.get(fp)
			}
		})
	}

	// Concurrent writers.
	for i := range goroutines {
		wg.Go(func() {
			for j := range 100 {
				c.set(fp, newCacheEntry(i*100+j, "w"))
			}
		})
	}

	// Concurrent deleters.
	for range goroutines {
		wg.Go(func() {
			for range 100 {
				c.delete(fp)
			}
		})
	}

	wg.Wait()
}
