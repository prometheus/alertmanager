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

	_ "github.com/prometheus/alertmanager/config/generated"
)

type InhibitRules []*InhibitRule

type InhibitRule struct {
	SourceFilters Filters
	TargetFilters Filters
	MatchOn       []string
}

// Returns those target AlertLabelSets which are not inhibited by any of the
// source AlertLabelSets.
func (i *InhibitRule) Filter(s AlertLabelSets, t AlertLabelSets) AlertLabelSets {
	s = i.SourceFilters.Filter(s)
	out := AlertLabelSets{}
	for _, tl := range t {
		inhibited := false
		if i.TargetFilters.Handles(tl) {
			for _, sl := range s {
				if tl.MatchOnLabels(sl, i.MatchOn) {
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

// Inhibitor calculates inhibition rules between its labelset inputs and only
// emits uninhibited alert labelsets.
type Inhibitor struct {
	mu           sync.Mutex
	inhibitRules InhibitRules
	dirty        bool
}

// Replaces the current InhibitRules with a new set.
func (i *Inhibitor) SetInhibitRules(r InhibitRules) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.inhibitRules = r
	i.dirty = true
}

// Returns those AlertLabelSets which are not inhibited by any other
// AlertLabelSet in the provided list.
func (i *Inhibitor) Filter(l AlertLabelSets) AlertLabelSets {
	i.mu.Lock()
	defer i.mu.Unlock()

	out := l
	for _, r := range i.inhibitRules {
		out = r.Filter(l, out)
	}
	return out
}

// Returns whether a given AlertLabelSet is inhibited by a group of other
// AlertLabelSets.
func (i *Inhibitor) IsInhibited(t AlertLabelSet, l AlertLabelSets) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, r := range i.inhibitRules {
		if len(r.Filter(l, AlertLabelSets{t})) != 1 {
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
