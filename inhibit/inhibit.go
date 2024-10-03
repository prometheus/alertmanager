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

package inhibit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

// An Inhibitor determines whether a given label set is muted based on the
// currently active alerts and a set of inhibition rules. It implements the
// Muter interface.
type Inhibitor struct {
	alerts provider.Alerts
	rules  []*InhibitRule
	marker types.AlertMarker
	logger *slog.Logger

	mtx    sync.RWMutex
	cancel func()
}

// NewInhibitor returns a new Inhibitor.
func NewInhibitor(ap provider.Alerts, rs []config.InhibitRule, mk types.AlertMarker, logger *slog.Logger) *Inhibitor {
	ih := &Inhibitor{
		alerts: ap,
		marker: mk,
		logger: logger,
	}
	for _, cr := range rs {
		r := NewInhibitRule(cr)
		ih.rules = append(ih.rules, r)
	}
	return ih
}

func (ih *Inhibitor) run(ctx context.Context) {
	it := ih.alerts.Subscribe()
	defer it.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case a := <-it.Next():
			if err := it.Err(); err != nil {
				ih.logger.Error("Error iterating alerts", "err", err)
				continue
			}
			// Update the inhibition rules' cache.
			for _, r := range ih.rules {
				if r.SourceMatchers.Matches(a.Labels) {
					r.set(a)
				}
			}
		}
	}
}

// Run the Inhibitor's background processing.
func (ih *Inhibitor) Run() {
	var (
		g   run.Group
		ctx context.Context
	)

	ih.mtx.Lock()
	ctx, ih.cancel = context.WithCancel(context.Background())
	ih.mtx.Unlock()
	runCtx, runCancel := context.WithCancel(ctx)

	for _, r := range ih.rules {
		go func(r *InhibitRule) {
			ticker := time.NewTicker(15 * time.Minute)
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				r.mtx.Lock()
				for icacheKey, cacheEntry := range r.icache {
					for fp, cachedAlert := range cacheEntry {
						if cachedAlert.alert.Resolved() {
							delete(cacheEntry, fp)
						}
					}
					if len(cacheEntry) == 0 {
						delete(r.icache, icacheKey)
					}
				}
				r.mtx.Unlock()
			}
		}(r)
	}

	g.Add(func() error {
		ih.run(runCtx)
		return nil
	}, func(err error) {
		runCancel()
	})

	if err := g.Run(); err != nil {
		ih.logger.Warn("error running inhibitor", "err", err)
	}
}

// Stop the Inhibitor's background processing.
func (ih *Inhibitor) Stop() {
	if ih == nil {
		return
	}

	ih.mtx.RLock()
	defer ih.mtx.RUnlock()
	if ih.cancel != nil {
		ih.cancel()
	}
}

// Mutes returns true iff the given label set is muted. It implements the Muter
// interface.
func (ih *Inhibitor) Mutes(lset model.LabelSet) bool {
	fp := lset.Fingerprint()
	now := time.Now()

	for _, r := range ih.rules {
		if !r.TargetMatchers.Matches(lset) {
			// If target side of rule doesn't match, we don't need to look any further.
			continue
		}
		// we know that the target side matches, but we don't know if this alert
		// is actually inhibited yet - let the InhibitRule figure that out.
		if inhibiting, matches := r.findInhibitor(lset, now); matches {
			ih.marker.SetInhibited(fp, inhibiting.String())
			return true
		}
	}

	ih.marker.SetInhibited(fp)

	return false
}

type cachedAlert struct {
	alert                  *types.Alert
	matchesSourceAndTarget bool
}

func newCachedAlert(a *types.Alert, targetMatchers labels.Matchers) cachedAlert {
	return cachedAlert{
		alert:                  a,
		matchesSourceAndTarget: targetMatchers.Matches(a.Labels),
	}
}

type iCacheEntry map[model.Fingerprint]cachedAlert

// An InhibitRule specifies that a class of (source) alerts should inhibit
// notifications for another class of (target) alerts if all specified matching
// labels are equal between the two alerts. This may be used to inhibit alerts
// from sending notifications if their meaning is logically a subset of a
// higher-level alert.
type InhibitRule struct {
	// The set of Filters which define the group of source alerts (which inhibit
	// the target alerts).
	SourceMatchers labels.Matchers
	// The set of Filters which define the group of target alerts (which are
	// inhibited by the source alerts).
	TargetMatchers labels.Matchers
	// A set of label names whose label values need to be identical in source and
	// target alerts in order for the inhibition to take effect.
	Equal map[model.LabelName]struct{}

	// Cache of alerts matching source labels.
	icache map[model.Fingerprint]iCacheEntry

	mtx *sync.RWMutex
}

