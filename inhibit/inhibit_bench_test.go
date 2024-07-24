// Copyright 2024 The Prometheus Authors
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

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

// BenchmarkMutes benchmarks the Mutes method for the Muter interface
// for different numbers of inhibition rules.
func BenchmarkMutes(b *testing.B) {
	b.Run("1 inhibition rule, 1 inhibiting alert, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 1, 1))
	})
	b.Run("10 inhibition rules, 1 inhibiting alert, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 10, 1, 1))
	})
	b.Run("100 inhibition rules, 1 inhibiting alert, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 100, 1, 1))
	})
	b.Run("1000 inhibition rules, 1 inhibiting alert, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1000, 1, 1))
	})
	b.Run("10000 inhibition rules, 1 inhibiting alert, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 10000, 1, 1))
	})
	b.Run("1 inhibition rule, 10 inhibiting alerts, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 10, 1))
	})
	b.Run("1 inhibition rule, 100 inhibiting alerts, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 100, 1))
	})
	b.Run("1 inhibition rule, 100 inhibiting alerts, 100 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 100, 100))
	})
	b.Run("1 inhibition rule, 1000 inhibiting alerts, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 1000, 1))
	})
	b.Run("1 inhibition rule, 1000 inhibiting alerts, 1000 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 1000, 1000))
	})
	b.Run("1 inhibition rule, 10000 inhibiting alerts, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 1, 10000, 1))
	})
	b.Run("100 inhibition rules, 1000 inhibiting alerts, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, allRulesMatchBenchmark(b, 100, 1000, 1))
	})
	b.Run("10 inhibition rules, last rule matches, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 10, 1))
	})
	b.Run("100 inhibition rules, last rule matches, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 100, 1))
	})
	b.Run("100 inhibition rules, last rule matches, 100 target alerts", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 100, 100))
	})
	b.Run("1000 inhibition rules, last rule matches, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 1000, 1))
	})
	b.Run("1000 inhibition rules, last rule matches, 1000 target alert", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 1000, 1000))
	})
	b.Run("10000 inhibition rules, last rule matches, 1 target alert", func(b *testing.B) {
		benchmarkMutes(b, lastRuleMatchesBenchmark(b, 10000, 1))
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
	benchFunc func(mutesFunc func(model.LabelSet) bool) error
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
func allRulesMatchBenchmark(b *testing.B, numInhibitionRules, numInhibitingAlerts, numTargetAlerts int) benchmarkOptions {
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
			for i := 0; i < numInhibitingAlerts; i++ {
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
		}, benchFunc: func(mutesFunc func(set model.LabelSet) bool) error {
			for i := 0; i < numTargetAlerts; i++ {
				if ok := mutesFunc(model.LabelSet{"dst": "0"}); !ok {
					return errors.New("expected dst=0 to be muted")
				}
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
func lastRuleMatchesBenchmark(b *testing.B, n, numTargetAlerts int) benchmarkOptions {
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
		}, benchFunc: func(mutesFunc func(set model.LabelSet) bool) error {
			for i := 0; i < numTargetAlerts; i++ {
				if ok := mutesFunc(model.LabelSet{"dst": "0"}); !ok {
					return errors.New("expected dst=0 to be muted")
				}
			}
			return nil
		},
	}
}

func benchmarkMutes(b *testing.B, opts benchmarkOptions) {
	r := prometheus.NewRegistry()
	m := types.NewMarker(r)
	s, err := mem.NewAlerts(context.TODO(), m, time.Minute, nil, log.NewNopLogger(), r)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	alerts, rules := benchmarkFromOptions(opts)
	for _, a := range alerts {
		tmp := a
		if err = s.Put(&tmp); err != nil {
			b.Fatal(err)
		}
	}

	ih := NewInhibitor(s, rules, m, log.NewNopLogger())
	defer ih.Stop()
	go ih.Run()

	// Wait some time for the inhibitor to seed its cache.
	<-time.After(time.Second)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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
