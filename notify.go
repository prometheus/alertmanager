package main

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

type Notifier interface {
	Notify(context.Context, ...*types.Alert) error
}

type LogNotifier struct {
	name string
}

func (ln *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	log.Infof("notify %q", ln.name)

	for _, a := range alerts {
		log.Infof("- %v", a)
	}
	return nil
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

func NewInhibitRule(cr config.InhibitRule) *InhibitRule {
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

// silencedNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type silencedNotifier struct {
	Notifier

	silencer types.Silencer
}

func (n *silencedNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var filtered []*types.Alert
	for _, a := range alerts {
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		if !n.silencer.Mutes(a.Labels) {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
		}
	}

	return n.Notifier.Notify(ctx, filtered...)
}
