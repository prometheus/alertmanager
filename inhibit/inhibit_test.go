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

package inhibit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
)

var nopLogger = promslog.NewNopLogger()

// checkMutes calls ih.Mutes with a fresh AlertMarker in the context,
// asserts the mute result matches wantMuted, and verifies the marker status.
func checkMutes(t *testing.T, ih *Inhibitor, target model.LabelSet, wantMuted bool, msgAndArgs ...any) {
	t.Helper()
	m := marker.NewAlertMarker()
	ctx := marker.WithContext(context.Background(), m)
	got := ih.Mutes(ctx, labelset.FromModel(target))
	require.Equal(t, wantMuted, got, msgAndArgs...)
	fp := target.Fingerprint()
	status := m.Status(fp)
	if wantMuted {
		require.Equal(t, alert.AlertStateSuppressed, status.State, msgAndArgs...)
		require.NotEmpty(t, status.InhibitedBy, msgAndArgs...)
	} else {
		require.Equal(t, alert.AlertStateActive, status.State, msgAndArgs...)
	}
}

// runInhibitor returns an inhibitor that has processed alerts and stopped, so
// each rule's source cache and index hold what processAlert put there.
func runInhibitor(t *testing.T, rules []amcommoncfg.InhibitRule, alerts ...*alert.Alert) *Inhibitor {
	t.Helper()

	ap := newFakeAlerts(alerts)
	ih := NewInhibitor(ap, rules, nopLogger, eventrecorder.NopRecorder())
	go func() {
		<-ap.finished
		ih.Stop()
	}()
	ih.Run()

	return ih
}

func TestInhibitRuleHasEqual(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cases := []struct {
		name                 string
		initial              map[model.Fingerprint]*alert.Alert
		equal                model.LabelNames
		targetMatchers       labels.Matchers
		input                model.LabelSet
		excludeTwoSidedMatch bool
		result               bool
	}{
		{
			name:    "no source alerts",
			initial: map[model.Fingerprint]*alert.Alert{},
			input:   model.LabelSet{"a": "b"},
			result:  false,
		},
		{
			name:    "no equal labels, any source alerts satisfies the requirement",
			initial: map[model.Fingerprint]*alert.Alert{1: alert.New(model.Alert{}, time.Time{}, false)},
			input:   model.LabelSet{"a": "b"},
			result:  true,
		},
		{
			name: "matching but already resolved",
			initial: map[model.Fingerprint]*alert.Alert{
				1: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "b", "b": "f"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(-time.Second),
				}, time.Time{}, false),
				2: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "b", "b": "c"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(-time.Second),
				}, time.Time{}, false),
			},
			equal:  model.LabelNames{"a", "b"},
			input:  model.LabelSet{"a": "b", "b": "c"},
			result: false,
		},
		{
			name: "matching and unresolved",
			initial: map[model.Fingerprint]*alert.Alert{
				1: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "b", "c": "d"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(-time.Second),
				}, time.Time{}, false),
				2: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "b", "c": "f"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(time.Hour),
				}, time.Time{}, false),
			},
			equal:  model.LabelNames{"a"},
			input:  model.LabelSet{"a": "b"},
			result: true,
		},
		{
			name: "equal label does not match",
			initial: map[model.Fingerprint]*alert.Alert{
				1: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "c", "c": "d"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(-time.Second),
				}, time.Time{}, false),
				2: alert.New(model.Alert{
					Labels:   model.LabelSet{"a": "c", "c": "f"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(-time.Second),
				}, time.Time{}, false),
			},
			equal:  model.LabelNames{"a"},
			input:  model.LabelSet{"a": "b"},
			result: false,
		},
		{
			name: "matching source-only alert still inhibits when newest equal source is two-sided",
			initial: map[model.Fingerprint]*alert.Alert{
				1: alert.New(model.Alert{
					Labels:   model.LabelSet{"s": "1", "e": "1"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(time.Hour),
				}, time.Time{}, false),
				2: alert.New(model.Alert{
					Labels:   model.LabelSet{"s": "1", "t": "1", "e": "1"},
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(2 * time.Hour),
				}, time.Time{}, false),
			},
			equal:          model.LabelNames{"e"},
			targetMatchers: labels.Matchers{{Type: labels.MatchEqual, Name: "t", Value: "1"}},
			input:          model.LabelSet{"s": "1", "t": "1", "e": "1"},
			// The indexed two-sided source must be ignored, but the source-only
			// alert with the same equal labels should still inhibit the target.
			excludeTwoSidedMatch: true,
			result:               true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			equal := map[model.LabelName]struct{}{}
			for _, ln := range c.equal {
				equal[ln] = struct{}{}
			}
			r := &InhibitRule{
				TargetMatchers: c.targetMatchers,
				Equal:          equal,
				// Equal-only data: identity constraints with the raw names as slots.
				cache: newCache(constraintsFor(c.equal, nil)),
			}
			for _, v := range c.initial {
				r.cache.set(v)
			}

			if _, have := r.hasEqual(c.input, c.excludeTwoSidedMatch, time.Now()); have != c.result {
				t.Errorf("Unexpected result %t, expected %t", have, c.result)
			}
		})
	}
}

