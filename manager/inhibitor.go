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
	hasChanged   bool
}

func NewInhibitor(r InhibitRules) *Inhibitor {
	return &Inhibitor{
		inhibitRules: r,
	}
}

func (i *Inhibitor) SetInhibitRules(r InhibitRules) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.inhibitRules = r
	i.hasChanged = true
}

func (i *Inhibitor) Filter(l AlertLabelSets) AlertLabelSets {
	out := l
	for _, r := range i.inhibitRules {
		out = r.Filter(l, out)
	}
	return out
}

func (i *Inhibitor) IsInhibited(t AlertLabelSet, l AlertLabelSets) bool {
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

	changed := i.hasChanged
	i.hasChanged = false
	return changed
}
