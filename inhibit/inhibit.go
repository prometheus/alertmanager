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
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/tracing"
)

var tracer = tracing.NewTracer("github.com/prometheus/alertmanager/inhibit")

// An Inhibitor determines whether a given label set is muted based on the
// currently active alerts and a set of inhibition rules. It implements the
// Muter interface.
type Inhibitor struct {
	alerts     provider.Alerts
	rules      []*InhibitRule
	logger     *slog.Logger
	propagator propagation.TextMapPropagator
	recorder   eventrecorder.Recorder

	mtx             sync.RWMutex
	loadingFinished sync.WaitGroup
	cancel          func()
}

// NewInhibitor returns a new Inhibitor.
func NewInhibitor(ap provider.Alerts, rs []amcommoncfg.InhibitRule, logger *slog.Logger, recorder eventrecorder.Recorder) *Inhibitor {
	ih := &Inhibitor{
		alerts:     ap,
		logger:     logger,
		propagator: otel.GetTextMapPropagator(),
		recorder:   recorder,
	}

	ih.loadingFinished.Add(1)
	ruleNames := make(map[string]struct{})
	for i, cr := range rs {
		if _, ok := ruleNames[cr.Name]; ok {
			ih.logger.Debug("duplicate inhibition rule name", "index", i, "name", cr.Name)
		}

		r := NewInhibitRule(cr)
		ih.rules = append(ih.rules, r)

		if cr.Name != "" {
			ruleNames[cr.Name] = struct{}{}
		}
	}
	return ih
}

func (ih *Inhibitor) run(ctx context.Context) {
	initalAlerts, it := ih.alerts.SlurpAndSubscribe("inhibitor")
	defer it.Close()

	for _, a := range initalAlerts {
		ih.processAlert(ctx, a)
	}

	ih.loadingFinished.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case a := <-it.Next():
			if err := it.Err(); err != nil {
				ih.logger.Error("Error iterating alerts", "err", err)
				continue
			}
			traceCtx := context.Background()
			if a.Header != nil {
				traceCtx = ih.propagator.Extract(traceCtx, propagation.MapCarrier(a.Header))
			}
			ih.processAlert(traceCtx, a.Data)
		}
	}
}

