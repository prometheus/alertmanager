// Copyright The Prometheus Authors
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

package jira

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestJiraRetryAfter(t *testing.T) {
	t.Parallel()

	const (
		search      = "POST /rest/api/2/search"
		cloudSearch = "POST /rest/api/3/search/jql"
		create      = "POST /rest/api/2/issue"
		update      = "PUT /rest/api/2/issue/MONITORING-1"
		lookup      = "GET /rest/api/2/issue/MONITORING-1/transitions"
		transition  = "POST /rest/api/2/issue/MONITORING-1/transitions"
	)

	for _, stage := range []struct {
		name     string
		requests []string
		create   bool
		reopen   bool
		cloud    bool
	}{
		{name: "search", requests: []string{search}},
		{name: "cloud search", requests: []string{cloudSearch}, cloud: true},
		{name: "create", requests: []string{search, create}, create: true},
		{name: "update", requests: []string{search, update}},
		{name: "resolve lookup", requests: []string{search, update, lookup}},
		{name: "resolve transition", requests: []string{search, update, lookup, transition}},
		{name: "reopen lookup", requests: []string{search, update, lookup}, reopen: true},
		{name: "reopen transition", requests: []string{search, update, lookup, transition}, reopen: true},
	} {
		t.Run(stage.name, func(t *testing.T) {
			t.Parallel()

			for _, tc := range []struct {
				name          string
				header        string
				statusCode    int
				delay         time.Duration
				retry         bool
				truncatedBody bool
			}{
				{name: "seconds", header: "120", statusCode: http.StatusTooManyRequests, delay: 2 * time.Minute, retry: true},
				{name: "HTTP date", header: "Mon, 02 Jan 2006 15:06:05 GMT", statusCode: http.StatusTooManyRequests, delay: 2 * time.Minute, retry: true},
				{name: "missing", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "invalid", header: "later", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "negative", header: "-1", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "zero", header: "0", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "past date", header: "Mon, 02 Jan 2006 15:03:05 GMT", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "invalid numeric value", header: "999999999999999999999seconds", statusCode: http.StatusTooManyRequests, retry: true},
				{name: "server error", statusCode: http.StatusServiceUnavailable, retry: true},
				{name: "server error with delay", header: "120", statusCode: http.StatusServiceUnavailable, delay: 2 * time.Minute, retry: true},
				{name: "client error", header: "120", statusCode: http.StatusBadRequest},
				{name: "truncated rate limit response", header: "120", statusCode: http.StatusTooManyRequests, truncatedBody: true},
				{name: "truncated successful response", header: "120", statusCode: http.StatusOK, truncatedBody: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					var requests []string
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						request := r.Method + " " + r.URL.Path
						requests = append(requests, request)
						if request == stage.requests[len(stage.requests)-1] {
							w.Header().Set("Date", "Mon, 02 Jan 2006 15:04:05 GMT")
							if tc.header != "" {
								w.Header().Set("Retry-After", tc.header)
							}
							if tc.truncatedBody {
								w.Header().Set("Content-Length", "100")
							}
							w.WriteHeader(tc.statusCode)
							fmt.Fprint(w, "request failed")
							return
						}
						switch request {
						case search:
							if stage.create {
								fmt.Fprint(w, `{"issues": []}`)
								return
							}
							status := "open"
							if stage.reopen {
								status = "done"
							}
							fmt.Fprintf(w, `{"issues": [{"key": "MONITORING-1", "fields": {"status": {"statusCategory": {"key": %q}}}}]}`, status)
						case update:
							fmt.Fprint(w, `{}`)
						case lookup:
							fmt.Fprint(w, `{"transitions": [{"id": "1", "name": "transition"}]}`)
						default:
							http.Error(w, "unexpected request", http.StatusInternalServerError)
						}
					}))
					t.Cleanup(srv.Close)

					u, err := url.Parse(srv.URL + "/rest/api/2")
					require.NoError(t, err)
					cfg := &JiraConfig{
						APIURL:            &amcommoncfg.URL{URL: u},
						APIType:           "datacenter",
						HTTPConfig:        &commoncfg.HTTPClientConfig{},
						Project:           "MONITORING",
						IssueType:         "Incident",
						ResolveTransition: "transition",
						ReopenTransition:  "transition",
					}
					if stage.cloud {
						cfg.APIType = "cloud"
					}
					n, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
					require.NoError(t, err)

					endsAt := time.Now().Add(-time.Minute)
					if stage.create || stage.reopen {
						endsAt = time.Now().Add(time.Hour)
					}
					a := alert.New(model.Alert{StartsAt: time.Now().Add(-time.Hour), EndsAt: endsAt}, time.Time{}, false)
					verdict := n.Notify(notify.WithGroupKey(context.Background(), "1"), a)
					if tc.truncatedBody {
						require.ErrorIs(t, verdict.Err(), io.ErrUnexpectedEOF)
						require.Equal(t, notify.DefaultReason, verdict.Reason())
					} else {
						require.ErrorContains(t, verdict.Err(), "request failed")
						require.Equal(t, notify.GetFailureReasonFromStatusCode(tc.statusCode), verdict.Reason())
					}
					require.Equal(t, tc.retry, verdict.ShouldRetry())
					require.Equal(t, tc.delay, verdict.Delay())
					require.Equal(t, stage.requests, requests)
				})
			}
		})
	}
}
