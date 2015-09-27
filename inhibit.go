package main

import (
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

type Inhibitor struct {
	alerts provider.Alerts
	rules  []*InhibitRule

	mtx sync.RWMutex
}

func (ih *Inhibitor) Mutes(lset model.LabelSet) bool {
	ih.mtx.RLock()
	defer ih.mtx.RUnlock()

	alerts, err := ih.alerts.All()
	if err != nil {
		// TODO(fabxc): log error.
		return false
	}

	for _, alert := range alerts {
		if alert.Resolved() {
			continue
		}
		for _, rule := range ih.rules {
			if rule.Mutes(alert.Labels, lset) {
				return true
			}
		}
	}
	return false
}

func (ih *Inhibitor) ApplyConfig(conf *config.Config) {
	ih.mtx.Lock()
	defer ih.mtx.Unlock()

	ih.rules = []*InhibitRule{}
	for _, cr := range conf.InhibitRules {
		ih.rules = append(ih.rules, NewInhibitRule(cr))
	}
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

func NewInhibitRule(cr *config.InhibitRule) *InhibitRule {
	var (
		sourcem types.Matchers
		targetm types.Matchers
		equal   map[model.LabelName]struct{}
	)

	for ln, lv := range cr.SourceMatch {
		sourcem = append(sourcem, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.SourceMatchRE {
		m, err := types.NewRegexMatcher(model.LabelName(ln), lv.String())
		if err != nil {
			// Must have been sanitized during config validation.
			panic(err)
		}
		sourcem = append(sourcem, m)
	}

	for ln, lv := range cr.TargetMatch {
		targetm = append(targetm, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.TargetMatchRE {
		m, err := types.NewRegexMatcher(model.LabelName(ln), lv.String())
		if err != nil {
			// Must have been sanitized during config validation.
			panic(err)
		}
		targetm = append(targetm, m)
	}

	for _, ln := range cr.Equal {
		equal[ln] = struct{}{}
	}

	return &InhibitRule{
		SourceMatchers: sourcem,
		TargetMatchers: targetm,
		Equal:          equal,
	}
}

func (r *InhibitRule) Mutes(source, target model.LabelSet) bool {
	if r.TargetMatchers.Match(target) && r.SourceMatchers.Match(source) {
		for ln := range r.Equal {
			if source[ln] != target[ln] {
				return false
			}
		}
	} else {
		return false
	}

	return true
}
