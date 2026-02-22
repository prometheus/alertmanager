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

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
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
	newRuleFunc func(idx int) config.InhibitRule
	// newAlertsFunc creates the inhibiting alerts for each inhibition rule.
	// It is called n times.
	newAlertsFunc func(idx int, r config.InhibitRule) []types.Alert
	// benchFunc runs the benchmark.
	benchFunc func(mutesFunc func(context.Context, model.LabelSet) bool) error
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
	return benchmarkOptions{
		n: numInhibitionRules,
		newRuleFunc: func(idx int) config.InhibitRule {
			return config.InhibitRule{
				SourceMatchers: config.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(idx)),
				},
				TargetMatchers: config.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
				},
			}
		},
		newAlertsFunc: func(idx int, _ config.InhibitRule) []types.Alert {
			var alerts []types.Alert
			for i := range numInhibitingAlerts {
				alerts = append(alerts, types.Alert{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"src": model.LabelValue(strconv.Itoa(idx)),
							"idx": model.LabelValue(strconv.Itoa(i)),
						},
					},
				})
			}
			return alerts
		}, benchFunc: func(mutesFunc func(context.Context, model.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), model.LabelSet{"dst": "0"}); !ok {
				return errors.New("expected dst=0 to be muted")
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
	return benchmarkOptions{
		n: n,
		newRuleFunc: func(idx int) config.InhibitRule {
			return config.InhibitRule{
				SourceMatchers: config.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(idx)),
				},
				TargetMatchers: config.Matchers{
					mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
				},
			}
		},
		newAlertsFunc: func(idx int, _ config.InhibitRule) []types.Alert {
			// Do not create an alert unless it is the last inhibition rule.
			if idx < n-1 {
				return nil
			}
			return []types.Alert{{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"src": model.LabelValue(strconv.Itoa(idx)),
					},
				},
			}}
		}, benchFunc: func(mutesFunc func(context.Context, model.LabelSet) bool) error {
			if ok := mutesFunc(context.Background(), model.LabelSet{"dst": "0"}); !ok {
				return errors.New("expected dst=0 to be muted")
			}
			return nil
		},
	}
}

func benchmarkMutes(b *testing.B, opts benchmarkOptions) {
	r := prometheus.NewRegistry()
	m := types.NewMarker(r)
	s, err := mem.NewAlerts(context.TODO(), m, time.Minute, 0, nil, promslog.NewNopLogger(), r, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	alerts, rules := benchmarkFromOptions(opts)
	for _, a := range alerts {
		tmp := a
		if err = s.Put(context.Background(), &tmp); err != nil {
			b.Fatal(err)
		}
	}

	ih := NewInhibitor(s, rules, m, promslog.NewNopLogger())
	defer ih.Stop()
	go ih.Run()

	// Wait some time for the inhibitor to seed its cache.
	<-time.After(time.Second)

	for b.Loop() {
		require.NoError(b, opts.benchFunc(ih.Mutes))
	}
}

func benchmarkFromOptions(opts benchmarkOptions) ([]types.Alert, []config.InhibitRule) {
	var (
		alerts = make([]types.Alert, 0, opts.n)
		rules  = make([]config.InhibitRule, 0, opts.n)
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

func BenchmarkMutesScaling(b *testing.B) {
	b.Run("different_targets", func(b *testing.B) {
		for _, numRules := range []int{10, 100, 1000} {
			b.Run("rules="+strconv.Itoa(numRules), func(b *testing.B) {
				benchmarkDifferentTargets(b, numRules)
			})
		}
	})

	b.Run("same_target", func(b *testing.B) {
		for _, numRules := range []int{10, 100, 1000} {
			b.Run("rules="+strconv.Itoa(numRules), func(b *testing.B) {
				benchmarkSameTarget(b, numRules)
			})
		}
	})

	b.Run("no_match", func(b *testing.B) {
		for _, numRules := range []int{10, 100, 1000} {
			b.Run("rules="+strconv.Itoa(numRules), func(b *testing.B) {
				benchmarkNoMatch(b, numRules)
			})
		}
	})
}

func benchmarkDifferentTargets(b *testing.B, numRules int) {
	r := prometheus.NewRegistry()
	m := types.NewMarker(r)
	s, err := mem.NewAlerts(context.TODO(), m, time.Minute, 0, nil, promslog.NewNopLogger(), r, nil)
	require.NoError(b, err)
	defer s.Close()

	rules := make([]config.InhibitRule, numRules)
	for i := range numRules {
		rules[i] = config.InhibitRule{
			SourceMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "alertname", "SourceAlert"),
				mustNewMatcher(b, labels.MatchEqual, "cluster", strconv.Itoa(i)),
			},
			TargetMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "severity", "warning"),
				mustNewMatcher(b, labels.MatchEqual, "cluster", strconv.Itoa(i)),
			},
		}
	}

	// Source alert for the LAST rule (worst case for linear scan)
	lastCluster := strconv.Itoa(numRules - 1)
	alert := types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"alertname": "SourceAlert",
				"cluster":   model.LabelValue(lastCluster),
			},
		},
	}
	require.NoError(b, s.Put(context.Background(), &alert))

	ih := NewInhibitor(s, rules, m, promslog.NewNopLogger())
	defer ih.Stop()
	go ih.Run()
	<-time.After(time.Second)

	targetLset := model.LabelSet{
		"alertname": "TargetAlert",
		"severity":  "warning",
		"cluster":   model.LabelValue(lastCluster),
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if !ih.Mutes(ctx, targetLset) {
			b.Fatal("expected alert to be muted")
		}
	}
}

