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

package limit

import (
	"container/heap"
	"sync"
	"time"
)

// item represents a value and its priority based on time.
type item[V any] struct {
	value    V
	priority time.Time
	index    int
}

// expired returns true if the item is expired (priority is before the given time).
func (i *item[V]) expired(at time.Time) bool {
	return i.priority.Before(at)
}

// sortedItems is a heap of items.
type sortedItems[V any] []*item[V]

// Len returns the number of items in the heap.
func (s sortedItems[V]) Len() int { return len(s) }

// Less reports whether the element with index i should sort before the element with index j.
func (s sortedItems[V]) Less(i, j int) bool { return s[i].priority.Before(s[j].priority) }

// Swap swaps the elements with indexes i and j.
func (s sortedItems[V]) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
	s[i].index = i
	s[j].index = j
}

// Push adds an item to the heap.
func (s *sortedItems[V]) Push(x any) {
	n := len(*s)
	item := x.(*item[V])
	item.index = n
	*s = append(*s, item)
}

// Pop removes and returns the minimum element (according to Less).
func (s *sortedItems[V]) Pop() any {
	old := *s
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // don't stop the GC from reclaiming the item eventually
	item.index = -1 // for safety
	*s = old[0 : n-1]
	return item
}

// update modifies the priority and value of an item in the heap.
func (s *sortedItems[V]) update(item *item[V], priority time.Time) {
	item.priority = priority
	heap.Fix(s, item.index)
}

// Bucket is a simple cache for values with priority(expiry).
// It has:
// - configurable capacity.
// - a mutex for thread safety.
// - a sorted heap of items for priority/expiry based eviction.
// - an index of items for fast updates.
type Bucket[V comparable] struct {
	mtx      sync.Mutex
	index    map[V]*item[V]
	items    sortedItems[V]
	capacity int
}

// NewBucket creates a new bucket with the given capacity.
// All internal data structures are initialized to the given capacity to avoid allocations during runtime.
func NewBucket[V comparable](capacity int) *Bucket[V] {
	items := make(sortedItems[V], 0, capacity)
	heap.Init(&items)
	return &Bucket[V]{
		index:    make(map[V]*item[V], capacity),
		items:    items,
		capacity: capacity,
	}
}

// IsStale returns true if the latest item in the bucket is expired.
func (b *Bucket[V]) IsStale() (stale bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.items.Len() == 0 {
		return true
	}

	latest := b.items[b.items.Len()-1]
	return latest.expired(time.Now())
}

// Upsert tries to add a new value and its priority to the bucket.
// If the value is already in the bucket, its priority is updated.
// If the bucket is not full, the new value is added.
// If the bucket is full, oldest expired item is evicted based on priority and the new value is added.
// Otherwise the new value is ignored and the method returns false.
func (b *Bucket[V]) Upsert(value V, priority time.Time) (ok bool) {
	if b.capacity < 1 {
		return false
	}

	b.mtx.Lock()
	defer b.mtx.Unlock()

	// If the value is already in the index, update it.
	if item, exists := b.index[value]; exists {
		b.items.update(item, priority)
		return true
	}

	// If the bucket is not full, add the new value to the heap and index.
	if b.items.Len() < b.capacity {
		item := &item[V]{
			value:    value,
			priority: priority,
		}
		b.index[value] = item
		heap.Push(&b.items, item)
		return true
	}

	// If the bucket is full, check the oldest item (at heap root) and evict it if expired
	oldest := b.items[0]
	if oldest.expired(time.Now()) {
		// Remove the expired item from both the heap and the index
		heap.Pop(&b.items)
		delete(b.index, oldest.value)

		// Add the new item
		item := &item[V]{
			value:    value,
			priority: priority,
		}
		b.index[value] = item
		heap.Push(&b.items, item)
		return true
	}

	return false
}
