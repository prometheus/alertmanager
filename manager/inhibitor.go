// Copyright 2013 Prometheus Team
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

package manager

import (
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
)

type InhibitRules []*InhibitRule

type InhibitRule struct {
	SourceFilters Filters
	TargetFilters Filters
	MatchOn       model.LabelNames
}

// Returns those target label sets which are not inhibited by any of the
// source label sets.
func (i *InhibitRule) Filter(s, t []model.LabelSet) []model.LabelSet {
	s = i.SourceFilters.Filter(s)
	out := []model.LabelSet{}
	for _, tl := range t {
		inhibited := false
		if i.TargetFilters.Handles(tl) {
			for _, sl := range s {
				if matchOnLabels(tl, sl, i.MatchOn) {
					inhibited = true
					break
				}
			}
		}
		if !inhibited {
			out = append(out, tl)
		}
	}
	return out
}

func matchOnLabels(l, r model.LabelSet, labels model.LabelNames) bool {
	for _, ln := range labels {
		if l[ln] != r[ln] {
			return false
		}
	}
	return true
}

// Inhibitor calculates inhibition rules between its labelset inputs and only
// emits uninhibited alert labelsets.
type Inhibitor struct {
	mu           sync.Mutex
	inhibitRules []*InhibitRule
	dirty        bool
}

// Replaces the current InhibitRules with a new set.
func (i *Inhibitor) SetInhibitRules(r []*config.InhibitRule) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.inhibitRules = i.inhibitRules[:0]

	for _, ih := range r {
		ihr := &InhibitRule{
			MatchOn: ih.MatchOn,
		}
		for _, f := range ih.SourceFilters {
			ihr.SourceFilters = append(ihr.SourceFilters, NewFilter(f.Name, f.Regex))
		}
		for _, f := range ih.TargetFilters {
			ihr.TargetFilters = append(ihr.TargetFilters, NewFilter(f.Name, f.Regex))
		}

		i.inhibitRules = append(i.inhibitRules, ihr)
	}
	i.dirty = true
}

// Returns those []model.LabelSet which are not inhibited by any other
// AlertLabelSet in the provided list.
func (i *Inhibitor) Filter(l []model.LabelSet) []model.LabelSet {
	i.mu.Lock()
	defer i.mu.Unlock()

	out := l
	for _, r := range i.inhibitRules {
		out = r.Filter(l, out)
	}
	return out
}

// Returns whether a given AlertLabelSet is inhibited by a group of other
// []model.LabelSet.
func (i *Inhibitor) IsInhibited(t model.LabelSet, l []model.LabelSet) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, r := range i.inhibitRules {
		if len(r.Filter(l, []model.LabelSet{t})) != 1 {
			return true
		}
	}
	return false
}

// Returns whether inhibits have changed since the last call to HasChanged.
func (i *Inhibitor) HasChanged() bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	dirty := i.dirty
	i.dirty = false
	return dirty
}