func benchmarkSameTarget(b *testing.B, numRules int) {
	r := prometheus.NewRegistry()
	m := types.NewMarker(r)
	s, err := mem.NewAlerts(context.TODO(), m, time.Minute, 0, nil, promslog.NewNopLogger(), r, nil)
	require.NoError(b, err)
	defer s.Close()

	rules := make([]config.InhibitRule, numRules)
	for i := range numRules {
		rules[i] = config.InhibitRule{
			SourceMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "src", strconv.Itoa(i)),
			},
			TargetMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "dst", "0"),
			},
		}
	}

	// Source alert for the LAST rule only
	alert := types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"src": model.LabelValue(strconv.Itoa(numRules - 1)),
			},
		},
	}
	require.NoError(b, s.Put(context.Background(), &alert))

	ih := NewInhibitor(s, rules, m, promslog.NewNopLogger())
	defer ih.Stop()
	go ih.Run()
	<-time.After(time.Second)

	targetLset := model.LabelSet{"dst": "0"}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if !ih.Mutes(ctx, targetLset) {
			b.Fatal("expected alert to be muted")
		}
	}
}

func benchmarkNoMatch(b *testing.B, numRules int) {
	r := prometheus.NewRegistry()
	m := types.NewMarker(r)
	s, err := mem.NewAlerts(context.TODO(), m, time.Minute, 0, nil, promslog.NewNopLogger(), r, nil)
	require.NoError(b, err)
	defer s.Close()

	rules := make([]config.InhibitRule, numRules)
	for i := range numRules {
		rules[i] = config.InhibitRule{
			SourceMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "alertname", "SourceAlert"),
			},
			TargetMatchers: config.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "cluster", strconv.Itoa(i)),
			},
		}
	}

	ih := NewInhibitor(s, rules, m, promslog.NewNopLogger())
	defer ih.Stop()
	go ih.Run()
	<-time.After(time.Second)

	// Alert with cluster that doesn't match any rule
	targetLset := model.LabelSet{
		"alertname": "TargetAlert",
		"cluster":   "nonexistent",
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if ih.Mutes(ctx, targetLset) {
			b.Fatal("expected alert to NOT be muted")
		}
	}
}

