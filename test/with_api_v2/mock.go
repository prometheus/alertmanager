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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-openapi/strfmt"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/test/testutils"
)

// Re-export common types and functions from testutils.
type (
	Interval  = testutils.Interval
	TestAlert = testutils.TestAlert
)

var (
	At      = testutils.At
	Between = testutils.Between
	Alert   = testutils.Alert
)

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
		start := strfmt.DateTime(opts.ExpandTime(s.startsAt))
		nsil.StartsAt = &start
	}
	if s.endsAt > 0 {
		end := strfmt.DateTime(opts.ExpandTime(s.endsAt))
		nsil.EndsAt = &end
	}
	comment := "some comment"
	createdBy := "admin@example.com"
	nsil.Comment = &comment
	nsil.CreatedBy = &createdBy

	return nsil
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
		opts:      c.Opts(),
	}

	server := httptest.NewServer(wh)
	wh.addr = server.Listener.Addr().String()

	t.Cleanup(func() {
		server.Close()
	})

	return wh
}

func (ws *MockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	testutils.HandleWebhookRequest(w, req, ws.collector, ws.opts, ws.Func)
}

func (ws *MockWebhook) Address() string {
	return ws.addr
}