func TestInhibitRuleHasEqualKeepsSourceOnlyAlertAfterGCSameEqual(t *testing.T) {
	t.Parallel()

	now := time.Now()
	sourceOnly := alert.New(model.Alert{
		Labels:   model.LabelSet{"s": "1", "e": "1", "id": "source-only"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	expiredSameEqual := alert.New(model.Alert{
		Labels:   model.LabelSet{"s": "1", "e": "1", "id": "expired"},
		StartsAt: now.Add(-2 * time.Hour),
		EndsAt:   now.Add(-time.Hour),
	}, time.Time{}, false)

	ih := runInhibitor(t, []amcommoncfg.InhibitRule{{
		TargetMatch: map[string]string{"t": "1"},
		Equal:       []string{"e"},
	}}, sourceOnly, expiredSameEqual)
	r := ih.rules[0]

	target := model.LabelSet{"s": "1", "t": "1", "e": "1"}
	_, found := r.hasEqual(target, true, now)
	require.True(t, found)

	r.cache.gc()

	_, found = r.hasEqual(target, true, now)
	require.True(t, found)
}

func TestInhibitRuleGCCallbackDoesNotRemoveRefreshedSameFingerprintSourceAlert(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oldSource := alert.New(model.Alert{
		Labels:   model.LabelSet{"s": "1", "e": "1"},
		StartsAt: now.Add(-2 * time.Hour),
		EndsAt:   now.Add(-time.Hour),
	}, now.Add(-time.Hour), false)
	refreshedSource := alert.New(model.Alert{
		Labels:   model.LabelSet{"s": "1", "e": "1"},
		StartsAt: now.Add(-2 * time.Hour),
		EndsAt:   now.Add(time.Hour),
	}, now, false)

	ih := runInhibitor(t, []amcommoncfg.InhibitRule{{Equal: []string{"e"}}}, oldSource, refreshedSource)
	r := ih.rules[0]

	r.cache.gc()

	_, found := r.hasEqual(model.LabelSet{"t": "1", "e": "1"}, false, now)
	require.True(t, found)
}

func TestInhibitRuleMatches(t *testing.T) {
	t.Parallel()

	rule1 := amcommoncfg.InhibitRule{
		SourceMatch: map[string]string{"s1": "1"},
		TargetMatch: map[string]string{"t1": "1"},
		Equal:       []string{"e"},
	}
	rule2 := amcommoncfg.InhibitRule{
		SourceMatch: map[string]string{"s2": "1"},
		TargetMatch: map[string]string{"t2": "1"},
		Equal:       []string{"e"},
	}

	ih := NewInhibitor(nil, []amcommoncfg.InhibitRule{rule1, rule2}, nopLogger, eventrecorder.NopRecorder())
	now := time.Now()
	// Active alert that matches the source filter of rule1.
	sourceAlert1 := alert.New(model.Alert{
		Labels:   model.LabelSet{"s1": "1", "t1": "2", "e": "1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	// Active alert that matches the source filter _and_ the target filter of rule2.
	sourceAlert2 := alert.New(model.Alert{
		Labels:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)

	ih.rules[0].cache.set(sourceAlert1)
	ih.rules[1].cache.set(sourceAlert2)

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
		checkMutes(t, ih, c.target, c.expected, "target %v", c.target)
	}
}

func TestInhibitRuleMatchers(t *testing.T) {
	t.Parallel()

	rule1 := amcommoncfg.InhibitRule{
		SourceMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s1", Value: "1"}},
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchNotEqual, Name: "t1", Value: "1"}},
		Equal:          []string{"e"},
	}
	rule2 := amcommoncfg.InhibitRule{
		SourceMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s2", Value: "1"}},
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "t2", Value: "1"}},
		Equal:          []string{"e"},
	}

	ih := NewInhibitor(nil, []amcommoncfg.InhibitRule{rule1, rule2}, nopLogger, eventrecorder.NopRecorder())
	now := time.Now()
	// Active alert that matches the source filter of rule1.
	sourceAlert1 := alert.New(model.Alert{
		Labels:   model.LabelSet{"s1": "1", "t1": "2", "e": "1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	// Active alert that matches the source filter _and_ the target filter of rule2.
	sourceAlert2 := alert.New(model.Alert{
		Labels:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)

	ih.rules[0].cache.set(sourceAlert1)
	ih.rules[1].cache.set(sourceAlert2)

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
		checkMutes(t, ih, c.target, c.expected, "target %v", c.target)
	}
}

func TestInhibitRuleName(t *testing.T) {
	t.Parallel()

	config1 := amcommoncfg.InhibitRule{
		Name: "test-rule",
		SourceMatchers: []*labels.Matcher{
			{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
		},
		TargetMatchers: []*labels.Matcher{
			{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
		},
		Equal: []string{"instance"},
	}
	config2 := amcommoncfg.InhibitRule{
		SourceMatchers: []*labels.Matcher{
			{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
		},
		TargetMatchers: []*labels.Matcher{
			{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
		},
		Equal: []string{"instance"},
	}

	rule1 := NewInhibitRule(config1)
	rule2 := NewInhibitRule(config2)

	require.Equal(t, "test-rule", rule1.Name, "Expected named rule to have adopt name from config")
	require.Empty(t, rule2.Name, "Expected unnamed rule to have empty name")
}

type fakeAlerts struct {
	alerts   []*alert.Alert
	finished chan struct{}
}

func newFakeAlerts(alerts []*alert.Alert) *fakeAlerts {
	return &fakeAlerts{
		alerts:   alerts,
		finished: make(chan struct{}),
	}
}

func (f *fakeAlerts) GetPending() provider.AlertIterator          { return nil }
func (f *fakeAlerts) Get(model.Fingerprint) (*alert.Alert, error) { return nil, nil }
func (f *fakeAlerts) Put(context.Context, ...*alert.Alert) error  { return nil }
func (f *fakeAlerts) Subscribe(name string) provider.AlertIterator {
	ch := make(chan *provider.Alert)
	done := make(chan struct{})
	go func() {
		for _, a := range f.alerts {
			ch <- &provider.Alert{
				Data:   a,
				Header: map[string]string{},
			}
		}
		// Send another (meaningless) alert to make sure that the inhibitor has
		// processed everything.
		ch <- &provider.Alert{
			Data: alert.New(model.Alert{
				Labels:   model.LabelSet{},
				StartsAt: time.Now(),
			}, time.Time{}, false),
			Header: map[string]string{},
		}
		close(f.finished)
		<-done
	}()
	return provider.NewAlertIterator(ch, done, nil)
}

func (f *fakeAlerts) SlurpAndSubscribe(name string) ([]*alert.Alert, provider.AlertIterator) {
	ch := make(chan *provider.Alert)
	done := make(chan struct{})
	go func() {
		for _, a := range f.alerts {
			ch <- &provider.Alert{
				Data:   a,
				Header: map[string]string{},
			}
		}
		// Send another (meaningless) alert to make sure that the inhibitor has
		// processed everything.
		ch <- &provider.Alert{
			Data: alert.New(model.Alert{
				Labels:   model.LabelSet{},
				StartsAt: time.Now(),
			}, time.Time{}, false),
			Header: map[string]string{},
		}
		close(f.finished)
		<-done
	}()
	return []*alert.Alert{}, provider.NewAlertIterator(ch, done, nil)
}

func TestInhibit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	inhibitRule := func() amcommoncfg.InhibitRule {
		return amcommoncfg.InhibitRule{
			SourceMatch: map[string]string{"s": "1"},
			TargetMatch: map[string]string{"t": "1"},
			Equal:       []string{"e"},
		}
	}
	// alertOne is muted by alertTwo when it is active.
	alertOne := func() *alert.Alert {
		return alert.New(model.Alert{
			Labels:   model.LabelSet{"t": "1", "e": "f"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		}, time.Time{}, false)
	}
	alertTwo := func(resolved bool) *alert.Alert {
		var end time.Time
		if resolved {
			end = now.Add(-time.Second)
		} else {
			end = now.Add(time.Hour)
		}
		return alert.New(model.Alert{
			Labels:   model.LabelSet{"s": "1", "e": "f"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   end,
		}, time.Time{}, false)
	}

	type exp struct {
		lbls  model.LabelSet
		muted bool
	}
	for i, tc := range []struct {
		alerts   []*alert.Alert
		expected []exp
	}{
		{
			// alertOne shouldn't be muted since alertTwo hasn't fired.
			alerts: []*alert.Alert{alertOne()},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "f"},
					muted: false,
				},
			},
		},
		{
			// alertOne should be muted by alertTwo which is active.
			alerts: []*alert.Alert{alertOne(), alertTwo(false)},
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
			alerts: []*alert.Alert{alertOne(), alertTwo(false), alertTwo(true)},
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
		inhibitor := NewInhibitor(ap, []amcommoncfg.InhibitRule{inhibitRule()}, nopLogger, eventrecorder.NopRecorder())

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
			checkMutes(t, inhibitor, expected.lbls, expected.muted, "tc: %d, labels %q", i, expected.lbls)
		}
	}
}

func TestInhibitRule_fingerprintEquals(t *testing.T) {
	c := newCache(constraintsFor(
		model.LabelNames{"cluster", "service"}, nil,
	))

	lset := model.LabelSet{
		"cluster":  "prod",
		"service":  "api",
		"instance": "host1",
	}

	fp := c.fingerprintSource(lset)

	// Same equal labels should produce same fingerprint
	lset2 := model.LabelSet{
		"cluster":  "prod",
		"service":  "api",
		"instance": "host2", // different non-equal label
	}
	require.Equal(t, fp, c.fingerprintSource(lset2))

	// Different equal label value should produce different fingerprint
	lset3 := model.LabelSet{
		"cluster": "staging",
		"service": "api",
	}
	require.NotEqual(t, fp, c.fingerprintSource(lset3))
}

func TestInhibitRuleIndexSurvivesGC(t *testing.T) {
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{Equal: []string{"cluster"}})

	active := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S1", "cluster": "c1"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(2 * time.Hour),
	}, time.Time{}, false)
	resolved := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S2", "cluster": "c1"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(-time.Minute),
	}, time.Time{}, false)
	r.cache.set(active)
	r.cache.set(resolved)

	target := model.LabelSet{"alertname": "T", "cluster": "c1"}
	fp, ok := r.hasEqual(target, false, now)
	require.True(t, ok)
	require.Equal(t, active.Fingerprint(), fp)

	r.cache.gc()
	require.Len(t, r.cache.alerts, 1)
	require.Contains(t, r.cache.alerts, active.Fingerprint())

	fp, ok = r.hasEqual(target, false, now)
	require.True(t, ok, "active source alert must still inhibit after GC of a sibling")
	require.Equal(t, active.Fingerprint(), fp)
	require.Len(t, r.cache.index, 1)

	active.EndsAt = now.Add(-time.Second)
	r.cache.set(active)
	r.cache.gc()
	_, ok = r.hasEqual(target, false, now)
	require.False(t, ok)
	require.Empty(t, r.cache.alerts)
	require.Empty(t, r.cache.index, "empty index keys must be removed")
}

func TestInhibitRuleTwoSidedDoesNotShadow(t *testing.T) {
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "warning"}},
		Equal:          []string{"cluster"},
	})

	sourceOnly := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S1", "cluster": "c1", "severity": "critical"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	twoSided := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S2", "cluster": "c1", "severity": "warning"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(2 * time.Hour),
	}, time.Time{}, false)
	r.cache.set(sourceOnly)
	r.cache.set(twoSided)

	target := model.LabelSet{"alertname": "T", "cluster": "c1", "severity": "warning"}
	fp, ok := r.hasEqual(target, true, now)
	require.True(t, ok)
	require.Equal(t, sourceOnly.Fingerprint(), fp)
}

