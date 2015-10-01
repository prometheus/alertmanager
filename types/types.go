package types

import (
	"encoding/json"
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
	UpdatedAt time.Time
	Timeout   bool
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

func (n *Notify) String() string {
	return fmt.Sprintf("<Notify:%q@%s to=%v res=%v deli=%v>", n.Alert, n.Timestamp, n.SendTo, n.Resolved, n.Delivered)
}

func (n *Notify) Fingerprint() model.Fingerprint {
	h := fnv.New64a()
	h.Write([]byte(n.SendTo))

	fp := model.Fingerprint(h.Sum64())

	return fp ^ n.Alert
}
