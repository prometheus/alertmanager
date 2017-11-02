// Copyright 2016 Prometheus Team
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
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

func TestInhibitRuleHasEqual(t *testing.T) {
	now := time.Now()
	cases := []struct {
		initial map[model.Fingerprint]*types.Alert
		equal   model.LabelNames
		input   model.LabelSet
		result  bool
	}{
		{
			// No source alerts at all.
			initial: map[model.Fingerprint]*types.Alert{},
			input:   model.LabelSet{"a": "b"},
			result:  false,
		},
		{
			// No equal labels, any source alerts satisfies the requirement.
			initial: map[model.Fingerprint]*types.Alert{1: &types.Alert{}},
			input:   model.LabelSet{"a": "b"},
			result:  true,
		},
		{
			// Matching but already resolved.
			initial: map[model.Fingerprint]*types.Alert{
				1: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "b": "f"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "b": "c"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
			},
			equal:  model.LabelNames{"a", "b"},
			input:  model.LabelSet{"a": "b", "b": "c"},
			result: false,
		},
		{
			// Matching and unresolved.
			initial: map[model.Fingerprint]*types.Alert{
				1: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "c": "d"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "c": "f"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(time.Hour),
					},
				},
			},
			equal:  model.LabelNames{"a"},
			input:  model.LabelSet{"a": "b"},
			result: true,
		},
		{
			// Equal label does not match.
			initial: map[model.Fingerprint]*types.Alert{
				1: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "c", "c": "d"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "c", "c": "f"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
			},
			equal:  model.LabelNames{"a"},
			input:  model.LabelSet{"a": "b"},
			result: false,
		},
	}

	for _, c := range cases {
		r := &InhibitRule{
			Equal:  map[model.LabelName]struct{}{},
			scache: map[model.Fingerprint]*types.Alert{},
		}
		for _, ln := range c.equal {
			r.Equal[ln] = struct{}{}
		}
		for k, v := range c.initial {
			r.scache[k] = v
		}

		if _, have := r.hasEqual(c.input); have != c.result {
			t.Errorf("Unexpected result %t, expected %t", have, c.result)
		}
		if !reflect.DeepEqual(r.scache, c.initial) {
			t.Errorf("Cache state unexpectedly changed")
			t.Errorf(pretty.Compare(r.scache, c.initial))
		}
	}
}

func TestInhibitRuleMatches(t *testing.T) {
	// Simple inhibut rule
	cr := config.InhibitRule{
		SourceMatch: map[string]string{"s": "1"},
		TargetMatch: map[string]string{"t": "1"},
		Equal:       model.LabelNames{"e"},
	}
	m := types.NewMarker()
	ih := NewInhibitor(nil, []*config.InhibitRule{&cr}, m, nil)
	ir := ih.rules[0]
	now := time.Now()
	// Active alert that matches the source filter
	sourceAlert := types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s": "1", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}
	ir.scache = map[model.Fingerprint]*types.Alert{1: &sourceAlert}

	cases := []struct {
		target   model.LabelSet
		expected bool
	}{
		{
			// Matches target filter, inhibited
			target:   model.LabelSet{"t": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter (plus noise), inhibited
			target:   model.LabelSet{"t": "1", "t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Doesn't match target filter, not inhibited
			target:   model.LabelSet{"t": "0", "e": "1"},
			expected: false,
		},
		{
			// Matches both source and target filters, not inhibited
			target:   model.LabelSet{"s": "1", "t": "1", "e": "1"},
			expected: false,
		},
		{
			// Matches target filter, equal label doesn't match, not inhibited
			target:   model.LabelSet{"t": "1", "e": "0"},
			expected: false,
		},
	}

	for _, c := range cases {
		if actual := ih.Mutes(c.target); actual != c.expected {
			t.Errorf("Expected (*Inhibitor).Mutes(%v) to return %t but got %t", c.target, c.expected, actual)
		}
	}
}

func TestInhibitRuleGC(t *testing.T) {
	// TODO(fabxc): add now() injection function to Resolved() to remove
	// dependency on machine time in this test.
	now := time.Now()
	newAlert := func(start, end time.Duration) *types.Alert {
		return &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"a": "b"},
				StartsAt: now.Add(start * time.Minute),
				EndsAt:   now.Add(end * time.Minute),
			},
		}
	}

	before := map[model.Fingerprint]*types.Alert{
		0: newAlert(-10, -5),
		1: newAlert(10, 20),
		2: newAlert(-10, 10),
		3: newAlert(-10, -1),
	}
	after := map[model.Fingerprint]*types.Alert{
		1: newAlert(10, 20),
		2: newAlert(-10, 10),
	}

	r := &InhibitRule{scache: before}
	r.gc()

	if !reflect.DeepEqual(r.scache, after) {
		t.Errorf("Unexpected cache state after GC")
		t.Errorf(pretty.Compare(r.scache, after))
	}
}
