package types

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
)

type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload map[string]string `json:"payload,omitempty"`

	CreatedAt  time.Time `json:"created_at,omitempty"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`

	// The authoritative timestamp.
	Timestamp time.Time `json:"timestamp"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
}

// func (a *Alert) Merge(o *Alert) bool {

// }

// Fingerprint returns a unique hash for the alert. It is equivalent to
// the fingerprint of the alert's label set.
func (a *Alert) Fingerprint() model.Fingerprint {
	return a.Labels.Fingerprint()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), a.Fingerprint())
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

func (a *Alert) Resolved() bool {
	if a.ResolvedAt.IsZero() {
		return false
	}
	return !a.ResolvedAt.After(time.Now())
}

// alertTimeline is a list of alerts sorted by their timestamp.
type alertTimeline []*Alert

func (at alertTimeline) Len() int           { return len(at) }
func (at alertTimeline) Less(i, j int) bool { return at[i].Timestamp.Before(at[j].Timestamp) }
func (at alertTimeline) Swap(i, j int)      { at[i], at[j] = at[j], at[i] }

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
