package nflogpb

import (
	"testing"
)

func TestIsFiringSubset(t *testing.T) {
	e := &Entry{
		FiringAlerts: []uint64{1, 2, 3},
	}

	tests := []struct {
		subset   map[uint64]struct{}
		expected bool
	}{
		{newSubset(), true}, //empty subset
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
		{newSubset(), true}, //empty subset
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
	for k, _ := range m {
		els = append(els, k)
	}

	return els
}
