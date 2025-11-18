// Copyright 2019 Prometheus Team
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
	"net"
	"net/http"

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
// This is the CLI-specific version with additional fields.
type TestSilence struct {
	id               string
	createdBy        string
	match            []string
	matchRE          []string
	startsAt, endsAt float64
	comment          string
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

// GetMatches returns the plain matchers for the silence.
func (s TestSilence) GetMatches() []string {
	return s.match
}

// MatchRE adds a new regex matcher to the silence.
func (s *TestSilence) MatchRE(v ...string) *TestSilence {
	if len(v)%2 == 1 {
		panic("bad key/values")
	}
	s.matchRE = append(s.matchRE, v...)
	return s
}

// GetMatchREs returns the regex matchers for the silence.
func (s *TestSilence) GetMatchREs() []string {
	return s.matchRE
}

// Comment sets the comment to the silence.
func (s *TestSilence) Comment(c string) *TestSilence {
	s.comment = c
	return s
}

// SetID sets the silence ID.
func (s *TestSilence) SetID(ID string) {
	s.id = ID
}

// ID gets the silence ID.
func (s *TestSilence) ID() string {
	return s.id
}

// EndsAt gets the silence end time.
func (s *TestSilence) EndsAt() float64 {
	return s.endsAt
}

type MockWebhook struct {
	opts      *AcceptanceOpts
	collector *Collector
	listener  net.Listener

	// Func is called early on when retrieving a notification by an
	// Alertmanager. If Func returns true, the given notification is dropped.
	// See sample usage in `send_test.go/TestRetry()`.
	Func func(timestamp float64) bool
}

func NewWebhook(c *Collector) *MockWebhook {
	l, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		// TODO(fabxc): if shutdown of mock destinations ever becomes a concern
		// we want to shut them down after test completion. Then we might want to
		// log the error properly, too.
		panic(err)
	}
	wh := &MockWebhook{
		listener:  l,
		collector: c,
		opts:      c.Opts(),
	}
	go func() {
		if err := http.Serve(l, wh); err != nil {
			panic(err)
		}
	}()

	return wh
}

func (ws *MockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	testutils.HandleWebhookRequest(w, req, ws.collector, ws.opts, ws.Func)
}

func (ws *MockWebhook) Address() string {
	return ws.listener.Addr().String()
}