// TestConstraintSlots covers slot assignment: raw names for equal labels,
// "#N" suffixes on collision, config order preserved.
func TestConstraintSlots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		equal model.LabelNames
		pairs []amcommoncfg.LabelPair
		want  []constraint
	}{
		{
			name: "no constraints",
			want: []constraint{},
		},
		{
			name: "equal labels keep their names as slots in config order",
			equal: model.LabelNames{
				"alertname",
				"cluster",
			},
			want: []constraint{
				{slot: "alertname", src: "alertname", tgt: "alertname"},
				{slot: "cluster", src: "cluster", tgt: "cluster"},
			},
		},
		{
			name: "pair whose source label is not equal to any other constraint",
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			want: []constraint{
				{slot: "hostname", src: "hostname", tgt: "remote"},
			},
		},
		{
			name:  "pair source label also listed in equal gets a suffixed slot",
			equal: model.LabelNames{"hostname"},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			want: []constraint{
				{slot: "hostname", src: "hostname", tgt: "hostname"},
				{slot: "hostname#2", src: "hostname", tgt: "remote"},
			},
		},
		{
			name: "two pairs sharing one source label count suffixes independently",
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "source", TargetLabel: "target_a"},
				{SourceLabel: "source", TargetLabel: "target_b"},
			},
			want: []constraint{
				{slot: "source", src: "source", tgt: "target_a"},
				{slot: "source#2", src: "source", tgt: "target_b"},
			},
		},
		{
			name:  "equal label plus two pairs on the same source label",
			equal: model.LabelNames{"source"},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "source", TargetLabel: "target_a"},
				{SourceLabel: "source", TargetLabel: "target_b"},
			},
			want: []constraint{
				{slot: "source", src: "source", tgt: "source"},
				{slot: "source#2", src: "source", tgt: "target_a"},
				{slot: "source#3", src: "source", tgt: "target_b"},
			},
		},
		{
			name:  "duplicate equal entries share one slot and stay deterministic",
			equal: model.LabelNames{"hostname", "hostname"},
			want: []constraint{
				{slot: "hostname", src: "hostname", tgt: "hostname"},
				{slot: "hostname", src: "hostname", tgt: "hostname"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := constraintsFor(c.equal, c.pairs)
			require.Equal(t, c.want, got)
			// Slot assignment must be deterministic for repeated calls.
			require.Equal(t, got, constraintsFor(c.equal, c.pairs))
		})
	}
}

