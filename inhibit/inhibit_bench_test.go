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
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
)

// BenchmarkMutes benchmarks the Mutes method for the Muter interface
// for different numbers of inhibition rules.
func BenchmarkMutes(b *testing.B) {
	b.Run("1 inhibition rule, 1 inhibiting alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 1))
	})
	b.Run("10 inhibition rules, 1 inhibiting alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 10, 1))
	})
	b.Run("100 inhibition rules, 1 inhibiting alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 100, 1))
	})
	b.Run("1000 inhibition rules, 1 inhibiting alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1000, 1))
	})
	b.Run("10000 inhibition rules, 1 inhibiting alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 10000, 1))
	})
	b.Run("1 inhibition rule, 10 inhibiting alerts", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 10))
	})
	b.Run("1 inhibition rule, 100 inhibiting alerts", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 100))
	})
	b.Run("1 inhibition rule, 1000 inhibiting alerts", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 1000))
	})
	b.Run("1 inhibition rule, 10000 inhibiting alerts", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 10000))
	})
	b.Run("1 inhibition rule, 10000 same-equal alerts, source-only candidate", func(b *testing.B) {
		benchmarkMutes(b, sameEqualSourceOnlyBenchmark(b, 10000))
	})
	b.Run("1 inhibition rule, 100 inhibiting alerts, equal_pairs", func(b *testing.B) {
		benchmarkMutes(b, equalPairsBenchmark(b, 100))
	})
	b.Run("1000 inhibition rules, 1 inhibiting alert, equal_pairs", func(b *testing.B) {
		benchmarkMutes(b, equalPairsRulesBenchmark(b, 1000))
	})
	b.Run("100 inhibition rules, 1000 inhibiting alerts", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 100, 1000))
	})
	b.Run("10 inhibition rules, last rule matches", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 10))
	})
	b.Run("100 inhibition rules, last rule matches", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 100))
	})
	b.Run("1000 inhibition rules, last rule matches", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 1000))
	})
	b.Run("10000 inhibition rules, last rule matches", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 10000))
	})
}

// benchmarkOptions allows the declaration of a wide range of benchmarks.
type benchmarkOptions struct {
	// n is the total number of inhibition rules.
	n int
	// newRuleFunc creates the next inhibition rule. It is called n times.
	newRuleFunc func(idx int) amcommoncfg.InhibitRule
	// newAlertsFunc creates the inhibiting alerts for each inhibition rule.
	// It is called n times.
	newAlertsFunc func(idx int, r amcommoncfg.InhibitRule) []*alert.Alert
	// benchFunc runs the benchmark.
	benchFunc func(mutesFunc func(context.Context, labelset.LabelSet) bool) error
}

// allRulesMatchBenchmark returns a new benchmark where all inhibition rules
// inhibit the label dst=0. It supports a number of variations, including
// customization of the number of inhibition rules, and the number of
// inhibiting alerts per inhibition rule.
//
// The source matchers are suffixed with the position of the inhibition rule
// in the list (e.g. src=1, src=2, etc...). The target matchers are the same
// across all inhibition rules (dst=0).
//
// Each inhibition rule can have zero or more alerts that match the source
// matchers, and is determined with numInhibitingAlerts.
//
// It expects dst=0 to be muted and will fail if not.
func allRulesMatchBenchmark(b *testing.B, numInhibitionRules, numInhibitingAlerts int) benchmarkOptions {
	// Built once: in production the MuteStage passes the alert's precomputed
	// fingerprint along with its labels, so hashing is not part of Mutes.
	target := labelset.FromModel(model.LabelSet{"dst": "0"})
	return benchmarkOptions{
		n: numInhibitionRules,
		newRuleFunc: func(idx int) amcommoncfg.InhibitRule {
			return amcommoncfg.InhibitRule{
				SourceMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(idx)),
				},
				TargetMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
				},
			}
		},
		newAlertsFunc: func(idx int, _ amcommoncfg.InhibitRule) []*alert.Alert {
			var alerts []*alert.Alert
			for i := range numInhibitingAlerts {
				alerts = append(alerts, alert.New(model.Alert{
					Labels: model.LabelSet{
						"src": model.LabelValue(strconv.Itoa(idx)),
						"idx": model.LabelValue(strconv.Itoa(i)),
					},
				}, time.Time{}, false))
			}
			return alerts
		}, benchFunc: func(mutesFunc func(context.Context, labelset.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), target); !ok {
				return errors.New("expected dst=0 to be muted")
			}
			return nil
		},
	}
}

