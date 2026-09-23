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
	got := ih.Mutes(ctx, target)
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
			src := Source{
				Equal: equal,
				cache: newCache(equal),
			}
			for _, v := range c.initial {
				src.cache.set(v)
			}

			if _, have := src.hasEqual(c.input, c.excludeTwoSidedMatch, time.Now(), c.targetMatchers); have != c.result {
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
	src := ih.rules[0].Sources[0]

	target := model.LabelSet{"s": "1", "t": "1", "e": "1"}
	_, found := src.hasEqual(target, true, now, ih.rules[0].TargetMatchers)
	require.True(t, found)

	src.cache.gc()

	_, found = src.hasEqual(target, true, now, ih.rules[0].TargetMatchers)
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
	src := ih.rules[0].Sources[0]

	src.cache.gc()

	_, found := src.hasEqual(model.LabelSet{"t": "1", "e": "1"}, false, now, ih.rules[0].TargetMatchers)
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

	ih.rules[0].Sources[0].cache.set(sourceAlert1)
	ih.rules[1].Sources[0].cache.set(sourceAlert2)

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

	ih.rules[0].Sources[0].cache.set(sourceAlert1)
	ih.rules[1].Sources[0].cache.set(sourceAlert2)

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
	c := newCache(map[model.LabelName]struct{}{
		"cluster": {},
		"service": {},
	})

	lset := model.LabelSet{
		"cluster":  "prod",
		"service":  "api",
		"instance": "host1",
	}

	fp := c.fingerprintEquals(lset)

	// Same equal labels should produce same fingerprint
	lset2 := model.LabelSet{
		"cluster":  "prod",
		"service":  "api",
		"instance": "host2", // different non-equal label
	}
	require.Equal(t, fp, c.fingerprintEquals(lset2))

	// Different equal label value should produce different fingerprint
	lset3 := model.LabelSet{
		"cluster": "staging",
		"service": "api",
	}
	require.NotEqual(t, fp, c.fingerprintEquals(lset3))
}

func TestInhibitRuleIndexSurvivesGC(t *testing.T) {
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{Equal: []string{"cluster"}})
	src := r.Sources[0]

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
	src.cache.set(active)
	src.cache.set(resolved)

	target := model.LabelSet{"alertname": "T", "cluster": "c1"}
	fp, ok := src.hasEqual(target, false, now, r.TargetMatchers)
	require.True(t, ok)
	require.Equal(t, active.Fingerprint(), fp)

	src.cache.gc()
	require.Len(t, src.cache.alerts, 1)
	require.Contains(t, src.cache.alerts, active.Fingerprint())

	fp, ok = src.hasEqual(target, false, now, r.TargetMatchers)
	require.True(t, ok, "active source alert must still inhibit after GC of a sibling")
	require.Equal(t, active.Fingerprint(), fp)
	require.Len(t, src.cache.index, 1)

	active.EndsAt = now.Add(-time.Second)
	src.cache.set(active)
	src.cache.gc()
	_, ok = src.hasEqual(target, false, now, r.TargetMatchers)
	require.False(t, ok)
	require.Empty(t, src.cache.alerts)
	require.Empty(t, src.cache.index, "empty index keys must be removed")
}

func TestInhibitRuleTwoSidedDoesNotShadow(t *testing.T) {
	now := time.Now()
	r := NewInhibitRule(amcommoncfg.InhibitRule{
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "warning"}},
		Equal:          []string{"cluster"},
	})
	src := r.Sources[0]

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
	src.cache.set(sourceOnly)
	src.cache.set(twoSided)

	target := model.LabelSet{"alertname": "T", "cluster": "c1", "severity": "warning"}
	fp, ok := src.hasEqual(target, true, now, r.TargetMatchers)
	require.True(t, ok)
	require.Equal(t, sourceOnly.Fingerprint(), fp)
}

func TestInhibitRuleMatchersWithSources(t *testing.T) {
	t.Parallel()

	rule1 := amcommoncfg.InhibitRule{
		Sources: []amcommoncfg.InhibitRuleSource{
			{
				SrcMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s1", Value: "1"}},
				Equal:       []string{"e"},
			},
		},
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchNotEqual, Name: "t1", Value: "1"}},
	}
	rule2 := amcommoncfg.InhibitRule{
		Sources: []amcommoncfg.InhibitRuleSource{
			{
				SrcMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "s2", Value: "1"}},
				Equal:       []string{"e"},
			},
		},
		TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "t2", Value: "1"}},
	}

	ih := NewInhibitor(nil, []amcommoncfg.InhibitRule{rule1, rule2}, nopLogger, eventrecorder.NopRecorder())
	now := time.Now()
	sourceAlert1 := &alert.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s1": "1", "t1": "2", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}
	sourceAlert2 := &alert.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		},
	}

	ih.rules[0].Sources[0].cache.set(sourceAlert1)
	ih.rules[1].Sources[0].cache.set(sourceAlert2)

	cases := []struct {
		target   model.LabelSet
		expected bool
	}{
		{
			target:   model.LabelSet{"t1": "1", "e": "1"},
			expected: false,
		},
		{
			target:   model.LabelSet{"t2": "1", "e": "1"},
			expected: true,
		},
		{
			target:   model.LabelSet{"t1": "1", "t2": "1", "e": "1"},
			expected: true,
		},
		{
			// Rule2's two-sided match excludes inhibition by rule2, but rule1's
			// target matcher (t1!=1) still matches and inhibits via rule1.
			target:   model.LabelSet{"s2": "1", "t2": "1", "e": "1"},
			expected: true,
		},
	}

	for _, c := range cases {
		checkMutes(t, ih, c.target, c.expected, "target %v", c.target)
	}
}