func TestNewInhibitRuleEqualAndPairShareSourceLabel(t *testing.T) {
	t.Parallel()

	// Slot collision: hostname is both an equal label and a pair source
	// label, so the pair gets the suffixed slot.
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		Equal:      []string{"hostname"},
		EqualPairs: []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
	})

	require.Equal(t, map[model.LabelName]struct{}{"hostname": {}}, r.Equal)
	require.Equal(t, []constraint{
		{slot: "hostname", src: "hostname", tgt: "hostname"},
		{slot: "hostname#2", src: "hostname", tgt: "remote"},
	}, r.cache.constraints)
}

func TestInhibitRulePairsTwoSidedDoesNotShadow(t *testing.T) {
	t.Parallel()

	// Pair variant of TestInhibitRuleTwoSidedDoesNotShadow: two-sided sources
	// are skipped and never shadow a source-only alert.
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		TargetMatchers: amcommoncfg.Matchers{
			&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
		},
		Equal:      []string{"cluster"},
		EqualPairs: []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
	})

	sourceOnly := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S1", "cluster": "c1", "hostname": "db1", "severity": "critical"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	twoSided := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "S2", "cluster": "c1", "hostname": "db1", "severity": "warning"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(2 * time.Hour),
	}, time.Time{}, false)
	r.cache.set(sourceOnly)
	r.cache.set(twoSided)

	target := model.LabelSet{"alertname": "T", "cluster": "c1", "remote": "db1", "severity": "warning"}
	fp, ok := r.hasEqual(target, true, now)
	require.True(t, ok)
	require.Equal(t, sourceOnly.Fingerprint(), fp)
}

