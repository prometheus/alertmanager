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
			name:     "zero for nil slices",
			entry:    newCacheEntry(nil, nil, 0),
			expected: 0,
		},
		{
			name:     "zero for empty slices",
			entry:    newCacheEntry([]string{}, []string{}, 0),
			expected: 0,
		},
		{
			name:     "active only",
			entry:    newCacheEntry([]string{"a", "b"}, nil, 1),
			expected: 2,
		},
		{
			name:     "pending only",
			entry:    newCacheEntry(nil, []string{"a"}, 1),
			expected: 1,
		},
		{
			name:     "active and pending",
			entry:    newCacheEntry([]string{"a"}, []string{"b", "c"}, 1),
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
	active := []string{"s1", "s2"}
	pending := []string{"s3"}
	e := newCacheEntry(active, pending, 42)

	require.Equal(t, active, e.activeIDs)
	require.Equal(t, pending, e.pendingIDs)
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
	c.set(fp, newCacheEntry([]string{"s1"}, []string{"s2"}, 5))
	entry = c.get(fp)
	require.Equal(t, []string{"s1"}, entry.activeIDs)
	require.Equal(t, []string{"s2"}, entry.pendingIDs)
	require.Equal(t, 5, entry.version)
}

func TestCacheGetReturnsCopy(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	c.set(fp, newCacheEntry([]string{"s1"}, nil, 1))

	// Mutating the returned entry should not affect the cache.
	entry := c.get(fp)
	entry.activeIDs = append(entry.activeIDs, "s2")
	entry.version = 99

	original := c.get(fp)
	require.Equal(t, []string{"s1"}, original.activeIDs)
	require.Equal(t, 1, original.version)
}

func TestCacheOverwrite(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	c.set(fp, newCacheEntry([]string{"s1"}, nil, 1))
	c.set(fp, newCacheEntry([]string{"s2", "s3"}, []string{"s4"}, 2))

	entry := c.get(fp)
	require.Equal(t, []string{"s2", "s3"}, entry.activeIDs)
	require.Equal(t, []string{"s4"}, entry.pendingIDs)
	require.Equal(t, 2, entry.version)
}

func TestCacheDelete(t *testing.T) {
	c := newTestCache()
	fp := model.Fingerprint(1)

	c.set(fp, newCacheEntry([]string{"s1"}, nil, 1))
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

	c.set(fp1, newCacheEntry([]string{"s1"}, nil, 1))
	c.set(fp2, newCacheEntry([]string{"s2"}, nil, 2))

	c.delete(fp1)

	// fp1 should be gone.
	entry1 := c.get(fp1)
	require.Equal(t, 0, entry1.count())
	// fp2 should be untouched.
	entry2 := c.get(fp2)
	require.Equal(t, []string{"s2"}, entry2.activeIDs)
}

func TestCacheMultipleFingerprints(t *testing.T) {
	c := newTestCache()

	for i := range 100 {
		fp := model.Fingerprint(i)
		c.set(fp, newCacheEntry([]string{"s"}, nil, i))
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
	c.set(fp, newCacheEntry([]string{"initial"}, nil, 0))

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent readers.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = c.get(fp)
			}
		}()
	}

	// Concurrent writers.
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 100 {
				c.set(fp, newCacheEntry([]string{"w"}, nil, i*100+j))
			}
		}()
	}

	// Concurrent deleters.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				c.delete(fp)
			}
		}()
	}

	wg.Wait()
}
