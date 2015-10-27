// Copyright 2015 Prometheus Team
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

package types

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

type MultiError []error

func (e MultiError) Error() string {
	var es []string
	for _, err := range e {
		es = append(es, err.Error())
	}
	return strings.Join(es, "; ")
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

type AlertSlice []*Alert

func (as AlertSlice) Less(i, j int) bool { return as[i].UpdatedAt.Before(as[j].UpdatedAt) }
func (as AlertSlice) Swap(i, j int)      { as[i], as[j] = as[j], as[i] }
func (as AlertSlice) Len() int           { return len(as) }

// Alerts turns a sequence of internal alerts into a list of
// exposable model.Alert structures.
func Alerts(alerts ...*Alert) model.Alerts {
	var res model.Alerts
	for _, a := range alerts {
		v := a.Alert
		// If the end timestamp was set as the expected value in case
		// of a timeout but is not reached yet, do not expose it.
		if a.Timeout && !a.Resolved() {
			v.EndsAt = time.Time{}
		}
		res = append(res, &v)
	}
	return res
}

// Merges the timespan of two alerts based and overwrites annotations
// based on the authoritative timestamp.
// A new alert is returned, the labels are assumed to be equal.
func (a *Alert) Merge(o *Alert) *Alert {
	// Let o always be the younger alert.
	if o.UpdatedAt.Before(a.UpdatedAt) {
		return o.Merge(a)
	}

	res := *o

	// Always pick the earliest starting time.
	if a.StartsAt.Before(o.StartsAt) {
		res.StartsAt = a.StartsAt
	}

	// An non-timeout resolved timestamp always rules.
	// The latest explicit resolved timestamp wins.
	if a.EndsAt.After(o.EndsAt) && !a.Timeout {
		res.EndsAt = a.EndsAt
	}

	return &res
}

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
