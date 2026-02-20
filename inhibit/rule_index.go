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
	"slices"
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/pkg/labels"
)

// matcherKey is used as a map key to avoid string concatenation allocations.
type matcherKey struct {
	name  string
	value string
}

// RuleIndexOptions configures the rule index behavior.
type RuleIndexOptions struct {
	// MinRulesForIndex is the minimum number of rules before indexing is used.
	MinRulesForIndex int

	// MaxMatcherOverlapRatio is the maximum fraction of rules a matcher can
	// appear in before being excluded from the index.
	MaxMatcherOverlapRatio float64
}

// DefaultRuleIndexOptions returns the default options for rule indexing.
func DefaultRuleIndexOptions() RuleIndexOptions {
	return RuleIndexOptions{
		MinRulesForIndex:       2,
		MaxMatcherOverlapRatio: 0.5,
	}
}

var visitedRulePool = sync.Pool{
	New: func() any {
		return make(map[*InhibitRule]struct{}, 16)
	},
}

func getVisitedRules() map[*InhibitRule]struct{} {
	return visitedRulePool.Get().(map[*InhibitRule]struct{})
}

func putVisitedRules(m map[*InhibitRule]struct{}) {
	clear(m)
	visitedRulePool.Put(m)
}

// ruleIndex provides O(k) rule candidate lookup instead of O(N) linear scan,
// where k = number of labels and N = number of inhibit rules.
type ruleIndex struct {
	// labelName -> labelValue -> rules with that equality target matcher
	exactIndex map[string]map[string][]*InhibitRule

	// rules with exactly one indexed equality target matcher (no dedup needed)
	singleMatcherRules map[*InhibitRule]struct{}

	// rules without indexed equality target matchers (must check linearly)
	linearRules []*InhibitRule

	// useLinearScan is true when rule count is below threshold
	useLinearScan bool

	allRules []*InhibitRule
}

func newRuleIndex(rules []*InhibitRule) *ruleIndex {
	return newRuleIndexWithOptions(rules, DefaultRuleIndexOptions())
}

func newRuleIndexWithOptions(rules []*InhibitRule, opts RuleIndexOptions) *ruleIndex {
	idx := &ruleIndex{
		exactIndex:         make(map[string]map[string][]*InhibitRule),
		singleMatcherRules: make(map[*InhibitRule]struct{}),
		linearRules:        nil,
		useLinearScan:      false,
		allRules:           rules,
	}

	// For small rule sets, linear scan is faster than index overhead
	if len(rules) < opts.MinRulesForIndex {
		idx.useLinearScan = true
		return idx
	}

	// First pass: count how many rules each matcher appears in to detect high-overlap
	matcherCount := make(map[matcherKey]int)
	for _, rule := range rules {
		for _, m := range rule.TargetMatchers {
			if m.Type == labels.MatchEqual {
				matcherCount[matcherKey{m.Name, m.Value}]++
			}
		}
	}

	// Determine which matchers are high-overlap and should be excluded
	maxOverlap := int(float64(len(rules)) * opts.MaxMatcherOverlapRatio)
	highOverlapMatchers := make(map[matcherKey]struct{}, len(matcherCount))
	for key, count := range matcherCount {
		if count > maxOverlap {
			highOverlapMatchers[key] = struct{}{}
		}
	}

	// Second pass: build index excluding high-overlap matchers
	for _, rule := range rules {
		// Count indexable matchers to determine if rule needs deduplication
		var indexableCount int
		for _, m := range rule.TargetMatchers {
			if m.Type != labels.MatchEqual {
				continue
			}
			if _, isHighOverlap := highOverlapMatchers[matcherKey{m.Name, m.Value}]; !isHighOverlap {
				indexableCount++
			}
		}

		if indexableCount == 0 {
			// No good indexable matchers, use linear scan for this rule
			idx.linearRules = append(idx.linearRules, rule)
			continue
		}

		if indexableCount == 1 {
			idx.singleMatcherRules[rule] = struct{}{}
		}

		// Add rule to index for each indexable matcher
		for _, m := range rule.TargetMatchers {
			if m.Type != labels.MatchEqual {
				continue
			}
			if _, isHighOverlap := highOverlapMatchers[matcherKey{m.Name, m.Value}]; isHighOverlap {
				continue
			}
			if idx.exactIndex[m.Name] == nil {
				idx.exactIndex[m.Name] = make(map[string][]*InhibitRule)
			}
			idx.exactIndex[m.Name][m.Value] = append(idx.exactIndex[m.Name][m.Value], rule)
		}
	}

	return idx
}

// forEachCandidate calls fn for each rule that might match the given label set.
// Returns true if any rule's callback returned true (indicating a match was found).
func (idx *ruleIndex) forEachCandidate(lset model.LabelSet, fn func(*InhibitRule) bool) bool {
	if len(idx.allRules) == 0 {
		return false
	}

	// Fast path: if rule count is small or no index was built, iterate all rules
	if idx.useLinearScan || len(idx.exactIndex) == 0 {
		return slices.ContainsFunc(idx.allRules, fn)
	}

	visited := getVisitedRules()
	defer putVisitedRules(visited)

	for labelName, labelValue := range lset {
		valueMap, ok := idx.exactIndex[string(labelName)]
		if !ok {
			continue
		}

		rules, ok := valueMap[string(labelValue)]
		if !ok {
			continue
		}

		for _, rule := range rules {
			// Rules with multiple indexed matchers need deduplication since they
			// appear in multiple index entries. Single-matcher rules can skip this.
			if _, isSingleMatcher := idx.singleMatcherRules[rule]; !isSingleMatcher {
				if _, seen := visited[rule]; seen {
					continue
				}
				visited[rule] = struct{}{}
			}

			if fn(rule) {
				return true
			}
		}
	}

	return slices.ContainsFunc(idx.linearRules, fn)
}
