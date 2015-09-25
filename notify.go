package manager

import (
	"github.com/prometheus/log"
	"golang.org/x/net/context"
)

type Notifier interface {
	Notify(context.Context, *Alert) error
}

type LogNotifier struct {
	name string
}

func (ln *LogNotifier) Notify(ctx context.Context, a *Alert) error {
	log.Infof("notify %q", ln.name)

	for _, a := range alerts {
		log.Infof("  - %v", a)
	}
	return nil
}

// routedNotifier forwards alerts to notifiers matching the alert in
// a routing tree.
type routedNotifier struct {
	notifiers map[string]Notifier
}

func (n *routedNotifier) Notify(alert *Alert) error {

}

// A Silencer determines whether a given label set is muted.
type Silencer interface {
	Mutes(model.LabelSet) bool
}

// A Silence determines whether a given label set is muted
// at the current time.
type Silence struct {
	ID model.Fingerprint

	// A set of matchers determining if an alert is
	Matchers Matchers
	// Name/email of the silence creator.
	CreatedBy string
	// When the silence was first created (Unix timestamp).
	CreatedAt, EndsAt time.Time

	// Additional comment about the silence.
	Comment string

	// timeFunc provides the time against which to evaluate
	// the silence.
	timeFunc func() time.Time
}

func (sil *Silence) Mutes(lset model.LabelSet) bool {
	t := sil.timeFunc()

	if t.Before(sil.CreatedAt) || t.After(sil.EndsAt) {
		return false
	}

	return sil.Matchers.Match(lset)
}

// silencedNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type silencedNotifier struct {
	Notifier

	silencer Silencer
}

func (n *silencedNotifier) Notify(alert *Alert) error {
	// TODO(fabxc): increment total alerts counter.
	// Do not send the alert if the silencer mutes it.
	if n.silencer.Mutes(alert.Labels) {
		// TODO(fabxc): increment muted alerts counter.
		return nil
	}

	return n.Notifier.Send(alert)
}

type Inhibitor interface {
	Inhibits(model.LabelSet) bool
}