func (ih *Inhibitor) processAlert(ctx context.Context, a *alert.Alert) {
	_, span := tracer.Start(ctx, "inhibit.Inhibitor.processAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// Update the inhibition rules' source caches.
	for _, r := range ih.rules {
		for _, src := range r.Sources {
			if src.SrcMatchers.Matches(a.Labels) {
				attr := attribute.String("alerting.inhibit_rule.name", r.Name)
				span.AddEvent("alert matched rule source", trace.WithAttributes(attr))
				span.SetAttributes(attr)
				src.cache.set(a)
			}
		}
	}
}

func (ih *Inhibitor) WaitForLoading() {
	ih.loadingFinished.Wait()
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

	for _, rule := range ih.rules {
		for _, src := range rule.Sources {
			go src.cache.run(runCtx, 15*time.Minute)
		}
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

// Mutes returns true iff the given label set is muted.  It implements the
// Muter interface.
func (ih *Inhibitor) Mutes(ctx context.Context, lset model.LabelSet) bool {
	fp := lset.Fingerprint()

	_, span := tracer.Start(ctx, "inhibit.Inhibitor.Mutes",
		trace.WithAttributes(attribute.String("alerting.alert.fingerprint", fp.String())),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	var inhibitedBy []string
	defer func() {
		// Get the marker from context and set the inhibited alerts on it if any.
		m, ok := marker.FromContext(ctx)
		if ok {
			m.SetInhibited(fp, inhibitedBy)
		}
	}()

	now := time.Now()
	for _, r := range ih.rules {
		if !r.TargetMatchers.Matches(lset) {
			// If target side of rule doesn't match, we don't need to look any further.
			continue
		}
		span.AddEvent("alert matched rule target",
			trace.WithAttributes(
				attribute.String("alerting.inhibit_rule.name", r.Name),
			),
		)
		// If we are here, the target side matches. Compute the two-sided exclusion
		// flag once: does this target alert match ANY source's matchers?
		excludeTwoSidedMatch := false
		for _, src := range r.Sources {
			if src.SrcMatchers.Matches(lset) {
				excludeTwoSidedMatch = true
				break
			}
		}
		// Check all sources — all must have a matching equal alert for
		// the inhibition to take effect.
		var inhibitorFPs []model.Fingerprint
		allSourcesMatch := true
		for _, src := range r.Sources {
			if inhibitedByFP, eq := src.hasEqual(lset, excludeTwoSidedMatch, now, r.TargetMatchers); eq {
				inhibitorFPs = append(inhibitorFPs, inhibitedByFP)
			} else {
				allSourcesMatch = false
				break
			}
		}
		if allSourcesMatch {
			seen := make(map[model.Fingerprint]struct{}, len(inhibitorFPs))
			for _, ifp := range inhibitorFPs {
				if _, ok := seen[ifp]; ok {
					continue
				}
				seen[ifp] = struct{}{}
				inhibitedBy = append(inhibitedBy, ifp.String())
			}
			span.AddEvent("alert inhibited",
				trace.WithAttributes(
					attribute.StringSlice("alerting.inhibit_rule.inhibitors", inhibitedBy),
				),
			)

			ih.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
				var rules []eventrecorder.InhibitRule
				for _, src := range r.Sources {
					rules = append(rules, eventrecorder.NewInhibitRule(r.Name, src.SrcMatchers, r.TargetMatchers, src.Equal))
				}
				return eventrecorder.NewInhibitionMutedAlertEvent(
					rules,
					fp, lset,
					inhibitorFPs,
				)
			})
			return true
		}
	}
	span.AddEvent("alert not inhibited")

	return false
}

// Source represents a single source definition within an inhibition rule,
// including its own matchers, equal labels, and cache.
type Source struct {
	SrcMatchers labels.Matchers
	Equal       map[model.LabelName]struct{}
	cache       *cache
}

// An InhibitRule specifies that a class of (source) alerts should inhibit
// notifications for another class of (target) alerts if all specified matching
// labels are equal between the two alerts. This may be used to inhibit alerts
// from sending notifications if their meaning is logically a subset of a
// higher-level alert.
type InhibitRule struct {
	// Name is an optional name for the inhibition rule.
	Name string
	// Sources define groups of source alerts (which inhibit the target alerts).
	// All sources must match for the inhibition to take effect.
	Sources []Source
	// The set of Filters which define the group of target alerts (which are
	// inhibited by the source alerts).
	TargetMatchers labels.Matchers
}

// NewInhibitRule returns a new InhibitRule based on a configuration definition.
func NewInhibitRule(cr amcommoncfg.InhibitRule) *InhibitRule {
	var (
		sources []Source
		targetm labels.Matchers
	)

	if len(cr.Sources) > 0 {
		for _, sm := range cr.Sources {
			var sourcesm labels.Matchers
			sourcesm = append(sourcesm, sm.SrcMatchers...)
			equal := map[model.LabelName]struct{}{}
			for _, ln := range sm.Equal {
				equal[model.LabelName(ln)] = struct{}{}
			}
			sources = append(sources, Source{
				SrcMatchers: sourcesm,
				Equal:       equal,
				cache:       newCache(equal),
			})
		}
	} else {
		var sourcem labels.Matchers
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

		equal := map[model.LabelName]struct{}{}
		for _, ln := range cr.Equal {
			equal[model.LabelName(ln)] = struct{}{}
		}

		sources = append(sources, Source{
			SrcMatchers: sourcem,
			Equal:       equal,
			cache:       newCache(equal),
		})
	}

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

	return &InhibitRule{
		Name:           cr.Name,
		Sources:        sources,
		TargetMatchers: targetm,
	}
}

// hasEqual checks whether this source's cache contains an active alert whose
// equal labels match the given label set. If so, the fingerprint of that alert
// is returned. If excludeTwoSidedMatch is true, cached alerts that also match
// the target matchers are skipped to prevent self-inhibition.
func (s *Source) hasEqual(lset model.LabelSet, excludeTwoSidedMatch bool, now time.Time, targetMatchers labels.Matchers) (model.Fingerprint, bool) {
	return s.cache.find(lset, now, func(a *alert.Alert) bool {
		return !excludeTwoSidedMatch || !targetMatchers.Matches(a.Labels)
	})
}
