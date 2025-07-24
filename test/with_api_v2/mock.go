// Copyright 2018 Prometheus Team
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

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/notify/webhook"
)

// At is a convenience method to allow for declarative syntax of Acceptance
// test definitions.
func At(ts float64) float64 {
	return ts
}

type Interval struct {
	start, end float64
}

func (iv Interval) String() string {
	return fmt.Sprintf("[%v,%v]", iv.start, iv.end)
}

func (iv Interval) contains(f float64) bool {
	return f >= iv.start && f <= iv.end
}

// Between is a convenience constructor for an interval for declarative syntax
// of Acceptance test definitions.
func Between(start, end float64) Interval {
	return Interval{start: start, end: end}
}

// TestSilence models a model.Silence with relative times.
type TestSilence struct {
	id               string
	match            []string
	matchRE          []string
	startsAt, endsAt float64

	mtx sync.RWMutex
}

// Silence creates a new TestSilence active for the relative interval given
// by start and end.
func Silence(start, end float64) *TestSilence {
	return &TestSilence{
		startsAt: start,
		endsAt:   end,
	}
}

// Match adds a new plain matcher to the silence.
func (s *TestSilence) Match(v ...string) *TestSilence {
	s.match = append(s.match, v...)
	return s
}

// MatchRE adds a new regex matcher to the silence.
func (s *TestSilence) MatchRE(v ...string) *TestSilence {
	if len(v)%2 == 1 {
		panic("bad key/values")
	}
	s.matchRE = append(s.matchRE, v...)
	return s
}

// SetID sets the silence ID.
func (s *TestSilence) SetID(ID string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.id = ID
}

// ID gets the silence ID.
func (s *TestSilence) ID() string {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.id
}

// nativeSilence converts the declared test silence into a regular
// silence with resolved times.
func (s *TestSilence) nativeSilence(opts *AcceptanceOpts) *models.Silence {
	nsil := &models.Silence{}

	t := false
	for i := 0; i < len(s.match); i += 2 {
		nsil.Matchers = append(nsil.Matchers, &models.Matcher{
			Name:    &s.match[i],
			Value:   &s.match[i+1],
			IsRegex: &t,
		})
	}
	t = true
	for i := 0; i < len(s.matchRE); i += 2 {
		nsil.Matchers = append(nsil.Matchers, &models.Matcher{
			Name:    &s.matchRE[i],
			Value:   &s.matchRE[i+1],
			IsRegex: &t,
		})
	}

	if s.startsAt > 0 {
		start := strfmt.DateTime(opts.expandTime(s.startsAt))
		nsil.StartsAt = &start
	}
	if s.endsAt > 0 {
		end := strfmt.DateTime(opts.expandTime(s.endsAt))
		nsil.EndsAt = &end
	}
	comment := "some comment"
	createdBy := "admin@example.com"
	nsil.Comment = &comment
	nsil.CreatedBy = &createdBy

	return nsil
}

// TestAlert models a model.Alert with relative times.
type TestAlert struct {
	labels           models.LabelSet
	annotations      models.LabelSet
	startsAt, endsAt float64
}

// Alert creates a new alert declaration with the given key/value pairs
// as identifying labels.
func Alert(keyval ...interface{}) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}
	a := &TestAlert{
		labels:      models.LabelSet{},
		annotations: models.LabelSet{},
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := keyval[i].(string)
		lv := keyval[i+1].(string)

		a.labels[ln] = lv
	}

	return a
}

// nativeAlert converts the declared test alert into a full alert based
// on the given parameters.
func (a *TestAlert) nativeAlert(opts *AcceptanceOpts) *models.GettableAlert {
	na := &models.GettableAlert{
		Alert: models.Alert{
			Labels: a.labels,
		},
		Annotations: a.annotations,
		StartsAt:    &strfmt.DateTime{},
		EndsAt:      &strfmt.DateTime{},
	}

	if a.startsAt > 0 {
		start := strfmt.DateTime(opts.expandTime(a.startsAt))
		na.StartsAt = &start
	}
	if a.endsAt > 0 {
		end := strfmt.DateTime(opts.expandTime(a.endsAt))
		na.EndsAt = &end
	}

	return na
}

// Annotate the alert with the given key/value pairs.
func (a *TestAlert) Annotate(keyval ...interface{}) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := keyval[i].(string)
		lv := keyval[i+1].(string)

		a.annotations[ln] = lv
	}

	return a
}

// Active declares the relative activity time for this alert. It
// must be a single starting value or two values where the second value
// declares the resolved time.
func (a *TestAlert) Active(tss ...float64) *TestAlert {
	if len(tss) > 2 || len(tss) == 0 {
		panic("only one or two timestamps allowed")
	}
	if len(tss) == 2 {
		a.endsAt = tss[1]
	}
	a.startsAt = tss[0]

	return a
}

func equalAlerts(a, b *models.GettableAlert, opts *AcceptanceOpts) bool {
	if !reflect.DeepEqual(a.Labels, b.Labels) {
		return false
	}
	if !reflect.DeepEqual(a.Annotations, b.Annotations) {
		return false
	}

	if !equalTime(time.Time(*a.StartsAt), time.Time(*b.StartsAt), opts) {
		return false
	}
	if (a.EndsAt == nil) != (b.EndsAt == nil) {
		return false
	}
	if (a.EndsAt != nil) && (b.EndsAt != nil) && !equalTime(time.Time(*a.EndsAt), time.Time(*b.EndsAt), opts) {
		return false
	}
	return true
}

func equalTime(a, b time.Time, opts *AcceptanceOpts) bool {
	if a.IsZero() != b.IsZero() {
		return false
	}

	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= opts.Tolerance
}

type MockWebhook struct {
	opts      *AcceptanceOpts
	collector *Collector
	addr      string

	// Func is called early on when retrieving a notification by an
	// Alertmanager. If Func returns true, the given notification is dropped.
	// See sample usage in `send_test.go/TestRetry()`.
	Func func(timestamp float64) bool
}

func NewWebhook(t *testing.T, c *Collector) *MockWebhook {
	t.Helper()

	wh := &MockWebhook{
		collector: c,
		opts:      c.opts,
	}

	server := httptest.NewServer(wh)
	wh.addr = server.Listener.Addr().String()

	t.Cleanup(func() {
		server.Close()
	})

	return wh
}

func (ws *MockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Inject Func if it exists.
	if ws.Func != nil {
		if ws.Func(ws.opts.relativeTime(time.Now())) {
			return
		}
	}

	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()

	var v webhook.Message
	if err := dec.Decode(&v); err != nil {
		panic(err)
	}

	// Transform the webhook message alerts back into model.Alerts.
	var alerts models.GettableAlerts
	for _, a := range v.Alerts {
		var (
			labels      = models.LabelSet{}
			annotations = models.LabelSet{}
		)
		for k, v := range a.Labels {
			labels[k] = v
		}
		for k, v := range a.Annotations {
			annotations[k] = v
		}

		start := strfmt.DateTime(a.StartsAt)
		end := strfmt.DateTime(a.EndsAt)

		alerts = append(alerts, &models.GettableAlert{
			Alert: models.Alert{
				Labels:       labels,
				GeneratorURL: strfmt.URI(a.GeneratorURL),
			},
			Annotations: annotations,
			StartsAt:    &start,
			EndsAt:      &end,
		})
	}

	ws.collector.add(alerts...)
}

func (ws *MockWebhook) Address() string {
	return ws.addr
}
