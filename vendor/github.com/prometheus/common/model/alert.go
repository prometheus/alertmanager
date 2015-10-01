package model

import (
	"fmt"
	"time"
)

// Alert is a generic representation of an alert in the Prometheus eco-system.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels LabelSet `json:"labels"`

	// Extra key/value information which does not define alert identity.
	Annotations LabelSet `json:"annotations"`

	// The known time range for this alert. Both ends are optional.
	StartsAt time.Time `json:"startsAt,omitempty"`
	EndsAt   time.Time `json:"endsAt,omitempty"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[AlertNameLabel])
}

// Fingerprint returns a unique hash for the alert. It is equivalent to
// the fingerprint of the alert's label set.
func (a *Alert) Fingerprint() Fingerprint {
	return a.Labels.Fingerprint()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), a.Fingerprint().String()[:7])
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

// Resolved returns true iff the activity interval ended in the past.
func (a *Alert) Resolved() bool {
	if a.EndsAt.IsZero() {
		return false
	}
	return !a.EndsAt.After(time.Now())
}
