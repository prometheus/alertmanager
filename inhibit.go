// Copyright 2015 Prometheus Team
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

package main

import (
	"sync"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

// An Inhibitor determines whether a given label set is muted
// based on the currently active alerts and a set of inhibition rules.
type Inhibitor struct {
	alerts provider.Alerts
	rules  []*InhibitRule
	marker types.Marker

	mtx sync.RWMutex
}

// NewInhibitor returns a new Inhibitor.
func NewInhibitor(ap provider.Alerts, rs []*config.InhibitRule, mk types.Marker) *Inhibitor {
	ih := &Inhibitor{
		alerts: ap,
		marker: mk,
	}
	for _, cr := range rs {
		ih.rules = append(ih.rules, NewInhibitRule(cr))
	}
	return ih
}

// Mutes returns true iff the given label set is muted.
func (ih *Inhibitor) Mutes(lset model.LabelSet) bool {
	alerts := ih.alerts.GetPending()
	defer alerts.Close()

	// TODO(fabxc): improve erroring for iterators so it does not
	// go silenced here.

	for alert := range alerts.Next() {
		if err := alerts.Err(); err != nil {
			log.Errorf("Error iterating alerts: %s", err)
			continue
		}
		if alert.Resolved() {
			continue
		}
		for _, rule := range ih.rules {
			if rule.Mutes(alert.Labels, lset) {
				ih.marker.SetInhibited(lset.Fingerprint(), true)
				return true
			}
		}
	}
	if err := alerts.Err(); err != nil {
		log.Errorf("Error after iterating alerts: %s", err)
	}

	ih.marker.SetInhibited(lset.Fingerprint(), false)

	return false
}

// An InhibitRule specifies that a class of (source) alerts should inhibit
// notifications for another class of (target) alerts if all specified matching
// labels are equal between the two alerts. This may be used to inhibit alerts
// from sending notifications if their meaning is logically a subset of a
// higher-level alert.
type InhibitRule struct {
	// The set of Filters which define the group of source alerts (which inhibit
	// the target alerts).
	SourceMatchers types.Matchers
	// The set of Filters which define the group of target alerts (which are
	// inhibited by the source alerts).
	TargetMatchers types.Matchers
	// A set of label names whose label values need to be identical in source and
	// target alerts in order for the inhibition to take effect.
	Equal map[model.LabelName]struct{}
}

// NewInhibitRule returns a new InihibtRule based on a configuration definition.
func NewInhibitRule(cr *config.InhibitRule) *InhibitRule {
	var (
		sourcem types.Matchers
		targetm types.Matchers
	)

	for ln, lv := range cr.SourceMatch {
		sourcem = append(sourcem, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.SourceMatchRE {
		sourcem = append(sourcem, types.NewRegexMatcher(model.LabelName(ln), lv.Regexp))
	}

	for ln, lv := range cr.TargetMatch {
		targetm = append(targetm, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.TargetMatchRE {
		targetm = append(targetm, types.NewRegexMatcher(model.LabelName(ln), lv.Regexp))
	}

	equal := map[model.LabelName]struct{}{}
	for _, ln := range cr.Equal {
		equal[ln] = struct{}{}
	}

	return &InhibitRule{
		SourceMatchers: sourcem,
		TargetMatchers: targetm,
		Equal:          equal,
	}
}

// Mutes returns true iff the Inhibition rule applies for the given
// source and target label set.
func (r *InhibitRule) Mutes(source, target model.LabelSet) bool {
	if !r.TargetMatchers.Match(target) || !r.SourceMatchers.Match(source) {
		return false
	}
	for ln := range r.Equal {
		if source[ln] != target[ln] {
			return false
		}
	}

	return true
}