// BenchmarkMinRulesForIndexThreshold compares linear vs indexed lookup at various rule counts.
//
// Results (ns/op):
//
//	rules | linear | indexed
//	   1  |     17 |      17
//	   2  |     29 |      85
//	   5  |     68 |      84
//	  10  |    135 |      94
//
// Crossover at ~7 rules. Default MinRulesForIndex=2 enables indexing early since
// high-overlap detection handles pathological cases.
func BenchmarkMinRulesForIndexThreshold(b *testing.B) {
	for _, numRules := range []int{1, 2, 3, 5, 10} {
		b.Run("rules="+strconv.Itoa(numRules), func(b *testing.B) {
			benchmarkRuleIndexThreshold(b, numRules)
		})
	}
}

func benchmarkRuleIndexThreshold(b *testing.B, numRules int) {
	rules := make([]*InhibitRule, numRules)
	for i := range numRules {
		rules[i] = &InhibitRule{
			TargetMatchers: labels.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "cluster", strconv.Itoa(i)),
			},
		}
	}

	lset := model.LabelSet{"cluster": "0"}

	b.Run("linear", func(b *testing.B) {
		opts := RuleIndexOptions{MinRulesForIndex: numRules + 1, MaxMatcherOverlapRatio: 0.5}
		idx := newRuleIndexWithOptions(rules, opts)

		b.ResetTimer()
		for b.Loop() {
			idx.forEachCandidate(lset, func(r *InhibitRule) bool {
				r.TargetMatchers.Matches(lset)
				return false
			})
		}
	})

	b.Run("indexed", func(b *testing.B) {
		opts := RuleIndexOptions{MinRulesForIndex: 1, MaxMatcherOverlapRatio: 0.5}
		idx := newRuleIndexWithOptions(rules, opts)

		b.ResetTimer()
		for b.Loop() {
			idx.forEachCandidate(lset, func(r *InhibitRule) bool {
				r.TargetMatchers.Matches(lset)
				return false
			})
		}
	})
}

// BenchmarkMaxMatcherOverlapRatio compares performance at various overlap thresholds.
//
// Results (ns/op):
//
//	ratio | time
//	 0.10 |  183
//	 0.20 |  185
//	 0.30 |  182
//	 0.40 |  185
//	 0.50 |  186
//	 0.60 |  552
//	 0.70 |  533
//	 0.80 |  546
//	 0.90 |  524
//	 1.00 |  571
//
// Clear cliff between 0.5 and 0.6 with 3x degradation. Default MaxMatcherOverlapRatio=0.5
// is optimal - highest value before performance degrades.
func BenchmarkMaxMatcherOverlapRatio(b *testing.B) {
	for _, ratio := range []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0} {
		b.Run("ratio="+strconv.FormatFloat(ratio, 'f', 2, 64), func(b *testing.B) {
			benchmarkOverlapRatio(b, ratio)
		})
	}
}

func benchmarkOverlapRatio(b *testing.B, ratio float64) {
	numRules := 100
	highOverlapCount := int(float64(numRules) * 0.6)

	rules := make([]*InhibitRule, numRules)
	for i := range highOverlapCount {
		rules[i] = &InhibitRule{
			TargetMatchers: labels.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "severity", "warning"),
			},
		}
	}
	for i := highOverlapCount; i < numRules; i++ {
		rules[i] = &InhibitRule{
			TargetMatchers: labels.Matchers{
				mustNewMatcher(b, labels.MatchEqual, "cluster", strconv.Itoa(i)),
			},
		}
	}

	opts := RuleIndexOptions{MinRulesForIndex: 2, MaxMatcherOverlapRatio: ratio}
	idx := newRuleIndexWithOptions(rules, opts)

	lset := model.LabelSet{"severity": "warning", "cluster": model.LabelValue(strconv.Itoa(highOverlapCount))}

	b.ResetTimer()
	b.ReportAllocs()

	var visited int
	for b.Loop() {
		visited = 0
		idx.forEachCandidate(lset, func(r *InhibitRule) bool {
			visited++
			return false
		})
	}
	_ = visited
}