// NewInhibitRule returns a new InhibitRule based on a configuration definition.
func NewInhibitRule(cr config.InhibitRule) *InhibitRule {
	var (
		sourcem labels.Matchers
		targetm labels.Matchers
	)
	// cr.SourceMatch will be deprecated. This for loop appends regex matchers.
	for ln, lv := range cr.SourceMatch {
		matcher, err := labels.NewMatcher(labels.MatchEqual, ln, lv)
		if err != nil {
			// This error must not happen because the config already validates the yaml.
			panic(err)
		}
		sourcem = append(sourcem, matcher)
	}
	// cr.SourceMatchRE will be deprecated. This for loop appends regex matchers.
	for ln, lv := range cr.SourceMatchRE {
		matcher, err := labels.NewMatcher(labels.MatchRegexp, ln, lv.String())
		if err != nil {
			// This error must not happen because the config already validates the yaml.
			panic(err)
		}
		sourcem = append(sourcem, matcher)
	}
	// We append the new-style matchers. This can be simplified once the deprecated matcher syntax is removed.
	sourcem = append(sourcem, cr.SourceMatchers...)

	// cr.TargetMatch will be deprecated. This for loop appends regex matchers.
	for ln, lv := range cr.TargetMatch {
		matcher, err := labels.NewMatcher(labels.MatchEqual, ln, lv)
		if err != nil {
			// This error must not happen because the config already validates the yaml.
			panic(err)
		}
		targetm = append(targetm, matcher)
	}
	// cr.TargetMatchRE will be deprecated. This for loop appends regex matchers.
	for ln, lv := range cr.TargetMatchRE {
		matcher, err := labels.NewMatcher(labels.MatchRegexp, ln, lv.String())
		if err != nil {
			// This error must not happen because the config already validates the yaml.
			panic(err)
		}
		targetm = append(targetm, matcher)
	}
	// We append the new-style matchers. This can be simplified once the deprecated matcher syntax is removed.
	targetm = append(targetm, cr.TargetMatchers...)

	equal := map[model.LabelName]struct{}{}
	for _, ln := range cr.Equal {
		equal[model.LabelName(ln)] = struct{}{}
	}

	return &InhibitRule{
		SourceMatchers: sourcem,
		TargetMatchers: targetm,
		Equal:          equal,
		icache:         make(map[model.Fingerprint]iCacheEntry),
		mtx:            &sync.RWMutex{},
	}
}

func (r *InhibitRule) set(a *types.Alert) {
	// these two operations are by far the most expensive part of the method
	// since they don't require hilding the mutex, call them here as a tiny
	// optimization
	icacheKey := r.icacheKey(a.Labels)
	fp := a.Fingerprint()

	r.mtx.Lock()
	defer r.mtx.Unlock()

	cacheEntry, ok := r.icache[icacheKey]
	if !ok {
		cacheEntry = make(iCacheEntry)
		r.icache[icacheKey] = cacheEntry
	}

	cacheEntry[fp] = newCachedAlert(a, r.TargetMatchers)
}

func (r *InhibitRule) icacheKey(lset model.LabelSet) model.Fingerprint {
	equalLabels := model.LabelSet{}
	for label := range r.Equal {
		equalLabels[label] = lset[label]
	}
	return equalLabels.Fingerprint()
}

// findInhibitor determines if any alert inhibits an lset that matches the target
// matchers. The fingerprint of the first matching result is returned.
func (r *InhibitRule) findInhibitor(lset model.LabelSet, now time.Time) (model.Fingerprint, bool) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	var sourceMatchersEvaluated, lsetMatchesSource bool
	if cacheEntry, ok := r.icache[r.icacheKey(lset)]; ok {
		for fp, cachedAlert := range cacheEntry {
			if cachedAlert.alert.ResolvedAt(now) {
				continue
			}

			if cachedAlert.matchesSourceAndTarget {
				if !sourceMatchersEvaluated {
					lsetMatchesSource = r.SourceMatchers.Matches(lset)
					sourceMatchersEvaluated = true
				}
				if lsetMatchesSource {
					continue
				}
			}

			return fp, true
		}

	}

	return model.Fingerprint(0), false
}
