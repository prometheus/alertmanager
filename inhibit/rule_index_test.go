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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestNewRuleIndex_EmptyRules(t *testing.T) {
	idx := newRuleIndex(nil)

	require.NotNil(t, idx)
	require.True(t, idx.useLinearScan)
	require.Empty(t, idx.allRules)
}

func TestNewRuleIndex_BelowThreshold(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
	}

	idx := newRuleIndex(rules)

	require.True(t, idx.useLinearScan)
	require.Empty(t, idx.exactIndex)
}

func TestNewRuleIndex_IndexedRules(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "staging"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "dev"),
			},
		},
	}

	idx := newRuleIndex(rules)

	require.False(t, idx.useLinearScan)
	require.Contains(t, idx.exactIndex, "cluster")
	require.Len(t, idx.exactIndex["cluster"], 3)
	require.Len(t, idx.exactIndex["cluster"]["prod"], 1)
	require.Len(t, idx.exactIndex["cluster"]["staging"], 1)
	require.Len(t, idx.exactIndex["cluster"]["dev"], 1)
}

func TestNewRuleIndex_HighOverlapMatchers(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "severity", "warning"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "severity", "warning"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "severity", "warning"),
			},
		},
	}

	idx := newRuleIndex(rules)

	require.False(t, idx.useLinearScan)
	require.Empty(t, idx.exactIndex)
	require.Len(t, idx.linearRules, 3)
}

func TestNewRuleIndex_RegexMatchers(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchRegexp, "cluster", "prod-.*"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchRegexp, "cluster", "staging-.*"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "env", "test"),
			},
		},
	}

	idx := newRuleIndex(rules)

	require.False(t, idx.useLinearScan)
	require.Contains(t, idx.exactIndex, "env")
	require.Len(t, idx.linearRules, 2)
}

func TestNewRuleIndex_MultipleMatchers(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
				newTestMatcher(t, labels.MatchEqual, "region", "us-east"),
			},
		},
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "staging"),
			},
		},
	}

	idx := newRuleIndex(rules)

	require.False(t, idx.useLinearScan)
	require.Contains(t, idx.exactIndex, "cluster")
	require.Contains(t, idx.exactIndex, "region")
	require.Len(t, idx.exactIndex["cluster"]["prod"], 1)
	require.Len(t, idx.exactIndex["region"]["us-east"], 1)
	require.NotContains(t, idx.singleMatcherRules, rules[0])
	require.Contains(t, idx.singleMatcherRules, rules[1])
}

func TestForEachCandidate_EmptyIndex(t *testing.T) {
	idx := newRuleIndex(nil)

	called := false
	result := idx.forEachCandidate(model.LabelSet{"foo": "bar"}, func(r *InhibitRule) bool {
		called = true
		return false
	})

	require.False(t, result)
	require.False(t, called)
}

func TestForEachCandidate_LinearScan(t *testing.T) {
	rules := []*InhibitRule{
		{
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
	}
	idx := newRuleIndex(rules)

	var visited []*InhibitRule
	result := idx.forEachCandidate(model.LabelSet{"cluster": "prod"}, func(r *InhibitRule) bool {
		visited = append(visited, r)
		return false
	})

	require.False(t, result)
	require.Len(t, visited, 1)
	require.Equal(t, rules[0], visited[0])
}

func TestForEachCandidate_IndexedLookup(t *testing.T) {
	rules := []*InhibitRule{
		{
			Name: "rule-prod",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
		{
			Name: "rule-staging",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "staging"),
			},
		},
		{
			Name: "rule-dev",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "dev"),
			},
		},
	}
	idx := newRuleIndex(rules)

	var visited []*InhibitRule
	result := idx.forEachCandidate(model.LabelSet{"cluster": "staging"}, func(r *InhibitRule) bool {
		visited = append(visited, r)
		return false
	})

	require.False(t, result)
	require.Len(t, visited, 1)
	require.Equal(t, "rule-staging", visited[0].Name)
}

func TestForEachCandidate_EarlyTermination(t *testing.T) {
	rules := []*InhibitRule{
		{
			Name: "rule-1",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
		{
			Name: "rule-2",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
	}
	idx := newRuleIndex(rules)

	var visited []*InhibitRule
	result := idx.forEachCandidate(model.LabelSet{"cluster": "prod"}, func(r *InhibitRule) bool {
		visited = append(visited, r)
		return true
	})

	require.True(t, result)
	require.Len(t, visited, 1)
}

func TestForEachCandidate_Deduplication(t *testing.T) {
	rule := &InhibitRule{
		Name: "multi-matcher",
		TargetMatchers: labels.Matchers{
			newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			newTestMatcher(t, labels.MatchEqual, "region", "us-east"),
		},
	}
	rules := []*InhibitRule{rule, {
		Name: "other",
		TargetMatchers: labels.Matchers{
			newTestMatcher(t, labels.MatchEqual, "cluster", "staging"),
		},
	}}
	idx := newRuleIndex(rules)

	var visited []*InhibitRule
	result := idx.forEachCandidate(model.LabelSet{
		"cluster": "prod",
		"region":  "us-east",
	}, func(r *InhibitRule) bool {
		visited = append(visited, r)
		return false
	})

	require.False(t, result)
	count := 0
	for _, v := range visited {
		if v.Name == "multi-matcher" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestForEachCandidate_LinearRulesIncluded(t *testing.T) {
	rules := []*InhibitRule{
		{
			Name: "indexed",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
		{
			Name: "linear-regex",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchRegexp, "env", ".*"),
			},
		},
	}
	idx := newRuleIndex(rules)

	var visited []string
	result := idx.forEachCandidate(model.LabelSet{"cluster": "prod", "env": "test"}, func(r *InhibitRule) bool {
		visited = append(visited, r.Name)
		return false
	})

	require.False(t, result)
	require.Contains(t, visited, "indexed")
	require.Contains(t, visited, "linear-regex")
}

func TestForEachCandidate_NoMatchingLabels(t *testing.T) {
	rules := []*InhibitRule{
		{
			Name: "rule-prod",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "prod"),
			},
		},
		{
			Name: "rule-staging",
			TargetMatchers: labels.Matchers{
				newTestMatcher(t, labels.MatchEqual, "cluster", "staging"),
			},
		},
	}
	idx := newRuleIndex(rules)

	var visited []*InhibitRule
	result := idx.forEachCandidate(model.LabelSet{"cluster": "dev"}, func(r *InhibitRule) bool {
		visited = append(visited, r)
		return false
	})

	require.False(t, result)
	require.Empty(t, visited)
}

func newTestMatcher(t *testing.T, op labels.MatchType, name, value string) *labels.Matcher {
	t.Helper()
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}
