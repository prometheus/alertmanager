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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

var nopLogger = promslog.NewNopLogger()

func TestInhibitRuleHasEqual(t *testing.T) {
	t.Parallel()

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
			initial: map[model.Fingerprint]*types.Alert{1: {}},
			input:   model.LabelSet{"a": "b"},
			result:  true,
		},
		{
			// Matching but already resolved.
			initial: map[model.Fingerprint]*types.Alert{
				1: {
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "b": "f"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: {
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
				1: {
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "b", "c": "d"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: {
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
				1: {
					Alert: model.Alert{
						Labels:   model.LabelSet{"a": "c", "c": "d"},
						StartsAt: now.Add(-time.Minute),
						EndsAt:   now.Add(-time.Second),
					},
				},
				2: {
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
			scache: store.NewAlerts(),
		}
		for _, ln := range c.equal {
			r.Equal[ln] = struct{}{}
		}
		for _, v := range c.initial {
			r.scache.Set(v)
		}

		if _, have := r.hasEqual(c.input, false); have != c.result {
			t.Errorf("Unexpected result %t, expected %t", have, c.result)
		}
	}
}

func TestInhibitRuleMatches(t *testing.T) {
	t.Parallel()

	rule1 := config.InhibitRule{
		SourceMatch: map[string]string{"s1": "1"},
		TargetMatch: map[string]string{"t1": "1"},
		Equal:       []string{"e"},
	}
	rule2 := config.InhibitRule{
		SourceMatch: map[string]string{"s2": "1"},
		TargetMatch: map[string]string{"t2": "1"},
		Equal:       []string{"e"},
	}

	m := types.NewMarker(prometheus.NewRegistry())
	ih := NewInhibitor(nil, []config.InhibitRule{rule1, rule2}, m, nopLogger)
	now := time.Now()
	// Active alert that matches the source filter of rule1.
	sourceAlert1 := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s1": "1", "t1": "2", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}
	// Active alert that matches the source filter _and_ the target filter of rule2.
	sourceAlert2 := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}

	ih.rules[0].scache = store.NewAlerts()
	ih.rules[0].scache.Set(sourceAlert1)
	ih.rules[1].scache = store.NewAlerts()
	ih.rules[1].scache.Set(sourceAlert2)

	cases := []struct {
		target   model.LabelSet
		expected bool
	}{
		{
			// Matches target filter of rule1, inhibited.
			target:   model.LabelSet{"t1": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter of rule2, inhibited.
			target:   model.LabelSet{"t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter of rule1 (plus noise), inhibited.
			target:   model.LabelSet{"t1": "1", "t3": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter of rule1 plus rule2, inhibited.
			target:   model.LabelSet{"t1": "1", "t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Doesn't match target filter, not inhibited.
			target:   model.LabelSet{"t1": "0", "e": "1"},
			expected: false,
		},
		{
			// Matches both source and target filters of rule1,
			// inhibited because sourceAlert1 matches only the
			// source filter of rule1.
			target:   model.LabelSet{"s1": "1", "t1": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches both source and target filters of rule2,
			// not inhibited because sourceAlert2 matches also both the
			// source and target filter of rule2.
			target:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			expected: false,
		},
		{
			// Matches target filter, equal label doesn't match, not inhibited
			target:   model.LabelSet{"t1": "1", "e": "0"},
			expected: false,
		},
	}

	for _, c := range cases {
		if actual := ih.Mutes(c.target); actual != c.expected {
			t.Errorf("Expected (*Inhibitor).Mutes(%v) to return %t but got %t", c.target, c.expected, actual)
		}
	}
}

func TestInhibitRuleMatchers(t *testing.T) {
	t.Parallel()

	rule1 := config.InhibitRule{
		SourceMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s1", Value: "1"}},
		TargetMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchNotEqual, Name: "t1", Value: "1"}},
		Equal:          []string{"e"},
	}
	rule2 := config.InhibitRule{
		SourceMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s2", Value: "1"}},
		TargetMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "t2", Value: "1"}},
		Equal:          []string{"e"},
	}

	m := types.NewMarker(prometheus.NewRegistry())
	ih := NewInhibitor(nil, []config.InhibitRule{rule1, rule2}, m, nopLogger)
	now := time.Now()
	// Active alert that matches the source filter of rule1.
	sourceAlert1 := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s1": "1", "t1": "2", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}
	// Active alert that matches the source filter _and_ the target filter of rule2.
	sourceAlert2 := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}

	ih.rules[0].scache = store.NewAlerts()
	ih.rules[0].scache.Set(sourceAlert1)
	ih.rules[1].scache = store.NewAlerts()
	ih.rules[1].scache.Set(sourceAlert2)

	cases := []struct {
		target   model.LabelSet
		expected bool
	}{
		{
			// Matches target filter of rule1, inhibited.
			target:   model.LabelSet{"t1": "1", "e": "1"},
			expected: false,
		},
		{
			// Matches target filter of rule2, inhibited.
			target:   model.LabelSet{"t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter of rule1 (plus noise), inhibited.
			target:   model.LabelSet{"t1": "1", "t3": "1", "e": "1"},
			expected: false,
		},
		{
			// Matches target filter of rule1 plus rule2, inhibited.
			target:   model.LabelSet{"t1": "1", "t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Doesn't match target filter, not inhibited.
			target:   model.LabelSet{"t1": "0", "e": "1"},
			expected: true,
		},
		{
			// Matches both source and target filters of rule1,
			// inhibited because sourceAlert1 matches only the
			// source filter of rule1.
			target:   model.LabelSet{"s1": "1", "t1": "1", "e": "1"},
			expected: false,
		},
		{
			// Matches both source and target filters of rule2,
			// not inhibited because sourceAlert2 matches also both the
			// source and target filter of rule2.
			target:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Matches target filter, equal label doesn't match, not inhibited
			target:   model.LabelSet{"t1": "1", "e": "0"},
			expected: false,
		},
	}

	for _, c := range cases {
		if actual := ih.Mutes(c.target); actual != c.expected {
			t.Errorf("Expected (*Inhibitor).Mutes(%v) to return %t but got %t", c.target, c.expected, actual)
		}
	}
}

type fakeAlerts struct {
	alerts   []*types.Alert
	finished chan struct{}
}

func newFakeAlerts(alerts []*types.Alert) *fakeAlerts {
	return &fakeAlerts{
		alerts:   alerts,
		finished: make(chan struct{}),
	}
}

func (f *fakeAlerts) GetPending() provider.AlertIterator          { return nil }
func (f *fakeAlerts) Get(model.Fingerprint) (*types.Alert, error) { return nil, nil }
func (f *fakeAlerts) Put(...*types.Alert) error                   { return nil }
func (f *fakeAlerts) Subscribe() provider.AlertIterator {
	ch := make(chan *types.Alert)
	done := make(chan struct{})
	go func() {
		for _, a := range f.alerts {
			ch <- a
		}
		// Send another (meaningless) alert to make sure that the inhibitor has
		// processed everything.
		ch <- &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{},
				StartsAt: time.Now(),
			},
		}
		close(f.finished)
		<-done
	}()
	return provider.NewAlertIterator(ch, done, nil)
}

func TestInhibit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	inhibitRule := func() config.InhibitRule {
		return config.InhibitRule{
			SourceMatch: map[string]string{"s": "1"},
			TargetMatch: map[string]string{"t": "1"},
			Equal:       []string{"e"},
		}
	}
	// alertOne is muted by alertTwo when it is active.
	alertOne := func() *types.Alert {
		return &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"t": "1", "e": "f"},
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
		}
	}
	alertTwo := func(resolved bool) *types.Alert {
		var end time.Time
		if resolved {
			end = now.Add(-time.Second)
		} else {
			end = now.Add(time.Hour)
		}
		return &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"s": "1", "e": "f"},
				StartsAt: now.Add(-time.Minute),
				EndsAt:   end,
			},
		}
	}

	type exp struct {
		lbls  model.LabelSet
		muted bool
	}
	for i, tc := range []struct {
		alerts   []*types.Alert
		expected []exp
	}{
		{
			// alertOne shouldn't be muted since alertTwo hasn't fired.
			alerts: []*types.Alert{alertOne()},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "f"},
					muted: false,
				},
			},
		},
		{
			// alertOne should be muted by alertTwo which is active.
			alerts: []*types.Alert{alertOne(), alertTwo(false)},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "f"},
					muted: true,
				},
				{
					lbls:  model.LabelSet{"s": "1", "e": "f"},
					muted: false,
				},
			},
		},
		{
			// alertOne shouldn't be muted since alertTwo is resolved.
			alerts: []*types.Alert{alertOne(), alertTwo(false), alertTwo(true)},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "f"},
					muted: false,
				},
				{
					lbls:  model.LabelSet{"s": "1", "e": "f"},
					muted: false,
				},
			},
		},
	} {
		ap := newFakeAlerts(tc.alerts)
		mk := types.NewMarker(prometheus.NewRegistry())
		inhibitor := NewInhibitor(ap, []config.InhibitRule{inhibitRule()}, mk, nopLogger)

		go func() {
			for ap.finished != nil {
				select {
				case <-ap.finished:
					ap.finished = nil
				default:
				}
			}
			inhibitor.Stop()
		}()
		inhibitor.Run()

		for _, expected := range tc.expected {
			if inhibitor.Mutes(expected.lbls) != expected.muted {
				mute := "unmuted"
				if expected.muted {
					mute = "muted"
				}
				t.Errorf("tc: %d, expected alert with labels %q to be %s", i, expected.lbls, mute)
			}
		}
	}
}
