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

package notify

import (
	"slices"
	"sync"
	"time"
)

type undeliveredEntry struct {
	firstFail       time.Time
	lastSeen        time.Time
	abandoned       bool
	abandonedFiring []uint64 // sorted; set when abandoned
}

// UndeliveredTracker records the first failure time for notification delivery
// retries so delivery can be abandoned after a wall-clock threshold. Entries
// that are inactive longer than gcTTL are removed periodically.
type UndeliveredTracker struct {
	mu      sync.Mutex
	entries map[string]undeliveredEntry
	gcTTL   time.Duration
}

// NewUndeliveredTracker returns a tracker that garbage-collects keys whose
// lastSeen is older than gcTTL. If gcTTL <= 0, a default of 1h is used.
func NewUndeliveredTracker(gcTTL time.Duration) *UndeliveredTracker {
	if gcTTL <= 0 {
		gcTTL = time.Hour
	}
	return &UndeliveredTracker{
		entries: make(map[string]undeliveredEntry),
		gcTTL:   gcTTL,
	}
}

func sortedFiringCopy(firing []uint64) []uint64 {
	out := slices.Clone(firing)
	slices.Sort(out)
	return out
}

func firingSlicesEqual(a, b []uint64) bool {
	return slices.Equal(a, b)
}

func (t *UndeliveredTracker) gc(now time.Time) {
	cutoff := now.Add(-t.gcTTL)
	for k, e := range t.entries {
		if e.lastSeen.Before(cutoff) {
			delete(t.entries, k)
		}
	}
}

// NoteFailure records the first wall-clock time we saw a failed delivery for key,
// or refreshes lastSeen if the series already started.
func (t *UndeliveredTracker) NoteFailure(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gc(now)
	if e, ok := t.entries[key]; ok {
		e.lastSeen = now
		t.entries[key] = e
		return
	}
	t.entries[key] = undeliveredEntry{firstFail: now, lastSeen: now}
}

// Clear removes state for key after successful delivery.
func (t *UndeliveredTracker) Clear(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// ResetIfFiringChanged deletes the key if it was abandoned for a different firing set.
func (t *UndeliveredTracker) ResetIfFiringChanged(key string, firing []uint64, now time.Time) {
	sorted := sortedFiringCopy(firing)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gc(now)
	e, ok := t.entries[key]
	if !ok || !e.abandoned {
		return
	}
	if firingSlicesEqual(sorted, e.abandonedFiring) {
		return
	}
	delete(t.entries, key)
}

// ShouldSuppressAbandoned returns true if the key was abandoned and the current
// firing set matches the set at abandon time. It refreshes lastSeen for GC.
func (t *UndeliveredTracker) ShouldSuppressAbandoned(key string, firing []uint64, now time.Time) bool {
	sorted := sortedFiringCopy(firing)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gc(now)
	e, ok := t.entries[key]
	if !ok || !e.abandoned {
		return false
	}
	if !firingSlicesEqual(sorted, e.abandonedFiring) {
		return false
	}
	e.lastSeen = now
	t.entries[key] = e
	return true
}

// MarkAbandoned records that delivery was abandoned for the given firing set.
func (t *UndeliveredTracker) MarkAbandoned(key string, firing []uint64, now time.Time) {
	sorted := sortedFiringCopy(firing)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gc(now)
	e, ok := t.entries[key]
	if !ok {
		e.firstFail = now
	}
	e.lastSeen = now
	e.abandoned = true
	e.abandonedFiring = sorted
	t.entries[key] = e
}

// ShouldAbandon returns true if key has been failing since at least firstFail+abandonAfter.
// It refreshes lastSeen for GC when the key exists. Always false if already abandoned.
func (t *UndeliveredTracker) ShouldAbandon(key string, abandonAfter time.Duration, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gc(now)
	e, ok := t.entries[key]
	if !ok {
		return false
	}
	if e.abandoned {
		e.lastSeen = now
		t.entries[key] = e
		return false
	}
	e.lastSeen = now
	t.entries[key] = e
	return now.Sub(e.firstFail) >= abandonAfter
}
