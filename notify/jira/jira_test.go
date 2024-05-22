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
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-kit/log"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
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
		log.NewNopLogger(),
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
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.APIURL = &config.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), log.NewNopLogger())
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

		searchResponse issueSearchResult
		issue          issue
		errMsg         string
	}{
		{
			title: "create new issue",
			cfg: &config.JiraConfig{
				Summary:      `{{ template "jira.default.summary" . }}`,
				Description:  `{{ template "jira.default.description" . }}`,
				IssueType:    "Incident",
				Project:      "OPS",
				Priority:     "High",
				StaticLabels: []string{"alertmanager"},
				GroupLabels:  []string{"alertname"},
				Components:   []string{"Monitoring"},
				CustomFields: map[string]any{
					"customfield_10001": "value",
					"customfield_10002": map[string]any{"value": "red"},
					"customfield_10003": []map[string]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
				},
				ReopenDuration:    config.Duration(1 * time.Hour),
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
					Summary:     "[FIRING:1] test (vm1)",
					Description: "\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n",
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"alertmanager", "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
					Components:  []idNameValue{{Name: "Monitoring"}},
					CustomFields: map[string]any{
						"customfield_10001": "value",
						"customfield_10002": map[string]any{"value": "red"},
						"customfield_10003": []map[string]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
					},
				},
			},
			errMsg: "",
		},
		{
			title: "reopen issue",
			cfg: &config.JiraConfig{
				Summary:      `{{ template "jira.default.summary" . }}`,
				Description:  `{{ template "jira.default.description" . }}`,
				IssueType:    "Incident",
				Project:      "OPS",
				Priority:     "High",
				StaticLabels: []string{"alertmanager"},
				GroupLabels:  []string{"alertname"},
				Components:   []string{"Monitoring"},
				CustomFields: map[string]any{
					"customfield_10001": "value",
					"customfield_10002": map[string]string{"value": "red"},
					"customfield_10003": []map[string]string{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
				},
				ReopenDuration:    config.Duration(1 * time.Hour),
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
					Labels:      []string{"alertmanager", "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
					Components:  []idNameValue{{Name: "Monitoring"}},
					CustomFields: map[string]any{
						"customfield_10001": "value",
						"customfield_10002": map[string]any{"value": "red"},
						"customfield_10003": []map[string]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
					},
				},
			},
			errMsg: "",
		},
		{
			title: "error resolve transition not found",
			cfg: &config.JiraConfig{
				Summary:      `{{ template "jira.default.summary" . }}`,
				Description:  `{{ template "jira.default.description" . }}`,
				IssueType:    "Incident",
				Project:      "OPS",
				Priority:     "High",
				StaticLabels: []string{"alertmanager"},
				GroupLabels:  []string{"alertname"},
				Components:   []string{"Monitoring"},
				CustomFields: map[string]any{
					"customfield_10001": "value",
					"customfield_10002": map[string]string{"value": "red"},
					"customfield_10003": []map[string]string{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
				},
				ReopenDuration:    config.Duration(1 * time.Hour),
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
					Labels:      []string{"alertmanager", "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
					Components:  []idNameValue{{Name: "Monitoring"}},
					CustomFields: map[string]any{
						"customfield_10001": "value",
						"customfield_10002": map[string]any{"value": "red"},
						"customfield_10003": []map[string]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
					},
				},
			},
			errMsg: "can't find transition CLOSE for issue OPS-3",
		},
		{
			title: "error reopen transition not found",
			cfg: &config.JiraConfig{
				Summary:      `{{ template "jira.default.summary" . }}`,
				Description:  `{{ template "jira.default.description" . }}`,
				IssueType:    "Incident",
				Project:      "OPS",
				Priority:     "High",
				StaticLabels: []string{"alertmanager"},
				GroupLabels:  []string{"alertname"},
				Components:   []string{"Monitoring"},
				CustomFields: map[string]any{
					"customfield_10001": "value",
					"customfield_10002": map[string]string{"value": "red"},
					"customfield_10003": []map[string]string{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
				},
				ReopenDuration:    config.Duration(1 * time.Hour),
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
					Labels:      []string{"alertmanager", "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "test"},
					Project:     &issueProject{Key: "OPS"},
					Priority:    &idNameValue{Name: "High"},
					Components:  []idNameValue{{Name: "Monitoring"}},
					CustomFields: map[string]any{
						"customfield_10001": "value",
						"customfield_10002": map[string]any{"value": "red"},
						"customfield_10003": []map[string]any{{"value": "red"}, {"value": "blue"}, {"value": "green"}},
					},
				},
			},
			errMsg: "can't find transition REOPEN for issue OPS-3",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

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

						assert.Equal(t, issue{Transition: &idNameValue{ID: "12345"}}, out)
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

						assert.Equal(t, issue{Transition: &idNameValue{ID: "54321"}}, out)
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
					issue.Fields.CustomFields = tc.issue.Fields.CustomFields

					assert.Equal(t, tc.issue.Key, issue.Key)
					assert.Equal(t, tc.issue.Fields, issue.Fields)

					if err := json.Unmarshal(body, &raw); err != nil {
						panic(err)
					}

					assert.Equal(t, tc.issue.Fields.CustomFields["customfield_10001"], raw["fields"].(map[string]any)["customfield_10001"])
					assert.Equal(t, tc.issue.Fields.CustomFields["customfield_10002"], raw["fields"].(map[string]any)["customfield_10002"])
					assert.Equal(t, tc.issue.Fields.CustomFields["customfield_10003"].([]map[string]any)[0], raw["fields"].(map[string]any)["customfield_10003"].([]any)[0])
					assert.Equal(t, tc.issue.Fields.CustomFields["customfield_10003"].([]map[string]any)[1], raw["fields"].(map[string]any)["customfield_10003"].([]any)[1])
					assert.Equal(t, tc.issue.Fields.CustomFields["customfield_10003"].([]map[string]any)[2], raw["fields"].(map[string]any)["customfield_10003"].([]any)[2])

					w.WriteHeader(http.StatusCreated)

				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			tc.cfg.APIURL = &config.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}

			notifier, err := New(tc.cfg, test.CreateTmpl(t), log.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")
			ctx = notify.WithGroupLabels(ctx, model.LabelSet{"alertname": "test"})

			_, err = notifier.Notify(ctx, tc.alert)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, tc.errMsg)
			}
		})
	}
}