func TestInhibitRulePairWithNoConstraintsByConfig(t *testing.T) {
	t.Parallel()

	// Zero-constraint rule: empty key, matches any cached source, same as
	// a pre-change rule without an equal list.
	r := NewInhibitRule(amcommoncfg.InhibitRule{})
	now := time.Now()
	r.cache.set(alert.New(model.Alert{
		Labels:   model.LabelSet{"any": "alert"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false))

	_, found := r.hasEqual(model.LabelSet{"any": "alert"}, false, now)
	require.True(t, found, "a rule without constraints matches any cached source alert")
}

func TestInhibitRuleHasEqualPairs(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cases := []struct {
		name        string
		src         model.LabelSet
		pairs       []amcommoncfg.LabelPair
		target      model.LabelSet
		excludeBoth bool
		want        bool
	}{
		{
			name: "pair and equal both match",
			src:  model.LabelSet{"cluster": "prod", "hostname": "db1"},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{"cluster": "prod", "remote": "db1"},
			want:   true,
		},
		{
			name: "equal holds but pair values differ",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{
				"cluster": "prod",
				"remote":  "db2",
			},
			want: false,
		},
		{
			name: "pair target label missing on the target",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{
				"cluster": "prod",
			},
			want: false,
		},
		{
			name: "pair source label missing on the source",
			src: model.LabelSet{
				"cluster": "prod",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{
				"cluster": "prod",
				"remote":  "db1",
			},
			want: false,
		},
		{
			name: "both pair labels missing compare as the empty string",
			src: model.LabelSet{
				"cluster": "prod",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{
				"cluster": "prod",
			},
			want: true,
		},
		{
			name: "AND across all pairs: one failing pair disables the rule",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
				"zone":     "z1",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
				{SourceLabel: "zone", TargetLabel: "region"},
			},
			target: model.LabelSet{
				"cluster": "prod",
				"remote":  "db1",
				"region":  "z2",
			},
			want: false,
		},
		{
			name: "AND across all pairs: all hold",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
				"zone":     "z1",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
				{SourceLabel: "zone", TargetLabel: "region"},
			},
			target: model.LabelSet{
				"cluster": "prod",
				"remote":  "db1",
				"region":  "z1",
			},
			want: true,
		},
		{
			name: "resolved source alert does not inhibit",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{
				"cluster": "prod",
				"remote":  "db1",
			},
			want: false,
		},
		{
			name: "two-sided source is skipped when exclusion is active",
			src: model.LabelSet{
				"cluster":  "prod",
				"hostname": "db1",
				"t":        "1", // matches the rule's target matcher too
			},
			pairs: []amcommoncfg.LabelPair{
				{SourceLabel: "hostname", TargetLabel: "remote"},
			},
			target: model.LabelSet{"cluster": "prod", "remote": "db1", "t": "1"},
			// Only source is two-sided, so it is skipped.
			excludeBoth: true,
			want:        false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewInhibitRule(amcommoncfg.InhibitRule{
				TargetMatchers: amcommoncfg.Matchers{
					&labels.Matcher{Type: labels.MatchEqual, Name: "t", Value: "1"},
				},
				Equal:      []string{"cluster"},
				EqualPairs: c.pairs,
			})

			src := alert.New(model.Alert{
				Labels:   c.src,
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(time.Hour),
			}, time.Time{}, false)
			if c.name == "resolved source alert does not inhibit" {
				src.EndsAt = now.Add(-time.Second)
			}
			r.cache.set(src)

			fp, found := r.hasEqual(c.target, c.excludeBoth, now)
			require.Equal(t, c.want, found, "want %t, got %t", c.want, found)
			if c.want {
				require.Equal(t, src.Fingerprint(), fp)
			}
		})
	}
}

func TestInhibitCombinedEqualAndPair(t *testing.T) {
	t.Parallel()

	// End to end through Mutes: RouterDown inhibits the InterfaceDown alerts
	// of neighbouring routers in the same site that face the down router.
	now := time.Now()
	ih := NewInhibitor(nil, []amcommoncfg.InhibitRule{{
		Name: "router-down-mutes-facing-interface-down",
		SourceMatchers: amcommoncfg.Matchers{
			&labels.Matcher{Type: labels.MatchEqual, Name: "alertname", Value: "RouterDown"},
		},
		TargetMatchers: amcommoncfg.Matchers{
			&labels.Matcher{Type: labels.MatchEqual, Name: "alertname", Value: "InterfaceDown"},
		},
		Equal:      []string{"site"},
		EqualPairs: []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
	}}, nopLogger, eventrecorder.NopRecorder())

	router := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "RouterDown", "hostname": "r1", "site": "fra"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	ih.rules[0].cache.set(router)

	cases := []struct {
		name   string
		target model.LabelSet
		muted  bool
	}{
		{
			name: "neighbour interface facing the down router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r2",
				"remote":    "r1",
				"site":      "fra",
			},
			muted: true,
		},
		{
			name: "pair mismatch: interface facing another router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r2",
				"remote":    "r3",
				"site":      "fra",
			},
			muted: false,
		},
		{
			name: "pair mismatch: interface located on the down router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r1",
				"remote":    "r2",
				"site":      "fra",
			},
			muted: false,
		},
		{
			name: "equal mismatch: site differs from the router site",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r2",
				"remote":    "r1",
				"site":      "ams",
			},
			muted: false,
		},
		{
			name: "remote missing: compares as the empty string",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r2",
				"site":      "fra",
			},
			muted: false,
		},
		{
			name: "site missing: compares as the empty string",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"hostname":  "r2",
				"remote":    "r1",
			},
			muted: false,
		},
		{
			name:   "target matcher mismatch: different alertname",
			target: model.LabelSet{"alertname": "NodeDown", "hostname": "r2", "remote": "r1", "site": "fra"},
			muted:  false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkMutes(t, ih, c.target, c.muted, "target %v", c.target)
		})
	}
}