func sameEqualSourceOnlyBenchmark(b *testing.B, numInhibitingAlerts int) benchmarkOptions {
	now := time.Now()

	// Built once: in production the MuteStage passes the alert's precomputed
	// fingerprint along with its labels, so hashing is not part of Mutes.
	target := labelset.FromModel(model.LabelSet{"src": "1", "dst": "1", "eq": "1"})
	return benchmarkOptions{
		n: 1,
		newRuleFunc: func(_ int) amcommoncfg.InhibitRule {
			return amcommoncfg.InhibitRule{
				SourceMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", "1"),
				},
				TargetMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "1"),
				},
				Equal: []string{"eq"},
			}
		},
		newAlertsFunc: func(_ int, _ amcommoncfg.InhibitRule) []*alert.Alert {
			alerts := make([]*alert.Alert, 0, numInhibitingAlerts+1)
			for i := range numInhibitingAlerts {
				alerts = append(alerts, alert.New(model.Alert{
					Labels: model.LabelSet{
						"src": model.LabelValue("1"),
						"eq":  model.LabelValue("1"),
						"idx": model.LabelValue(strconv.Itoa(i)),
					},
					EndsAt: now.Add(time.Hour),
				}, time.Time{}, false))
			}
			alerts = append(alerts, alert.New(model.Alert{
				Labels: model.LabelSet{
					"src": model.LabelValue("1"),
					"dst": model.LabelValue("1"),
					"eq":  model.LabelValue("1"),
					"idx": model.LabelValue("two-sided"),
				},
				EndsAt: now.Add(2 * time.Hour),
			}, time.Time{}, false))
			return alerts
		},
		benchFunc: func(mutesFunc func(context.Context, labelset.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), target); !ok {
				return errors.New("expected source-and-target alert to be muted by a source-only alert")
			}
			return nil
		},
	}
}

// equalPairsBenchmark: one rule combining equal [eq] and pair eq->rc (shared source label).
func equalPairsBenchmark(b *testing.B, numInhibitingAlerts int) benchmarkOptions {
	now := time.Now()

	// Precomputed once: in production the fingerprint is handed to Mutes.
	target := labelset.FromModel(model.LabelSet{"src": "1", "dst": "1", "eq": "1", "rc": "1"})
	return benchmarkOptions{
		n: 1,
		newRuleFunc: func(_ int) amcommoncfg.InhibitRule {
			return amcommoncfg.InhibitRule{
				SourceMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", "1"),
				},
				TargetMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "1"),
				},
				Equal: []string{"eq"},
				EqualPairs: []amcommoncfg.LabelPair{
					{SourceLabel: "eq", TargetLabel: "rc"},
				},
			}
		},
		newAlertsFunc: func(_ int, _ amcommoncfg.InhibitRule) []*alert.Alert {
			alerts := make([]*alert.Alert, 0, numInhibitingAlerts)
			for i := range numInhibitingAlerts {
				alerts = append(alerts, alert.New(model.Alert{
					Labels: model.LabelSet{
						"src": model.LabelValue("1"),
						"eq":  model.LabelValue("1"),
						"idx": model.LabelValue(strconv.Itoa(i)),
					},
					EndsAt: now.Add(time.Hour),
				}, time.Time{}, false))
			}
			return alerts
		},
		benchFunc: func(mutesFunc func(context.Context, labelset.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), target); !ok {
				return errors.New("expected dst=1 to be muted via the equal pair")
			}
			return nil
		},
	}
}

