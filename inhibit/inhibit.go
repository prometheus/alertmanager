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
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

var tracer = otel.Tracer("github.com/prometheus/alertmanager/inhibit")

// An Inhibitor determines whether a given label set is muted based on the
// currently active alerts and a set of inhibition rules. It implements the
// Muter interface.
type Inhibitor struct {
	alerts     provider.Alerts
	rules      []*InhibitRule
	marker     types.AlertMarker
	logger     *slog.Logger
	propagator propagation.TextMapPropagator

	mtx             sync.RWMutex
	loadingFinished sync.WaitGroup
	cancel          func()
}

// NewInhibitor returns a new Inhibitor.
func NewInhibitor(ap provider.Alerts, rs []config.InhibitRule, mk types.AlertMarker, logger *slog.Logger) *Inhibitor {
	ih := &Inhibitor{
		alerts:     ap,
		marker:     mk,
		logger:     logger,
		propagator: otel.GetTextMapPropagator(),
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

func (ih *Inhibitor) processAlert(ctx context.Context, a *types.Alert) {
	_, span := tracer.Start(ctx, "inhibit.Inhibitor.processAlert",
		trace.WithAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// Update the inhibition rules' cache.
	for _, r := range ih.rules {
		if r.SourceMatchers.Matches(a.Labels) {
			attr := attribute.String("alerting.inhibit_rule.name", r.Name)
			span.AddEvent("alert matched rule source", trace.WithAttributes(attr))
			if err := r.scache.Set(a); err != nil {
				message := "error on set alert"
				ih.logger.Error(message, "err", err)
				span.SetStatus(codes.Error, message)
				span.RecordError(err)
				continue
			}
			span.SetAttributes(attr)
			r.updateIndex(a)
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
		go rule.scache.Run(runCtx, 15*time.Minute)
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
func (ih *Inhibitor) Mutes(ctx context.Context, lset model.LabelSet) bool {
	fp := lset.Fingerprint()

	_, span := tracer.Start(ctx, "inhibit.Inhibitor.Mutes",
		trace.WithAttributes(attribute.String("alerting.alert.fingerprint", fp.String())),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

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
		// If we are here, the target side matches. If the source side matches, too, we
		// need to exclude inhibiting alerts for which the same is true.
		if inhibitedByFP, eq := r.hasEqual(lset, r.SourceMatchers.Matches(lset), now); eq {
			ih.marker.SetInhibited(fp, inhibitedByFP.String())
			span.AddEvent("alert inhibited",
				trace.WithAttributes(
					attribute.String("alerting.inhibit_rule.source.fingerprint", inhibitedByFP.String()),
				),
			)
			return true
		}
	}
	ih.marker.SetInhibited(fp)
	span.AddEvent("alert not inhibited")

	return false
}

// An InhibitRule specifies that a class of (source) alerts should inhibit
// notifications for another class of (target) alerts if all specified matching
// labels are equal between the two alerts. This may be used to inhibit alerts
// from sending notifications if their meaning is logically a subset of a
// higher-level alert.
type InhibitRule struct {
	// Name is an optional name for the inhibition rule.
	Name string
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
	scache *store.Alerts

	// Index of fingerprints of source alert equal labels to fingerprint of source alert.
	// The index helps speed up source alert lookups from scache significantely in scenarios with 100s of source alerts cached.
	// The index items might overwrite eachother if multiple source alerts have exact equal labels.
	// Overwrites only happen if the new source alert has bigger EndsAt value.
	sindex *index
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

	rule := &InhibitRule{
		Name:           cr.Name,
		SourceMatchers: sourcem,
		TargetMatchers: targetm,
		Equal:          equal,
		scache:         store.NewAlerts(),
		sindex:         newIndex(),
	}

	rule.scache.SetGCCallback(rule.gcCallback)

	return rule
}

// fingerprintEquals returns the fingerprint of the equal labels of the given label set.
func (r *InhibitRule) fingerprintEquals(lset model.LabelSet) model.Fingerprint {
	equalSet := model.LabelSet{}
	for n := range r.Equal {
		equalSet[n] = lset[n]
	}
	return equalSet.Fingerprint()
}

// updateIndex updates the source alert index if necessary.
func (r *InhibitRule) updateIndex(alert *types.Alert) {
	fp := alert.Fingerprint()
	// Calculate source labelset subset which is in equals.
	eq := r.fingerprintEquals(alert.Labels)

	// Check if the equal labelset is already in the index.
	indexed, ok := r.sindex.Get(eq)
	if !ok {
		// If not, add it.
		r.sindex.Set(eq, fp)
		return
	}
	// If the indexed fingerprint is the same as the new fingerprint, do nothing.
	if indexed == fp {
		return
	}

	// New alert and existing index are not the same, compare them.
	existing, err := r.scache.Get(indexed)
	if err != nil {
		// failed to get the existing alert, overwrite the index.
		r.sindex.Set(eq, fp)
		return
	}

	// If the new alert resolves after the existing alert, replace the index.
	if existing.ResolvedAt(alert.EndsAt) {
		r.sindex.Set(eq, fp)
		return
	}
	// If the existing alert resolves after the new alert, do nothing.
}

// findEqualSourceAlert returns the source alert that matches the equal labels of the given label set.
func (r *InhibitRule) findEqualSourceAlert(lset model.LabelSet, now time.Time) (*types.Alert, bool) {
	equalsFP := r.fingerprintEquals(lset)
	sourceFP, ok := r.sindex.Get(equalsFP)
	if ok {
		alert, err := r.scache.Get(sourceFP)
		if err != nil {
			return nil, false
		}

		if alert.ResolvedAt(now) {
			return nil, false
		}

		return alert, true
	}

	return nil, false
}

func (r *InhibitRule) gcCallback(alerts []*types.Alert) {
	for _, a := range alerts {
		fp := r.fingerprintEquals(a.Labels)
		r.sindex.Delete(fp)
	}
}

// hasEqual checks whether the source cache contains alerts matching the equal
// labels for the given label set. If so, the fingerprint of one of those alerts
// is returned. If excludeTwoSidedMatch is true, alerts that match both the
// source and the target side of the rule are disregarded.
func (r *InhibitRule) hasEqual(lset model.LabelSet, excludeTwoSidedMatch bool, now time.Time) (model.Fingerprint, bool) {
	equal, found := r.findEqualSourceAlert(lset, now)
	if found {
		if excludeTwoSidedMatch && r.TargetMatchers.Matches(equal.Labels) {
			return model.Fingerprint(0), false
		}
		return equal.Fingerprint(), found
	}

	return model.Fingerprint(0), false
}
