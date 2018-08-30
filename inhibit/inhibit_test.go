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

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

var nopLogger = log.NewNopLogger()

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
			scache: store.NewAlerts(5 * time.Minute),
		}
		for _, ln := range c.equal {
			r.Equal[ln] = struct{}{}
		}
		for _, v := range c.initial {
			r.scache.Set(v)
		}

		if _, have := r.hasEqual(c.input); have != c.result {
			t.Errorf("Unexpected result %t, expected %t", have, c.result)
		}
	}
}

func TestInhibitRuleMatches(t *testing.T) {
	t.Parallel()

	// Simple inhibut rule
	cr := config.InhibitRule{
		SourceMatch: map[string]string{"s": "1"},
		TargetMatch: map[string]string{"t": "1"},
		Equal:       model.LabelNames{"e"},
	}
	m := types.NewMarker()
	ih := NewInhibitor(nil, []*config.InhibitRule{&cr}, m, nopLogger)
	ir := ih.rules[0]
	now := time.Now()
	// Active alert that matches the source filter
	sourceAlert := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s": "1", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}

	ir.scache = store.NewAlerts(5 * time.Minute)
	ir.scache.Set(sourceAlert)

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
	inhibitRule := func() *config.InhibitRule {
		return &config.InhibitRule{
			SourceMatch: map[string]string{"s": "1"},
			TargetMatch: map[string]string{"t": "1"},
			Equal:       model.LabelNames{"e"},
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
		mk := types.NewMarker()
		inhibitor := NewInhibitor(ap, []*config.InhibitRule{inhibitRule()}, mk, nopLogger)

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