// equalPairsRulesBenchmark: every rule has equal + pair; only the last rule matches.
func equalPairsRulesBenchmark(b *testing.B, numInhibitionRules int) benchmarkOptions {
	return benchmarkOptions{
		n: numInhibitionRules,
		newRuleFunc: func(idx int) amcommoncfg.InhibitRule {
			return amcommoncfg.InhibitRule{
				SourceMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(idx)),
				},
				TargetMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
				},
				Equal: []string{"eq"},
				EqualPairs: []amcommoncfg.LabelPair{
					{SourceLabel: "eq", TargetLabel: "rc"},
				},
			}
		},
		newAlertsFunc: func(idx int, _ amcommoncfg.InhibitRule) []*alert.Alert {
			// Only the last rule is given a source alert.
			if idx < numInhibitionRules-1 {
				return nil
			}
			return []*alert.Alert{alert.New(model.Alert{
				Labels: model.LabelSet{
					"src": model.LabelValue(strconv.Itoa(idx)),
					"eq":  model.LabelValue("0"),
				},
			}, time.Time{}, false)}
		},
		benchFunc: func(mutesFunc func(context.Context, labelset.LabelSet) bool) error {
			target := model.LabelSet{"dst": "0", "eq": "0", "rc": "0"}
			if ok := mutesFunc(context.Background(), labelset.FromModel(target)); !ok {
				return errors.New("expected dst=0 to be muted via the equal pair")
			}
			return nil
		},
	}
}

// lastRuleMatchesBenchmark returns a new benchmark where the last inhibition
// rule inhibits the label dst=0. All other inhibition rules are no-ops.
//
// The source matchers are suffixed with the position of the inhibition rule
// in the list (e.g. src=1, src=2, etc...). The target matchers are the same
// across all inhibition rules (dst=0).
//
// It expects dst=0 to be muted and will fail if not.
func lastRuleMatchesBenchmark(b *testing.B, n int) benchmarkOptions {
	// Built once: in production the MuteStage passes the alert's precomputed
	// fingerprint along with its labels, so hashing is not part of Mutes.
	target := labelset.FromModel(model.LabelSet{"dst": "0"})
	return benchmarkOptions{
		n: n,
		newRuleFunc: func(idx int) amcommoncfg.InhibitRule {
			return amcommoncfg.InhibitRule{
				SourceMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(idx)),
				},
				TargetMatchers: amcommoncfg.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
				},
			}
		},
		newAlertsFunc: func(idx int, _ amcommoncfg.InhibitRule) []*alert.Alert {
			// Do not create an alert unless it is the last inhibition rule.
			if idx < n-1 {
				return nil
			}
			return []*alert.Alert{alert.New(model.Alert{
				Labels: model.LabelSet{
					"src": model.LabelValue(strconv.Itoa(idx)),
				},
			}, time.Time{}, false)}
		}, benchFunc: func(mutesFunc func(context.Context, labelset.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), target); !ok {
				return errors.New("expected dst=0 to be muted")
			}
			return nil
		},
	}
}

func benchmarkMutes(b *testing.B, opts benchmarkOptions) {
	r := prometheus.NewRegistry()
	s, err := mem.NewAlerts(context.TODO(), time.Minute, 0, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), r, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	alerts, rules := benchmarkFromOptions(opts)
	for _, a := range alerts {
		if err = s.Put(context.Background(), a); err != nil {
			b.Fatal(err)
		}
	}

	ih := NewInhibitor(s, rules, promslog.NewNopLogger(), eventrecorder.NopRecorder())
	defer ih.Stop()
	go ih.Run()

	// Wait some time for the inhibitor to seed its cache.
	<-time.After(time.Second)

	for b.Loop() {
		require.NoError(b, opts.benchFunc(ih.Mutes))
	}
}