func TestInhibitIssue2254RouterDown(t *testing.T) {
	t.Parallel()

	// Issue #2254 end to end through Mutes: a down router mutes the
	// InterfaceDown alerts on itself (equal hostname) and on the far end of
	// its links (pair hostname -> remote), but not unrelated links.
	now := time.Now()
	sourceMatchers := amcommoncfg.Matchers{
		&labels.Matcher{Type: labels.MatchEqual, Name: "alertname", Value: "RouterDown"},
	}
	targetMatchers := amcommoncfg.Matchers{
		&labels.Matcher{Type: labels.MatchEqual, Name: "alertname", Value: "InterfaceDown"},
	}
	ih := NewInhibitor(nil, []amcommoncfg.InhibitRule{
		{
			Name:           "router-down-mutes-local-interface-down",
			SourceMatchers: sourceMatchers,
			TargetMatchers: targetMatchers,
			Equal:          []string{"hostname"},
		},
		{
			Name:           "router-down-mutes-remote-interface-down",
			SourceMatchers: sourceMatchers,
			TargetMatchers: targetMatchers,
			EqualPairs:     []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
		},
	}, nopLogger, eventrecorder.NopRecorder())

	router := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "RouterDown", "instance": "192.168.0.1", "hostname": "r1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	for _, r := range ih.rules {
		r.cache.set(router)
	}

	cases := []struct {
		name   string
		target model.LabelSet
		muted  bool
	}{
		{
			name: "interface on the down router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"instance":  "10.0.0.2",
				"hostname":  "r1",
				"interface": "Te0/2/0/1",
				"remote":    "r2",
			},
			muted: true,
		},
		{
			name: "interface on a neighbour facing the down router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"instance":  "10.0.0.1",
				"hostname":  "r2",
				"interface": "Te0/7/0/12",
				"remote":    "r1",
			},
			muted: true,
		},
		{
			name: "interface on a neighbour facing another router",
			target: model.LabelSet{
				"alertname": "InterfaceDown",
				"instance":  "10.0.0.5",
				"hostname":  "r2",
				"interface": "Te0/7/0/13",
				"remote":    "r3",
			},
			muted: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkMutes(t, ih, c.target, c.muted, "target %v", c.target)
		})
	}
}

