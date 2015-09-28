package types

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/prometheus/common/model"
)

type Annotations map[model.LabelName]string

type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which does not define alert identity.
	Annotations Annotations `json:"annotations,omitempty"`

	CreatedAt  time.Time `json:"createdAt,omitempty"`
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`

	// The authoritative timestamp.
	Timestamp time.Time `json:"timestamp"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
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
type Muter interface {
	Mutes(model.LabelSet) bool
}

type MuteFunc func(model.LabelSet) bool

func (f MuteFunc) Mutes(lset model.LabelSet) bool { return f(lset) }

// A Silence determines whether a given label set is muted
// at the current time.
type Silence struct {
	ID model.Fingerprint

	// A set of matchers determining if an alert is affected
	// by the silence.
	Matchers Matchers
	// The activity interval of the silence.
	StartsAt, EndsAt time.Time

	// Additional creation information.
	CreateBy, Comment string

	// timeFunc provides the time against which to evaluate
	// the silence.
	timeFunc func() time.Time
}

func (sil *Silence) Mutes(lset model.LabelSet) bool {
	t := sil.timeFunc()

	if t.Before(sil.StartsAt) || t.After(sil.EndsAt) {
		return false
	}

	return sil.Matchers.Match(lset)
}

func (sil *Silence) UnmarshalJSON(b []byte) error {
	var v = struct {
		ID       model.Fingerprint
		Matchers []struct {
			Name    model.LabelName `json:"name"`
			Value   string          `json:"value"`
			IsRegex bool            `json:"isRegex"`
		} `json:"matchers"`
		StartsAt  time.Time `json:"startsAt"`
		EndsAt    time.Time `json:"endsAt"`
		CreatedBy string    `json:"createdBy"`
		Comment   string    `json:"comment,omitempty"`
	}{}

	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	sil.ID = v.ID
	sil.CreateBy = v.CreatedBy
	sil.Comment = v.Comment
	sil.StartsAt = v.StartsAt
	sil.EndsAt = v.EndsAt

	for _, m := range v.Matchers {
		if !m.IsRegex {
			sil.Matchers = append(sil.Matchers, NewMatcher(m.Name, m.Value))
			continue
		}
		rem, err := NewRegexMatcher(m.Name, m.Value)
		if err != nil {
			return err
		}
		sil.Matchers = append(sil.Matchers, rem)
	}
	return nil
}

func (sil *Silence) MarshalJSON() ([]byte, error) {
	type matcher struct {
		Name    model.LabelName `json:"name"`
		Value   string          `json:"value"`
		IsRegex bool            `json:"isRegex"`
	}
	var v = struct {
		ID        model.Fingerprint
		Matchers  []matcher `json:"matchers"`
		StartsAt  time.Time `json:"startsAt"`
		EndsAt    time.Time `json:"endsAt"`
		CreatedBy string    `json:"createdBy"`
		Comment   string    `json:"comment,omitempty"`
	}{
		ID:        sil.ID,
		StartsAt:  sil.StartsAt,
		EndsAt:    sil.EndsAt,
		CreatedBy: sil.CreateBy,
		Comment:   sil.Comment,
	}

	for _, m := range sil.Matchers {
		v.Matchers = append(v.Matchers, matcher{
			Name:    m.Name,
			Value:   m.Value,
			IsRegex: m.isRegex,
		})
	}
	return json.Marshal(v)
}

type Notify struct {
	Alert     model.Fingerprint
	SendTo    string
	Resolved  bool
	Delivered bool
	Timestamp time.Time
}

func (n *Notify) Fingerprint() model.Fingerprint {
	h := fnv.New64a()
	h.Write([]byte(n.SendTo))

	fp := model.Fingerprint(h.Sum64())

	return fp ^ n.Alert
}
