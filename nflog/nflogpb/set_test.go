// Copyright 2018 Prometheus Team
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

package nflogpb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsFiringSubset(t *testing.T) {
	e := &Entry{
		FiringAlerts: []uint64{1, 2, 3},
	}

	tests := []struct {
		subset   map[uint64]struct{}
		expected bool
	}{
		{newSubset(), true}, // empty subset
		{newSubset(1), true},
		{newSubset(2), true},
		{newSubset(3), true},
		{newSubset(1, 2), true},
		{newSubset(1, 2), true},
		{newSubset(1, 2, 3), true},
		{newSubset(4), false},
		{newSubset(1, 5), false},
		{newSubset(1, 2, 3, 6), false},
	}

	for _, test := range tests {
		if result := e.IsFiringSubset(test.subset); result != test.expected {
			t.Errorf("Expected %t, got %t for subset %v", test.expected, result, elements(test.subset))
		}
	}
}

func TestIsResolvedSubset(t *testing.T) {
	e := &Entry{
		ResolvedAlerts: []uint64{1, 2, 3},
	}

	tests := []struct {
		subset   map[uint64]struct{}
		expected bool
	}{
		{newSubset(), true}, // empty subset
		{newSubset(1), true},
		{newSubset(2), true},
		{newSubset(3), true},
		{newSubset(1, 2), true},
		{newSubset(1, 2), true},
		{newSubset(1, 2, 3), true},
		{newSubset(4), false},
		{newSubset(1, 5), false},
		{newSubset(1, 2, 3, 6), false},
	}

	for _, test := range tests {
		if result := e.IsResolvedSubset(test.subset); result != test.expected {
			t.Errorf("Expected %t, got %t for subset %v", test.expected, result, elements(test.subset))
		}
	}
}

func newSubset(elements ...uint64) map[uint64]struct{} {
	subset := make(map[uint64]struct{})
	for _, el := range elements {
		subset[el] = struct{}{}
	}

	return subset
}

func elements(m map[uint64]struct{}) []uint64 {
	els := make([]uint64, 0, len(m))
	for k := range m {
		els = append(els, k)
	}

	return els
}

// TestEntryAlertSets covers the distinction the muted alerts feature rests on:
// FiringAlerts is every alert that was firing, and the receiver was shown only
// the ones that were not also muted.
func TestEntryAlertSets(t *testing.T) {
	tests := []struct {
		name             string
		entry            *Entry
		notifiedFiring   map[uint64]struct{}
		notifiedResolved map[uint64]struct{}
		firingSet        map[uint64]struct{}
	}{{
		name:             "empty entry",
		entry:            &Entry{},
		notifiedFiring:   newSubset(),
		notifiedResolved: newSubset(),
		firingSet:        newSubset(),
	}, {
		name:             "nothing muted",
		entry:            &Entry{FiringAlerts: []uint64{1, 2}, ResolvedAlerts: []uint64{3}},
		notifiedFiring:   newSubset(1, 2),
		notifiedResolved: newSubset(3),
		firingSet:        newSubset(1, 2),
	}, {
		// A muted alert is recorded in both lists, so it drops out of what the
		// receiver was shown while still counting as part of the group.
		name:             "one firing alert muted",
		entry:            &Entry{FiringAlerts: []uint64{1, 2}, ResolvedAlerts: []uint64{3}, MutedAlerts: []uint64{2}},
		notifiedFiring:   newSubset(1),
		notifiedResolved: newSubset(3),
		firingSet:        newSubset(1, 2),
	}, {
		name:             "one resolved alert muted",
		entry:            &Entry{FiringAlerts: []uint64{1}, ResolvedAlerts: []uint64{3, 4}, MutedAlerts: []uint64{4}},
		notifiedFiring:   newSubset(1),
		notifiedResolved: newSubset(3),
		firingSet:        newSubset(1),
	}, {
		name:             "every alert muted",
		entry:            &Entry{FiringAlerts: []uint64{1, 2}, MutedAlerts: []uint64{1, 2}},
		notifiedFiring:   newSubset(),
		notifiedResolved: newSubset(),
		firingSet:        newSubset(1, 2),
	}, {
		// Muted hashes that are in neither list change nothing.
		name:             "muted alert not in the group",
		entry:            &Entry{FiringAlerts: []uint64{1}, MutedAlerts: []uint64{9}},
		notifiedFiring:   newSubset(1),
		notifiedResolved: newSubset(),
		firingSet:        newSubset(1),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.notifiedFiring, test.entry.NotifiedFiringAlerts())
			require.Equal(t, test.notifiedResolved, test.entry.NotifiedResolvedAlerts())
			require.Equal(t, test.firingSet, test.entry.FiringAlertSet())
		})
	}
}

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name   string
		set    map[uint64]struct{}
		subset map[uint64]struct{}
		want   bool
	}{
		{name: "both empty", set: newSubset(), subset: newSubset(), want: true},
		{name: "empty subset", set: newSubset(1, 2), subset: newSubset(), want: true},
		{name: "empty set", set: newSubset(), subset: newSubset(1), want: false},
		{name: "proper subset", set: newSubset(1, 2, 3), subset: newSubset(1, 3), want: true},
		{name: "equal", set: newSubset(1, 2), subset: newSubset(1, 2), want: true},
		{name: "one member missing", set: newSubset(1, 2), subset: newSubset(1, 3), want: false},
		{name: "nil set", set: nil, subset: newSubset(1), want: false},
		{name: "nil subset", set: newSubset(1), subset: nil, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsSubset(test.set, test.subset))
		})
	}
}
