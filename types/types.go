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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
)

// Marker helps to mark alerts as silenced and/or inhibited.
// All methods are goroutine-safe.
type Marker interface {
	SetInhibited(alert model.Fingerprint, b bool)
	SetSilenced(alert model.Fingerprint, sil ...uint64)

	Silenced(alert model.Fingerprint) (uint64, bool)
	Inhibited(alert model.Fingerprint) bool
}

// NewMarker returns an instance of a Marker implementation.
func NewMarker() Marker {
	return &memMarker{
		inhibited: map[model.Fingerprint]struct{}{},
		silenced:  map[model.Fingerprint]uint64{},
	}
}

type memMarker struct {
	inhibited map[model.Fingerprint]struct{}
	silenced  map[model.Fingerprint]uint64

	mtx sync.RWMutex
}

func (m *memMarker) Inhibited(alert model.Fingerprint) bool {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	_, ok := m.inhibited[alert]
	return ok
}

func (m *memMarker) Silenced(alert model.Fingerprint) (uint64, bool) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	sid, ok := m.silenced[alert]
	return sid, ok
}

func (m *memMarker) SetInhibited(alert model.Fingerprint, b bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if !b {
		delete(m.inhibited, alert)
	} else {
		m.inhibited[alert] = struct{}{}
	}
}

func (m *memMarker) SetSilenced(alert model.Fingerprint, sil ...uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if len(sil) == 0 {
		delete(m.silenced, alert)
	} else {
		m.silenced[alert] = sil[0]
	}
}

// MultiError contains multiple errors and implements the error interface. Its
// zero value is ready to use. All its methods are goroutine safe.
type MultiError struct {
	mtx    sync.Mutex
	errors []error
}

// Add adds an error to the MultiError.
func (e *MultiError) Add(err error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.errors = append(e.errors, err)
}

// Len returns the number of errors added to the MultiError.
func (e *MultiError) Len() int {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	return len(e.errors)
}

// Errors returns the errors added to the MuliError. The returned slice is a
// copy of the internal slice of errors.
func (e *MultiError) Errors() []error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	return append(make([]error, 0, len(e.errors)), e.errors...)
}

func (e *MultiError) Error() string {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	es := make([]string, 0, len(e.errors))
	for _, err := range e.errors {
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
	UpdatedAt    time.Time `json:"-"`
	Timeout      bool      `json:"-"`
	WasSilenced  bool      `json:"-"`
	WasInhibited bool      `json:"-"`
}

// AlertSlice is a sortable slice of Alerts.
type AlertSlice []*Alert

func (as AlertSlice) Less(i, j int) bool { return as[i].UpdatedAt.Before(as[j].UpdatedAt) }
func (as AlertSlice) Swap(i, j int)      { as[i], as[j] = as[j], as[i] }
func (as AlertSlice) Len() int           { return len(as) }

// Alerts turns a sequence of internal alerts into a list of
// exposable model.Alert structures.
func Alerts(alerts ...*Alert) model.Alerts {
	res := make(model.Alerts, 0, len(alerts))
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

// Merge merges the timespan of two alerts based and overwrites annotations
// based on the authoritative timestamp.  A new alert is returned, the labels
// are assumed to be equal.
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

	// A non-timeout resolved timestamp always rules.
	// The latest explicit resolved timestamp wins.
	if a.EndsAt.After(o.EndsAt) && !a.Timeout {
		res.EndsAt = a.EndsAt
	}

	return &res
}

// A Muter determines whether a given label set is muted.
type Muter interface {
	Mutes(model.LabelSet) bool
}

// A MuteFunc is a function that implements the Muter interface.
type MuteFunc func(model.LabelSet) bool

// Mutes implements the Muter interface.
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

// NewSilence creates a new internal Silence from a public silence object.
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
		rem := NewRegexMatcher(m.Name, regexp.MustCompile("^(?:"+m.Value+")$"))
		sil.Matchers = append(sil.Matchers, rem)
	}
	return sil
}

// Mutes implements the Muter interface.
func (sil *Silence) Mutes(lset model.LabelSet) bool {
	t := sil.timeFunc()

	if t.Before(sil.StartsAt) || t.After(sil.EndsAt) {
		return false
	}

	b := sil.Matchers.Match(lset)

	return b
}

// NotifyInfo holds information about the last successful notification
// of an alert to a receiver.
type NotifyInfo struct {
	Alert     model.Fingerprint
	Receiver  string
	Resolved  bool
	Timestamp time.Time
}

func (n *NotifyInfo) String() string {
	return fmt.Sprintf("<Notify:%q@%s to=%v res=%v>", n.Alert, n.Timestamp, n.Receiver, n.Resolved)
}

// Fingerprint returns a quasi-unique fingerprint for the NotifyInfo.
func (n *NotifyInfo) Fingerprint() model.Fingerprint {
	h := fnv.New64a()
	h.Write([]byte(n.Receiver))

	fp := model.Fingerprint(h.Sum64())

	return fp ^ n.Alert
}