func benchmarkFromOptions(opts benchmarkOptions) ([]*alert.Alert, []amcommoncfg.InhibitRule) {
	var (
		alerts = make([]*alert.Alert, 0, opts.n)
		rules  = make([]amcommoncfg.InhibitRule, 0, opts.n)
	)
	for i := 0; i < opts.n; i++ {
		r := opts.newRuleFunc(i)
		alerts = append(alerts, opts.newAlertsFunc(i, r)...)
		rules = append(rules, r)
	}
	return alerts, rules
}

func mustNewMatcher(b *testing.B, op labels.MatchType, name, value string) *labels.Matcher {
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(b, err)
	return m
}

// BenchmarkExtremes brackets the time/memory/GC extremes of Mutes on the
// slot index: a probe builds a K-entry key (O(K) + one map allocation),
// probes O(1), then walks O(|set|) sources behind the probed key. All label
// names and values are fixed; constraint values are "v" unless noted.
//
//	Best/Time_pure_equal_1src        K=3 equal-only hit; the legacy byte-identical-key path.
//	Best/GC_pure_equal_10labels      K=10 equal-only hit; main-line 10-label parity.
//	Worst/Time_K10_10k_set_twosided  K=10, |set|=10k, whole bucket two-sided-rejected; the O(|set|) ceiling.
//	Worst/Time_K10_10k_nomatch       K=10, |set|=10k, one probe value off; empty-bucket reject.
//	Worst/Time_K10_1src_keybuild     K=10, |set|=1; key build in isolation.
//	Worst/GC_K10_keybuild_allocs     Same setup; the K=10 key's B/op & allocs.
//	Worst/Time_collision_slots       equal [a] + pairs a->x,y,z; the #2..#4 collision slots.
func BenchmarkExtremes(b *testing.B) {
	b.Run("Best/Time_pure_equal_1src", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:     []string{"a", "b", "c"},
			sources:   1,
			wantMuted: true,
		})
	})
	b.Run("Best/GC_pure_equal_10labels", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:     []string{"el0", "el1", "el2", "el3", "el4", "el5", "el6", "el7", "el8", "el9"},
			sources:   1,
			wantMuted: true,
		})
	})
	b.Run("Worst/Time_K10_10k_set_twosided", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:              extremesEqual5,
			pairs:              extremesPairs5,
			sources:            10000,
			targetMatchesSrc:   true,
			sourcesMatchTarget: true,
			wantMuted:          false,
		})
	})
	b.Run("Worst/Time_K10_10k_nomatch", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:              extremesEqual5,
			pairs:              extremesPairs5,
			sources:            10000,
			targetMatchesSrc:   true,
			sourcesMatchTarget: true,
			differ:             "pb4",
			wantMuted:          false,
		})
	})
	b.Run("Worst/Time_K10_1src_keybuild", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:     extremesEqual5,
			pairs:     extremesPairs5,
			sources:   1,
			wantMuted: true,
		})
	})
	b.Run("Worst/GC_K10_keybuild_allocs", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:     extremesEqual5,
			pairs:     extremesPairs5,
			sources:   1,
			wantMuted: true,
		})
	})
	b.Run("Worst/Time_collision_slots", func(b *testing.B) {
		benchmarkExtremes(b, extremesScen{
			equal:     []string{"a"},
			pairs:     [][2]string{{"a", "x"}, {"a", "y"}, {"a", "z"}},
			sources:   1000,
			wantMuted: true,
		})
	})
}

// extremesEqual5/extremesPairs5: the K=10 set of the K10 subtests.
var (
	extremesEqual5 = []string{"eq0", "eq1", "eq2", "eq3", "eq4"}
	extremesPairs5 = [][2]string{
		{"pa0", "pb0"},
		{"pa1", "pb1"},
		{"pa2", "pb2"},
		{"pa3", "pb3"},
		{"pa4", "pb4"},
	}
)