func TestInhibitByMultipleSources(t *testing.T) {
	t.Parallel()

	now := time.Now()
	inhibitRules := func() []amcommoncfg.InhibitRule {
		return []amcommoncfg.InhibitRule{
			{
				Sources: []amcommoncfg.InhibitRuleSource{
					{
						SrcMatchers: amcommoncfg.Matchers{
							&labels.Matcher{Type: labels.MatchEqual, Name: "s1", Value: "1"},
							&labels.Matcher{Type: labels.MatchEqual, Name: "s11", Value: "1"},
						},
						Equal: []string{"e"},
					},
					{
						SrcMatchers: amcommoncfg.Matchers{
							&labels.Matcher{Type: labels.MatchEqual, Name: "s2", Value: "1"},
							&labels.Matcher{Type: labels.MatchEqual, Name: "s22", Value: "1"},
						},
						Equal: []string{"f"},
					},
				},
				TargetMatchers: amcommoncfg.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "t", Value: "1"}},
			},
		}
	}
	alertOne := func() *alert.Alert {
		return &alert.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"t": "1", "e": "1", "f": "1"},
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(time.Hour),
			},
		}
	}
	alertTwo := func(resolved bool) *alert.Alert {
		var end time.Time
		if resolved {
			end = now.Add(-time.Second)
		} else {
			end = now.Add(time.Hour)
		}
		return &alert.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"s1": "1", "s11": "1", "e": "1"},
				StartsAt: now.Add(-time.Minute),
				EndsAt:   end,
			},
		}
	}
	alertThree := func(resolved bool) *alert.Alert {
		var end time.Time
		if resolved {
			end = now.Add(-time.Second)
		} else {
			end = now.Add(time.Hour)
		}
		return &alert.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"s2": "1", "s22": "1", "f": "1"},
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
		alerts   []*alert.Alert
		expected []exp
	}{
		{
			// No source alerts cached, so nothing with t=1 should be muted.
			alerts: []*alert.Alert{alertOne()},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "1", "f": "1"},
					muted: false,
				},
			},
		},
		{
			// Source 1 (alertTwo) is active but source 2 (alertThree) is resolved.
			// AND fails — nothing should be muted.
			alerts: []*alert.Alert{alertOne(), alertTwo(false), alertThree(true)},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "1", "f": "1"},
					muted: false,
				},
				{
					lbls:  model.LabelSet{"s1": "1", "s11": "1", "e": "1"},
					muted: false,
				},
				{
					lbls:  model.LabelSet{"s2": "1", "s22": "1", "f": "1"},
					muted: false,
				},
			},
		},
		{
			// Source 1 (alertTwo) is resolved but source 2 (alertThree) is active.
			// AND fails — nothing should be muted.
			alerts: []*alert.Alert{alertOne(), alertTwo(true), alertThree(false)},
			expected: []exp{
				{
					lbls:  model.LabelSet{"t": "1", "e": "1", "f": "1"},
					muted: false,
				},
				{
					lbls:  model.LabelSet{"s1": "1", "e": "1", "f": "1"},
					muted: false,
				},
				{
					lbls:  model.LabelSet{"s2": "1", "e": "1", "f": "1"},
					muted: false,
				},
			},
		},
		{
			// Both sources active. Targets are muted only when both equal labels match.
			alerts: []*alert.Alert{alertOne(), alertTwo(false), alertThree(false)},
			expected: []exp{
				{
					// t=1 matches target, but e is missing so source 1 equal check fails.
					lbls:  model.LabelSet{"t": "1", "f": "5"},
					muted: false,
				},
				{
					// t=1 matches target, e=1 matches source 1, f=1 matches source 2.
					lbls:  model.LabelSet{"t": "1", "f": "1", "e": "1"},
					muted: true,
				},
				{
					// Extra labels are ignored. t=1 matches, e=1 and f=1 match both sources.
					lbls:  model.LabelSet{"s3": "1", "t": "1", "s11": "1", "e": "1", "f": "1"},
					muted: true,
				},
				{
					// t=1 matches target, but e=2 doesn't match source 1's cached e=1.
					lbls:  model.LabelSet{"t": "1", "e": "2", "f": "1"},
					muted: false,
				},
				{
					// t=1 matches target, but f=4 doesn't match source 2's cached f=1.
					lbls:  model.LabelSet{"t": "1", "e": "1", "f": "4"},
					muted: false,
				},
			},
		},
	} {
		ap := newFakeAlerts(tc.alerts)
		inhibitor := NewInhibitor(ap, inhibitRules(), nopLogger, eventrecorder.NopRecorder())

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

func BenchmarkFingerprintEquals(b *testing.B) {
	// Test fingerprintEquals with varying number of equal labels
	for _, numLabels := range []int{1, 3, 5, 10} {
		b.Run(fmt.Sprintf("%d_equal_labels", numLabels), func(b *testing.B) {
			equalLabels := make(map[model.LabelName]struct{}, numLabels)
			for i := range numLabels {
				equalLabels[model.LabelName(fmt.Sprintf("label_%d", i))] = struct{}{}
			}

			c := newCache(equalLabels)

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
				_ = c.fingerprintEquals(lset)
			}
		})
	}
}
