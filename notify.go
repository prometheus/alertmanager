package main

import (
	"fmt"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

type notifyKey int

const (
	notifyName notifyKey = iota
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

// routedNotifier dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type routedNotifier struct {
	mtx       sync.RWMutex
	notifiers map[string]Notifier
}

func (n *routedNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := ctx.Value(notifyName).(string)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	n.mtx.RLock()
	defer n.mtx.RUnlock()

	notifier, ok := n.notifiers[name]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", name)
	}

	return notifier.Notify(ctx, alerts...)
}

func (n *routedNotifier) ApplyConfig(conf *config.Config) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.notifiers = map[string]Notifier{}
	for _, cn := range conf.NotificationConfigs {
		// TODO(fabxc): create proper notifiers.
		n.notifiers[cn.Name] = &LogNotifier{name: cn.Name}
	}
}

// mutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type mutingNotifier struct {
	Notifier

	silencer types.Silencer
}

func (n *mutingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
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
	return r.TargetMatchers.Match(target) && r.SourceMatchers.Match(source)
}