// extremesScen describes one subtest; all label values are fixed ("v").
type extremesScen struct {
	// `equal` label names (identity constraints).
	equal []string
	// `equal_pairs` as (source, target) label names.
	pairs [][2]string
	// Active sources sharing the rule's index key.
	sources int
	// Target also matches the source matcher: two-sided exclusion active.
	targetMatchesSrc bool
	// Every source matches the target matcher: whole bucket two-sided-rejected.
	sourcesMatchTarget bool
	// The one target-side constraint value set to "w" (empty bucket).
	differ string
	// Expected outcome, asserted each iteration.
	wantMuted bool
}

// extremesRule builds the rule: src=1 / dst=1 matchers plus the constraints.
func extremesRule(b *testing.B, scen extremesScen) amcommoncfg.InhibitRule {
	rule := amcommoncfg.InhibitRule{
		SourceMatchers: amcommoncfg.Matchers{mustNewMatcher(b, labels.MatchEqual, "src", "1")},
		TargetMatchers: amcommoncfg.Matchers{mustNewMatcher(b, labels.MatchEqual, "dst", "1")},
		Equal:          scen.equal,
	}
	for _, p := range scen.pairs {
		rule.EqualPairs = append(rule.EqualPairs, amcommoncfg.LabelPair{
			SourceLabel: model.LabelName(p[0]),
			TargetLabel: model.LabelName(p[1]),
		})
	}
	return rule
}

// sourceLabels builds one source alert's labels: src, idx, "v" on every
// constraint source label (plus dst when set).
func (s extremesScen) sourceLabels(i int) model.LabelSet {
	lset := model.LabelSet{
		"src": "1",
		"idx": model.LabelValue(strconv.Itoa(i)),
	}
	if s.sourcesMatchTarget {
		lset["dst"] = "1"
	}
	for _, name := range s.equal {
		lset[model.LabelName(name)] = "v"
	}
	for _, p := range s.pairs {
		lset[model.LabelName(p[0])] = "v"
	}
	return lset
}

// targetLabels builds the fixed target: dst, "v" on every constraint target
// label (src when set, differ's label as "w").
func (s extremesScen) targetLabels() model.LabelSet {
	lset := model.LabelSet{"dst": "1"}
	if s.targetMatchesSrc {
		lset["src"] = "1"
	}
	for _, name := range s.equal {
		if name == s.differ {
			lset[model.LabelName(name)] = "w"
		} else {
			lset[model.LabelName(name)] = "v"
		}
	}
	for _, p := range s.pairs {
		if p[1] == s.differ {
			lset[model.LabelName(p[1])] = "w"
		} else {
			lset[model.LabelName(p[1])] = "v"
		}
	}
	return lset
}

// benchmarkExtremes seeds the alerts then times Mutes on the fixed target;
// WaitForLoading stands in for the seed wait.
func benchmarkExtremes(b *testing.B, scen extremesScen) {
	r := prometheus.NewRegistry()
	s, err := mem.NewAlerts(context.TODO(), time.Minute, 0, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), r, nil)
	require.NoError(b, err)
	defer s.Close()

	rule := extremesRule(b, scen)
	endsAt := time.Now().Add(time.Hour)
	for i := range scen.sources {
		a := alert.New(model.Alert{
			Labels: scen.sourceLabels(i),
			EndsAt: endsAt,
		}, time.Time{}, false)
		require.NoError(b, s.Put(context.Background(), a))
	}

	ih := NewInhibitor(s, []amcommoncfg.InhibitRule{rule}, promslog.NewNopLogger(), eventrecorder.NopRecorder())
	defer ih.Stop()
	go ih.Run()
	ih.WaitForLoading()

	target := labelset.FromModel(scen.targetLabels())
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		require.Equal(b, scen.wantMuted, ih.Mutes(context.Background(), target))
	}
}
