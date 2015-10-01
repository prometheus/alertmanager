package types

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
)

// Reloadable is a component that can change its state based
// on a new configuration.
type Reloadable interface {
	ApplyConfig(*config.Config) bool
}

// Alert wraps a model.Alert with additional information relevant
// to internal of the Alertmanager.
// The type is never exposed to external communication and the
// embedded alert has to be sanitized beforehand.
type Alert struct {
	model.Alert

	// The authoritative timestamp.
	UpdatedAt time.Time `json:"-"`
	Timeout   bool      `json:"-"`
}

// Alerts turns a sequence of internal alerts into a list of
// exposable model.Alert structures.
func Alerts(alerts ...*Alert) model.Alerts {
	var res model.Alerts
	for _, a := range alerts {
		v := a.Alert
		// If the end timestamp was set as the expected value in case
		// of a timeout, do not expose it.
		if a.Timeout {
			v.EndsAt = time.Time{}
		}
		res = append(res, &v)
	}
	return res
}

// Merges the timespan of two alerts based and overwrites annotations
// based on the authoritative timestamp.
// A new alert is returned, the labels are assumed to be equal.
// func (a *Alert) Merge(o *Alert) *Alert {
// 	// Let o always be the younger alert.
// 	if a.Timestamp.Before(a.Timestamp) {
// 		return o.Merge(a)
// 	}

// 	res := &Alert{
// 		Labels:       o.Labels,
// 		Annotiations: o.Annotations,
// 		Timestamp:    o.Timestamp,
// 	}

// }

// A Silencer determines whether a given label set is muted.
type Muter interface {
	Mutes(model.LabelSet) bool
}

type MuteFunc func(model.LabelSet) bool

func (f MuteFunc) Mutes(lset model.LabelSet) bool { return f(lset) }

// A Silence determines whether a given label set is muted
// at the current time.
type Silence struct {
	model.Silence

	// A set of matchers determining if an alert is affected
	// by the silence.
	Matchers Matchers `json:"-"`

	// timeFunc provides the time against which to evaluate
	// the silence.
	timeFunc func() time.Time
}

// NewSilence creates a new internal Silence from a public silence
// object.
func NewSilence(s *model.Silence) *Silence {
	sil := &Silence{
		Silence:  *s,
		timeFunc: time.Now,
	}
	for _, m := range s.Matchers {
		if !m.IsRegex {
			sil.Matchers = append(sil.Matchers, NewMatcher(m.Name, m.Value))
			continue
		}
		rem, err := NewRegexMatcher(m.Name, m.Value)
		if err != nil {
			// Must have been sanitized beforehand.
			panic(err)
		}
		sil.Matchers = append(sil.Matchers, rem)
	}
	return sil
}

func (sil *Silence) Mutes(lset model.LabelSet) bool {
	t := sil.timeFunc()

	if t.Before(sil.StartsAt) || t.After(sil.EndsAt) {
		return false
	}

	b := sil.Matchers.Match(lset)

	return b
}

// Notify holds information about the last notification state
// of an Alert.
type Notify struct {
	Alert     model.Fingerprint
	SendTo    string
	Resolved  bool
	Delivered bool
	Timestamp time.Time
}

func (n *Notify) String() string {
	return fmt.Sprintf("<Notify:%q@%s to=%v res=%v deli=%v>", n.Alert, n.Timestamp, n.SendTo, n.Resolved, n.Delivered)
}

func (n *Notify) Fingerprint() model.Fingerprint {
	h := fnv.New64a()
	h.Write([]byte(n.SendTo))

	fp := model.Fingerprint(h.Sum64())

	return fp ^ n.Alert
}
