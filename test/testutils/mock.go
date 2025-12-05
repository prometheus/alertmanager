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

package testutils

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
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

// TestAlert models a model.Alert with relative times.
type TestAlert struct {
	Labels           models.LabelSet
	Annotations      models.LabelSet
	StartsAt, EndsAt float64
	Summary          string // CLI-specific field, unused in with_api_v2
}

// Alert creates a new alert declaration with the given key/value pairs
// as identifying labels.
func Alert(keyval ...any) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}
	a := &TestAlert{
		Labels:      models.LabelSet{},
		Annotations: models.LabelSet{},
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := keyval[i].(string)
		lv := keyval[i+1].(string)

		a.Labels[ln] = lv
	}

	return a
}

// NativeAlert converts the declared test alert into a full alert based
// on the given parameters.
func (a *TestAlert) NativeAlert(opts *AcceptanceOpts) *models.GettableAlert {
	na := &models.GettableAlert{
		Alert: models.Alert{
			Labels: a.Labels,
		},
		Annotations: a.Annotations,
		StartsAt:    &strfmt.DateTime{},
		EndsAt:      &strfmt.DateTime{},
	}

	if a.StartsAt > 0 {
		start := strfmt.DateTime(opts.ExpandTime(a.StartsAt))
		na.StartsAt = &start
	}
	if a.EndsAt > 0 {
		end := strfmt.DateTime(opts.ExpandTime(a.EndsAt))
		na.EndsAt = &end
	}

	return na
}

// Annotate the alert with the given key/value pairs.
func (a *TestAlert) Annotate(keyval ...any) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := keyval[i].(string)
		lv := keyval[i+1].(string)

		a.Annotations[ln] = lv
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
		a.EndsAt = tss[1]
	}
	a.StartsAt = tss[0]

	return a
}

// HasLabels returns true if the two label sets are equivalent, otherwise false.
// CLI-specific method, unused in with_api_v2.
func (a *TestAlert) HasLabels(labels models.LabelSet) bool {
	return reflect.DeepEqual(a.Labels, labels)
}

// EqualAlerts compares two alerts for equality, considering the tolerance.
func EqualAlerts(a, b *models.GettableAlert, opts *AcceptanceOpts) bool {
	if !reflect.DeepEqual(a.Labels, b.Labels) {
		return false
	}
	if !reflect.DeepEqual(a.Annotations, b.Annotations) {
		return false
	}

	if !EqualTime(time.Time(*a.StartsAt), time.Time(*b.StartsAt), opts) {
		return false
	}
	if (a.EndsAt == nil) != (b.EndsAt == nil) {
		return false
	}
	if (a.EndsAt != nil) && (b.EndsAt != nil) && !EqualTime(time.Time(*a.EndsAt), time.Time(*b.EndsAt), opts) {
		return false
	}
	return true
}

// EqualTime compares two times for equality within the tolerance.
func EqualTime(a, b time.Time, opts *AcceptanceOpts) bool {
	if a.IsZero() != b.IsZero() {
		return false
	}

	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= opts.Tolerance
}

// MockWebhook provides a mock HTTP webhook receiver for testing.
type MockWebhook struct {
	opts      *AcceptanceOpts
	collector *Collector
	addr      string
	closing   atomic.Bool

	// Func is called early on when retrieving a notification by an
	// Alertmanager. If Func returns true, the given notification is dropped.
	// See sample usage in `send_test.go/TestRetry()`.
	Func func(timestamp float64) bool
}

// NewWebhook creates a new MockWebhook that collects alerts via HTTP.
func NewWebhook(t *testing.T, c *Collector) *MockWebhook {
	t.Helper()

	wh := &MockWebhook{
		collector: c,
		opts:      c.Opts(),
	}

	server := httptest.NewServer(wh)
	wh.addr = server.Listener.Addr().String()

	t.Cleanup(func() {
		wh.closing.Store(true)
		server.Close()
	})

	return wh
}

// ServeHTTP handles incoming webhook requests.
func (ws *MockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Inject drop function if it exists.
	if ws.Func != nil {
		if ws.Func(ws.opts.RelativeTime(time.Now())) {
			return
		}
	}

	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()

	var v webhook.Message
	if err := dec.Decode(&v); err != nil {
		// During shutdown, ignore EOF errors from interrupted connections
		if ws.closing.Load() && (err == io.EOF || err.Error() == "EOF") {
			return
		}
		panic(err)
	}

	// Transform the webhook message alerts back into model.Alerts.
	var alerts models.GettableAlerts
	for _, a := range v.Alerts {
		var (
			labels      = models.LabelSet{}
			annotations = models.LabelSet{}
		)
		maps.Copy(labels, a.Labels)
		maps.Copy(annotations, a.Annotations)

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

	ws.collector.Add(alerts...)
}

// Address returns the address of the mock webhook server.
func (ws *MockWebhook) Address() string {
	return ws.addr
}