func TestInhibitRulePairProbeDiscriminatesSourceAlerts(t *testing.T) {
	t.Parallel()

	// Two sources differing only in the pair's source label: the probe
	// must find exactly the matching one.
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		Equal:      []string{"cluster"},
		EqualPairs: []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
	})
	src1 := alert.New(model.Alert{
		Labels:   model.LabelSet{"cluster": "c1", "hostname": "db1"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	src2 := alert.New(model.Alert{
		Labels:   model.LabelSet{"cluster": "c1", "hostname": "db2"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, time.Time{}, false)
	r.cache.set(src1)
	r.cache.set(src2)

	fp, ok := r.hasEqual(model.LabelSet{"cluster": "c1", "remote": "db1"}, false, now)
	require.True(t, ok)
	require.Equal(t, src1.Fingerprint(), fp, "probe for remote=db1 must find the db1 source")

	fp, ok = r.hasEqual(model.LabelSet{"cluster": "c1", "remote": "db2"}, false, now)
	require.True(t, ok)
	require.Equal(t, src2.Fingerprint(), fp, "probe for remote=db2 must find the db2 source")

	_, ok = r.hasEqual(model.LabelSet{"cluster": "c1", "remote": "db3"}, false, now)
	require.False(t, ok, "no source alert has hostname=db3")
}

func TestInhibitRulePairGCRemovesEverySlot(t *testing.T) {
	t.Parallel()

	// gc must drop resolved sources from map and index alike; a wrong
	// key re-derivation would orphan entries.
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		Equal:      []string{"cluster"},
		EqualPairs: []amcommoncfg.LabelPair{{SourceLabel: "hostname", TargetLabel: "remote"}},
	})
	active := alert.New(model.Alert{
		Labels:   model.LabelSet{"cluster": "c1", "hostname": "db1"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(2 * time.Hour),
	}, time.Time{}, false)
	resolvedSameKey := alert.New(model.Alert{
		Labels:   model.LabelSet{"cluster": "c1", "hostname": "db1", "id": "resolved"},
		StartsAt: now.Add(-2 * time.Hour),
		EndsAt:   now.Add(-time.Minute),
	}, time.Time{}, false)
	resolvedOtherKey := alert.New(model.Alert{
		Labels:   model.LabelSet{"cluster": "c2", "hostname": "db9"},
		StartsAt: now.Add(-2 * time.Hour),
		EndsAt:   now.Add(-time.Hour),
	}, time.Time{}, false)
	r.cache.set(active)
	r.cache.set(resolvedSameKey)
	r.cache.set(resolvedOtherKey)
	require.Len(t, r.cache.alerts, 3)
	require.Len(t, r.cache.index, 2, "active and resolvedSameKey share one slot tuple, resolvedOtherKey another")

	r.cache.gc()

	require.Len(t, r.cache.alerts, 1)
	require.Contains(t, r.cache.alerts, active.Fingerprint())
	require.Len(t, r.cache.index, 1, "the resolvedOtherKey slot must be removed entirely")
	// No orphans: every indexed fingerprint is a stored alert.
	for key, fps := range r.cache.index {
		for fp := range fps {
			_, stored := r.cache.alerts[fp]
			require.True(t, stored, "orphaned fingerprint %x in index slot %x", fp, key)
		}
	}
	_, found := r.hasEqual(model.LabelSet{"cluster": "c1", "remote": "db1"}, false, now)
	require.True(t, found, "active source must still inhibit after gc")

	// After the last alert resolves, both maps end up empty.
	active.EndsAt = now.Add(-time.Second)
	r.cache.set(active)
	r.cache.gc()
	require.Empty(t, r.cache.alerts)
	require.Empty(t, r.cache.index, "all slots must be removed once every alert resolved")
}

// legacyFingerprintEquals is the pre-equal_pairs implementation, verbatim,
// so parity can be asserted against it.
func legacyFingerprintEquals(equal map[model.LabelName]struct{}, lset model.LabelSet) model.Fingerprint {
	equalSet := make(model.LabelSet, len(equal))
	for n := range equal {
		equalSet[n] = lset[n]
	}
	return equalSet.Fingerprint()
}

// TestFingerprintSourceParityEqualOnly proves equal-only rules build
// byte-identical keys with no extra allocations. Serial: AllocsPerRun.
func TestFingerprintSourceParityEqualOnly(t *testing.T) {
	for n := 0; n <= 6; n++ {
		var (
			names model.LabelNames
			equal = make(map[model.LabelName]struct{}, n)
			lset  = make(model.LabelSet, n+1)
		)
		for i := 0; i < n; i++ {
			name := model.LabelName(fmt.Sprintf("label_%d", i))
			names = append(names, name)
			equal[name] = struct{}{}
			lset[name] = model.LabelValue(fmt.Sprintf("value_%d", i%3))
		}
		lset["noise"] = "untracked"

		c := newCache(constraintsFor(names, nil))

		want := legacyFingerprintEquals(equal, lset)
		require.Equal(t, want, c.fingerprintSource(lset),
			"%d equal labels: new index key deviates from the legacy fingerprint", n)
		// Target-side projection agrees for identity constraints too.
		require.Equal(t, want, c.fingerprintTarget(lset))

		legacy := testing.AllocsPerRun(100, func() { legacyFingerprintEquals(equal, lset) })
		fast := testing.AllocsPerRun(100, func() { c.fingerprintSource(lset) })
		require.LessOrEqual(t, fast, legacy,
			"%d equal labels: identity fast path must not allocate more than the legacy path (legacy=%d, new=%d)", n, legacy, fast)
	}
}

func BenchmarkFingerprintEquals(b *testing.B) {
	// fingerprintSource with varying equal-label counts (identity slots),
	// i.e. the pure-equal fast path.
	for _, numLabels := range []int{1, 3, 5, 10} {
		b.Run(fmt.Sprintf("%d_equal_labels", numLabels), func(b *testing.B) {
			equalNames := make(model.LabelNames, numLabels)
			for i := range numLabels {
				equalNames[i] = model.LabelName(fmt.Sprintf("label_%d", i))
			}

			c := newCache(constraintsFor(equalNames, nil))

			// Create a label set with matching values
			lset := make(model.LabelSet, numLabels+2)
			lset["source"] = "true"
			lset["target"] = "true"
			for i := range numLabels {
				lset[model.LabelName(fmt.Sprintf("label_%d", i))] = model.LabelValue(fmt.Sprintf("value_%d", i))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				_ = c.fingerprintSource(lset)
			}
		})
	}
}
