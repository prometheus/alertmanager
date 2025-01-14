// Copyright 2023 Prometheus Team
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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

func TestJiraRetry(t *testing.T) {
	notifier, err := New(
		&config.JiraConfig{
			APIURL: &config.URL{
				URL: &url.URL{
					Scheme: "https",
					Host:   "example.atlassian.net",
					Path:   "/rest/api/2",
				},
			},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)

	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("retry - error on status %d", statusCode))
	}
}

func TestJiraTemplating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"total": 0, "issues": []}`))
			return
		default:
			dec := json.NewDecoder(r.Body)
			out := make(map[string]any)
			err := dec.Decode(&out)
			if err != nil {
				panic(err)
			}
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	for _, tc := range []struct {
		title string
		cfg   *config.JiraConfig

		retry  bool
		errMsg string
	}{
		{
			title: "full-blown message",
			cfg: &config.JiraConfig{
				Summary:     `{{ template "jira.default.summary" . }}`,
				Description: `{{ template "jira.default.description" . }}`,
			},
			retry: false,
		},
		{
			title: "template project",
			cfg: &config.JiraConfig{
				Project:     `{{ .CommonLabels.lbl1 }}`,
				Summary:     `{{ template "jira.default.summary" . }}`,
				Description: `{{ template "jira.default.description" . }}`,
			},
			retry: false,
		},
		{
			title: "template issue type",
			cfg: &config.JiraConfig{
				IssueType:   `{{ .CommonLabels.lbl1 }}`,
				Summary:     `{{ template "jira.default.summary" . }}`,
				Description: `{{ template "jira.default.description" . }}`,
			},
			retry: false,
		},
		{
			title: "summary with templating errors",
			cfg: &config.JiraConfig{
				Summary: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "description with templating errors",
			cfg: &config.JiraConfig{
				Summary:     `{{ template "jira.default.summary" . }}`,
				Description: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "priority with templating errors",
			cfg: &config.JiraConfig{
				Summary:     `{{ template "jira.default.summary" . }}`,
				Description: `{{ template "jira.default.description" . }}`,
				Priority:    "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	} {
		tc := tc

		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.APIURL = &config.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			ok, err := pd.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
			require.Equal(t, tc.retry, ok)
		})
	}
}

func TestJiraNotify(t *testing.T) {
	for _, tc := range []struct {
		title string
		cfg   *config.JiraConfig

		alert *types.Alert

		customFieldAssetFn func(t *testing.T, issue map[string]any)
		searchResponse     issueSearchResult
		issue              issue
		errMsg             string
	}{
		{
			title: "create new issue",
			cfg: &config.JiraConfig{
				Summary:           `{{ template "jira.default.summary" . }}`,
				Description:       `{{ template "jira.default.description" . }}`,
				IssueType:         "Incident",
				Project:           "OPS",
				Priority:          `{{ template "jira.default.priority" . }}`,
				Labels:            []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
						"severity":  "critical",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total:  0,
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     "[FIRING:1] test (vm1 critical)",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n  - severity = critical\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "",
		},
		{
			title: "create new issue with template project and issue type",
			cfg: &config.JiraConfig{
				Summary:           `{{ template "jira.default.summary" . }}`,
				Description:       `{{ template "jira.default.description" . }}`,
				IssueType:         "{{ .CommonLabels.issue_type }}",
				Project:           "{{ .CommonLabels.project }}",
				Priority:          `{{ template "jira.default.priority" . }}`,
				Labels:            []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname":  "test",
						"instance":   "vm1",
						"severity":   "critical",
						"project":    "MONITORING",
						"issue_type": "MINOR",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total:  0,
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     "[FIRING:1] test (vm1 MINOR MONITORING critical)",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n  - issue_type = MINOR\n  - project = MONITORING\n  - severity = critical\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "MINOR"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "MONITORING"},
					Priority:    &idNameValue{Name: "High"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "",
		},
		{
			title: "create new issue with custom field and too long summary",
			cfg: &config.JiraConfig{
				Summary:     strings.Repeat("A", maxSummaryLenRunes+10),
				Description: `{{ template "jira.default.description" . }}`,
				IssueType:   "Incident",
				Project:     "OPS",
				Priority:    `{{ template "jira.default.priority" . }}`,
				Labels:      []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				Fields: map[string]any{
					"components":        map[any]any{"name": "Monitoring"},
					"customfield_10001": "value",
					"customfield_10002": 0,
					"customfield_10003": []any{0},
					"customfield_10004": map[any]any{"value": "red"},
					"customfield_10005": map[any]any{"value": 0},
					"customfield_10006": []map[any]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
					"customfield_10007": []map[any]any{{"value": "red"}, {"value": "blue"}, {"value": 0}},
					"customfield_10008": []map[any]any{{"value": 0}, {"value": 1}, {"value": 2}},
				},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total:  0,
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     strings.Repeat("A", maxSummaryLenRunes-1) + "…",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {
				require.Equal(t, "value", issue["customfield_10001"])
				require.Equal(t, float64(0), issue["customfield_10002"])
				require.Equal(t, []any{float64(0)}, issue["customfield_10003"])
				require.Equal(t, map[string]any{"value": "red"}, issue["customfield_10004"])
				require.Equal(t, map[string]any{"value": float64(0)}, issue["customfield_10005"])
				require.Equal(t, []any{map[string]any{"value": "red"}, map[string]any{"value": "blue"}, map[string]any{"value": "green"}}, issue["customfield_10006"])
				require.Equal(t, []any{map[string]any{"value": "red"}, map[string]any{"value": "blue"}, map[string]any{"value": float64(0)}}, issue["customfield_10007"])
				require.Equal(t, []any{map[string]any{"value": float64(0)}, map[string]any{"value": float64(1)}, map[string]any{"value": float64(2)}}, issue["customfield_10008"])
			},
			errMsg: "",
		},
		{
			title: "reopen issue",
			cfg: &config.JiraConfig{
				Summary:           `{{ template "jira.default.summary" . }}`,
				Description:       `{{ template "jira.default.description" . }}`,
				IssueType:         "Incident",
				Project:           "OPS",
				Priority:          `{{ template "jira.default.priority" . }}`,
				Labels:            []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total: 1,
				Issues: []issue{
					{
						Key: "OPS-1",
						Fields: &issueFields{
							Status: &issueStatus{
								Name: "Closed",
								StatusCategory: struct {
									Key string `json:"key"`
								}{
									Key: "done",
								},
							},
						},
					},
				},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     "[FIRING:1] test (vm1)",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "",
		},
		{
			title: "error resolve transition not found",
			cfg: &config.JiraConfig{
				Summary:           `{{ template "jira.default.summary" . }}`,
				Description:       `{{ template "jira.default.description" . }}`,
				IssueType:         "Incident",
				Project:           "OPS",
				Priority:          `{{ template "jira.default.priority" . }}`,
				Labels:            []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
					},
					StartsAt: time.Now().Add(-time.Hour),
					EndsAt:   time.Now().Add(-time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total: 1,
				Issues: []issue{
					{
						Key: "OPS-3",
						Fields: &issueFields{
							Status: &issueStatus{
								Name: "Open",
								StatusCategory: struct {
									Key string `json:"key"`
								}{
									Key: "open",
								},
							},
						},
					},
				},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     "[RESOLVED] test (vm1)",
					Description: "\n\n\n# Alerts Resolved:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "can't find transition CLOSE for issue OPS-3",
		},
		{
			title: "error reopen transition not found",
			cfg: &config.JiraConfig{
				Summary:           `{{ template "jira.default.summary" . }}`,
				Description:       `{{ template "jira.default.description" . }}`,
				IssueType:         "Incident",
				Project:           "OPS",
				Priority:          `{{ template "jira.default.priority" . }}`,
				Labels:            []string{"alertmanager", "{{ .GroupLabels.alertname }}"},
				ReopenDuration:    model.Duration(1 * time.Hour),
				ReopenTransition:  "REOPEN",
				ResolveTransition: "CLOSE",
				WontFixResolution: "WONTFIX",
			},
			alert: &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			},
			searchResponse: issueSearchResult{
				Total: 1,
				Issues: []issue{
					{
						Key: "OPS-3",
						Fields: &issueFields{
							Status: &issueStatus{
								Name: "Closed",
								StatusCategory: struct {
									Key string `json:"key"`
								}{
									Key: "done",
								},
							},
						},
					},
				},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     "[FIRING:1] test (vm1)",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "can't find transition REOPEN for issue OPS-3",
		},
	} {
		tc := tc

		t.Run(tc.title, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search":
					enc := json.NewEncoder(w)
					if err := enc.Encode(tc.searchResponse); err != nil {
						panic(err)
					}

					return
				case "/issue/OPS-1/transitions":
					switch r.Method {
					case http.MethodGet:
						w.WriteHeader(http.StatusOK)

						transitions := issueTransitions{
							Transitions: []idNameValue{
								{ID: "12345", Name: "REOPEN"},
							},
						}

						enc := json.NewEncoder(w)
						if err := enc.Encode(transitions); err != nil {
							panic(err)
						}
					case http.MethodPost:
						dec := json.NewDecoder(r.Body)
						var out issue
						err := dec.Decode(&out)
						if err != nil {
							panic(err)
						}

						require.Equal(t, issue{Transition: &idNameValue{ID: "12345"}}, out)
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Fatalf("unexpected method %s", r.Method)
					}

					return
				case "/issue/OPS-2/transitions":
					switch r.Method {
					case http.MethodGet:
						w.WriteHeader(http.StatusOK)

						transitions := issueTransitions{
							Transitions: []idNameValue{
								{ID: "54321", Name: "CLOSE"},
							},
						}

						enc := json.NewEncoder(w)
						if err := enc.Encode(transitions); err != nil {
							panic(err)
						}
					case http.MethodPost:
						dec := json.NewDecoder(r.Body)
						var out issue
						err := dec.Decode(&out)
						if err != nil {
							panic(err)
						}

						require.Equal(t, issue{Transition: &idNameValue{ID: "54321"}}, out)
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Fatalf("unexpected method %s", r.Method)
					}

					return
				case "/issue/OPS-3/transitions":
					switch r.Method {
					case http.MethodGet:
						w.WriteHeader(http.StatusOK)

						transitions := issueTransitions{
							Transitions: []idNameValue{},
						}

						enc := json.NewEncoder(w)
						if err := enc.Encode(transitions); err != nil {
							panic(err)
						}
					default:
						t.Fatalf("unexpected method %s", r.Method)
					}

					return
				case "/issue/OPS-1":
				case "/issue/OPS-2":
				case "/issue/OPS-3":
					fallthrough
				case "/issue":
					body, err := io.ReadAll(r.Body)
					if err != nil {
						panic(err)
					}

					var (
						issue issue
						raw   map[string]any
					)

					if err := json.Unmarshal(body, &issue); err != nil {
						panic(err)
					}

					// We don't care about the key, so copy it over.
					issue.Fields.Fields = tc.issue.Fields.Fields

					require.Equal(t, tc.issue.Key, issue.Key)
					require.Equal(t, tc.issue.Fields, issue.Fields)

					if err := json.Unmarshal(body, &raw); err != nil {
						panic(err)
					}

					if fields, ok := raw["fields"].(map[string]any); ok {
						tc.customFieldAssetFn(t, fields)
					} else {
						t.Errorf("fields should a map of string")
					}

					w.WriteHeader(http.StatusCreated)

					w.WriteHeader(http.StatusCreated)

				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			tc.cfg.APIURL = &config.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}

			notifier, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")
			ctx = notify.WithGroupLabels(ctx, model.LabelSet{"alertname": "test"})

			_, err = notifier.Notify(ctx, tc.alert)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, tc.errMsg)
			}
		})
	}
}

func TestJiraPriority(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		title string

		alerts []*types.Alert

		expectedPriority string
	}{
		{
			"empty",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"",
		},
		{
			"critical",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "critical",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"High",
		},
		{
			"warning",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "warning",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"Medium",
		},
		{
			"info",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "info",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"Low",
		},
		{
			"critical+warning+info",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "critical",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "warning",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "info",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"High",
		},
		{
			"warning+info",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "warning",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "info",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"Medium",
		},
		{
			"critical(resolved)+warning+info",
			[]*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "critical",
						},
						StartsAt: time.Now().Add(-time.Hour),
						EndsAt:   time.Now().Add(-time.Hour),
					},
				},
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "warning",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "test",
							"instance":  "vm1",
							"severity":  "info",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			},
			"Medium",
		},
	} {
		tc := tc

		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse("http://example.com/")
			require.NoError(t, err)

			tmpl, err := template.FromGlobs([]string{})
			require.NoError(t, err)

			tmpl.ExternalURL = u

			var (
				data = tmpl.Data("jira", model.LabelSet{}, tc.alerts...)

				tmplTextErr  error
				tmplText     = notify.TmplText(tmpl, data, &tmplTextErr)
				tmplTextFunc = func(tmpl string) (string, error) {
					result := tmplText(tmpl)
					return result, tmplTextErr
				}
			)

			priority, err := tmplTextFunc(`{{ template "jira.default.priority" . }}`)
			require.NoError(t, err)
			require.Equal(t, tc.expectedPriority, priority)
		})
	}
}
