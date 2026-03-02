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

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

func jiraStringDescription(v string) *jiraDescription {
	return &jiraDescription{StringDescription: stringPtr(v)}
}

func stringPtr(v string) *string {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func TestJiraRetry(t *testing.T) {
	notifier, err := New(
		&config.JiraConfig{
			APIURL: &amcommoncfg.URL{
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
		require.Equal(t, expected, actual, "retry - error on status %d", statusCode)
	}
}

func TestSearchExistingIssue(t *testing.T) {
	expectedJQL := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			// Unmarshal the JSON data into the struct
			var data issueSearch
			err = json.Unmarshal(body, &data)
			if err != nil {
				http.Error(w, "Error unmarshaling JSON", http.StatusBadRequest)
				return
			}
			require.Equal(t, expectedJQL, data.JQL)
			w.Write([]byte(`{"issues": []}`))
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
		title         string
		cfg           *config.JiraConfig
		groupKey      string
		firing        bool
		expectedJQL   string
		expectedIssue *issue
		expectedErr   bool
		expectedRetry bool
	}{
		{
			title: "search existing issue with project template for firing alert",
			cfg: &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
				Project:     `{{ .CommonLabels.project }}`,
			},
			groupKey:    "1",
			firing:      true,
			expectedJQL: `statusCategory != Done and project="PROJ" and labels="ALERT{1}" order by status ASC,resolutiondate DESC`,
		},
		{
			title: "search existing issue with reopen duration for firing alert",
			cfg: &config.JiraConfig{
				Summary:          config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:      config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
				Project:          `{{ .CommonLabels.project }}`,
				ReopenDuration:   model.Duration(60 * time.Minute),
				ReopenTransition: "REOPEN",
			},
			groupKey:    "1",
			firing:      true,
			expectedJQL: `(resolutiondate is EMPTY OR resolutiondate >= -60m) and project="PROJ" and labels="ALERT{1}" order by status ASC,resolutiondate DESC`,
		},
		{
			title: "search existing issue for resolved alert",
			cfg: &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
				Project:     `{{ .CommonLabels.project }}`,
			},
			groupKey:    "1",
			firing:      false,
			expectedJQL: `statusCategory != Done and project="PROJ" and labels="ALERT{1}" order by status ASC,resolutiondate DESC`,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			expectedJQL = tc.expectedJQL
			tc.cfg.APIURL = &amcommoncfg.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}

			as := []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"project": "PROJ",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}

			pd, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)
			logger := pd.logger.With("group_key", tc.groupKey)

			ctx := notify.WithGroupKey(context.Background(), tc.groupKey)
			data := notify.GetTemplateData(ctx, pd.tmpl, as, logger)

			var tmplTextErr error
			tmplText := notify.TmplText(pd.tmpl, data, &tmplTextErr)
			tmplTextFunc := func(tmpl string) (string, error) {
				return tmplText(tmpl), tmplTextErr
			}

			issue, retry, err := pd.searchExistingIssue(ctx, logger, tc.groupKey, tc.firing, tmplTextFunc)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedIssue, issue)
			require.Equal(t, tc.expectedRetry, retry)
		})
	}
}

func TestPrepareSearchRequest(t *testing.T) {
	for _, tc := range []struct {
		title           string
		cfg             *config.JiraConfig
		jql             string
		expectedBody    any
		expectedURL     string
		expectedURLPath string
	}{
		{
			title: "cloud API type",
			cfg: &config.JiraConfig{
				APIType: "cloud",
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "example.atlassian.net",
						Path:   "/rest/api/2",
					},
				},
			},
			jql: "project=TEST and labels=\"ALERT{123}\"",
			expectedBody: issueSearch{
				JQL:        "project=TEST and labels=\"ALERT{123}\"",
				MaxResults: 2,
				Fields:     []string{"status"},
			},
			expectedURL:     "https://example.atlassian.net/rest/api/3/search/jql",
			expectedURLPath: "/rest/api/2",
		},
		{
			title: "auto API type with atlassian.net url",
			cfg: &config.JiraConfig{
				APIType: "auto",
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "example.atlassian.net",
						Path:   "/rest/api/2",
					},
				},
			},
			jql: "project=TEST and labels=\"ALERT{123}\"",
			expectedBody: issueSearch{
				JQL:        "project=TEST and labels=\"ALERT{123}\"",
				MaxResults: 2,
				Fields:     []string{"status"},
			},
			expectedURL:     "https://example.atlassian.net/rest/api/3/search/jql",
			expectedURLPath: "/rest/api/2",
		},
		{
			title: "auto API type without atlassian.net url",
			cfg: &config.JiraConfig{
				APIType: "auto",
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "jira.example.com",
						Path:   "/rest/api/2",
					},
				},
			},
			jql: "project=TEST and labels=\"ALERT{123}\"",
			expectedBody: issueSearch{
				JQL:        "project=TEST and labels=\"ALERT{123}\"",
				MaxResults: 2,
				Fields:     []string{"status"},
			},
			expectedURL:     "https://jira.example.com/rest/api/2/search",
			expectedURLPath: "/rest/api/2",
		},
		{
			title: "atlassian.net URL suffix but datacenter api type",
			cfg: &config.JiraConfig{
				APIType: "datacenter",
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "example.atlassian.net",
						Path:   "/rest/api/2",
					},
				},
			},
			jql: "project=TEST and labels=\"ALERT{123}\"",
			expectedBody: issueSearch{
				JQL:        "project=TEST and labels=\"ALERT{123}\"",
				MaxResults: 2,
				Fields:     []string{"status"},
			},
			expectedURL:     "https://example.atlassian.net/rest/api/2/search",
			expectedURLPath: "/rest/api/2",
		},
		{
			title: "datacenter API type",
			cfg: &config.JiraConfig{
				APIType: "datacenter",
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "jira.example.com",
						Path:   "/rest/api/2",
					},
				},
			},
			jql: "project=TEST and labels=\"ALERT{123}\"",
			expectedBody: issueSearch{
				JQL:        "project=TEST and labels=\"ALERT{123}\"",
				MaxResults: 2,
				Fields:     []string{"status"},
			},
			expectedURL:     "https://jira.example.com/rest/api/2/search",
			expectedURLPath: "/rest/api/2",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}

			notifier, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			requestBody, searchURL := notifier.prepareSearchRequest(tc.jql)

			require.Equal(t, tc.expectedURL, searchURL)
			require.Equal(t, tc.expectedBody, requestBody)
			// Verify that the original APIURL.Path is not modified
			require.Equal(t, tc.expectedURLPath, notifier.conf.APIURL.Path)
		})
	}
}

func TestJiraTemplating(t *testing.T) {
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"issues": []}`))
			return
		default:
			dec := json.NewDecoder(r.Body)
			out := make(map[string]any)
			if err := dec.Decode(&out); err != nil {
				panic(err)
			}
			capturedBody = out
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	for _, tc := range []struct {
		title string
		cfg   *config.JiraConfig

		retry              bool
		errMsg             string
		expectedFieldKey   string
		expectedFieldValue any
	}{
		{
			title: "full-blown message with templated custom field",
			cfg: &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
				Fields: map[string]any{
					"customfield_14400": `{{ template "jira.host" . }}`,
				},
			},
			retry:              false,
			expectedFieldKey:   "customfield_14400",
			expectedFieldValue: "host1.example.com",
		},
		{
			title: "template project",
			cfg: &config.JiraConfig{
				Project:     `{{ .CommonLabels.lbl1 }}`,
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
			},
			retry: false,
		},
		{
			title: "template issue type",
			cfg: &config.JiraConfig{
				IssueType:   `{{ .CommonLabels.lbl1 }}`,
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
			},
			retry: false,
		},
		{
			title: "summary with templating errors",
			cfg: &config.JiraConfig{
				Summary: config.JiraFieldConfig{Template: "{{ "},
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "description with templating errors",
			cfg: &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: "{{ "},
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "priority with templating errors",
			cfg: &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
				Priority:    "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			capturedBody = nil

			tc.cfg.APIURL = &amcommoncfg.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			// Add the jira.host template just for this test
			if tc.expectedFieldKey == "customfield_14400" {
				err = pd.tmpl.Parse(strings.NewReader(`{{ define "jira.host" }}{{ .CommonLabels.hostname }}{{ end }}`))
				require.NoError(t, err)
			}

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")
			ctx = notify.WithGroupLabels(ctx, model.LabelSet{
				"lbl1":     "val1",
				"hostname": "host1.example.com",
			})

			ok, err := pd.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1":     "val1",
							"hostname": "host1.example.com",
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

			// Verify that custom fields were templated correctly
			if tc.expectedFieldKey != "" {
				require.NotNil(t, capturedBody, "expected request body")
				fields, ok := capturedBody["fields"].(map[string]any)
				require.True(t, ok, "fields should be a map")
				require.Equal(t, tc.expectedFieldValue, fields[tc.expectedFieldKey])
			}
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
				Summary:           config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:       config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     stringPtr("[FIRING:1] test (vm1 critical)"),
					Description: jiraStringDescription("\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n  - severity = critical\n\nAnnotations:\n\nSource: \n\n\n\n\n"),
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
			title: "update existing issue with disabled summary and description",
			cfg: &config.JiraConfig{
				Summary: config.JiraFieldConfig{
					Template:     `{{ template "jira.default.summary" . }}`,
					EnableUpdate: boolPtr(false),
				},
				Description: config.JiraFieldConfig{
					Template:     `{{ template "jira.default.description" . }}`,
					EnableUpdate: boolPtr(false),
				},
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
				Issues: []issue{
					{
						Key: "MONITORING-1",
						Fields: &issueFields{
							Summary:     stringPtr("Original Summary"),
							Description: jiraStringDescription("Original Description"),
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
				Key: "MONITORING-1",
				Fields: &issueFields{
					// Summary and Description should NOT be present in the update request
					Issuetype: &idNameValue{Name: "MINOR"},
					Labels:    []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:   &issueProject{Key: "MONITORING"},
					Priority:  &idNameValue{Name: "High"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {
				// Verify that summary and description are NOT in the update request
				_, hasSummary := issue["summary"]
				_, hasDescription := issue["description"]
				require.False(t, hasSummary, "summary should not be present in update request")
				require.False(t, hasDescription, "description should not be present in update request")
			},
			errMsg: "",
		},
		{
			title: "create new issue with template project and issue type",
			cfg: &config.JiraConfig{
				Summary:           config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:       config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     stringPtr("[FIRING:1] test (vm1 MINOR MONITORING critical)"),
					Description: jiraStringDescription("\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n  - issue_type = MINOR\n  - project = MONITORING\n  - severity = critical\n\nAnnotations:\n\nSource: \n\n\n\n\n"),
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
				Summary:     config.JiraFieldConfig{Template: strings.Repeat("A", maxSummaryLenRunes+10)},
				Description: config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
					"customfield_10009": []map[any]any{{1: 0}, {1.0: 1}, {"a": []any{2}}},
					"customfield_10010": []any{map[any]any{1: 0}, []int{3}},
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
				Issues: []issue{},
			},
			issue: issue{
				Key: "",
				Fields: &issueFields{
					Summary:     stringPtr(strings.Repeat("A", maxSummaryLenRunes-1) + "…"),
					Description: jiraStringDescription("\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n"),
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
				require.Equal(t, []any([]any{map[string]any{}, map[string]any{}, map[string]any{"a": []any{2.0}}}),
					issue["customfield_10009"])
				require.Equal(t, []any{map[string]any{}, []any{3.0}}, issue["customfield_10010"])
			},
			errMsg: "",
		},
		{
			title: "reopen issue",
			cfg: &config.JiraConfig{
				Summary:           config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:       config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
					Summary:     stringPtr("[FIRING:1] test (vm1)"),
					Description: jiraStringDescription("\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n"),
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
				Summary:           config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:       config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
					Summary:     stringPtr("[RESOLVED] test (vm1)"),
					Description: jiraStringDescription("\n\n\n# Alerts Resolved:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n"),
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
				Summary:           config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description:       config.JiraFieldConfig{Template: `{{ template "jira.default.description" . }}`},
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
					Summary:     stringPtr("[FIRING:1] test (vm1)"),
					Description: jiraStringDescription("\n\n# Alerts Firing:\n\nLabels:\n  - alertname = test\n  - instance = vm1\n\nAnnotations:\n\nSource: \n\n\n\n\n"),
					Issuetype:   &idNameValue{Name: "Incident"},
					Labels:      []string{"ALERT{6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b}", "alertmanager", "test"},
					Project:     &issueProject{Key: "OPS"},
				},
			},
			customFieldAssetFn: func(t *testing.T, issue map[string]any) {},
			errMsg:             "can't find transition REOPEN for issue OPS-3",
		},
	} {
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
				case "/issue/MONITORING-1":
					body, err := io.ReadAll(r.Body)
					if err != nil {
						panic(err)
					}

					var raw map[string]any
					if err := json.Unmarshal(body, &raw); err != nil {
						panic(err)
					}

					if fields, ok := raw["fields"].(map[string]any); ok {
						tc.customFieldAssetFn(t, fields)
					}

					w.WriteHeader(http.StatusNoContent)
					return
				case "/issue/OPS-1":
				case "/issue/OPS-2":
				case "/issue/OPS-3":
				case "/issue/OPS-4":
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

			tc.cfg.APIURL = &amcommoncfg.URL{URL: u}
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
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse("http://example.com/")
			require.NoError(t, err)

			tmpl, err := template.FromGlobs([]string{})
			require.NoError(t, err)

			tmpl.ExternalURL = u

			var (
				data = tmpl.Data("jira", model.LabelSet{}, notify.ReasonFirstNotification.String(), tc.alerts...)

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

func TestPrepareIssueRequestBodyAPIv3DescriptionValidation(t *testing.T) {
	for _, tc := range []struct {
		name                string
		descriptionTemplate string
		expectErrSubstring  string
	}{
		{
			name:                "valid JSON description",
			descriptionTemplate: `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`,
		},
		{
			name:                "invalid JSON description",
			descriptionTemplate: `not-json`,
			expectErrSubstring:  "invalid JSON for API v3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.JiraConfig{
				Summary:     config.JiraFieldConfig{Template: `{{ template "jira.default.summary" . }}`},
				Description: config.JiraFieldConfig{Template: tc.descriptionTemplate},
				IssueType:   "Incident",
				Project:     "OPS",
				Labels:      []string{"alertmanager"},
				Priority:    `{{ template "jira.default.priority" . }}`,
				APIURL: &amcommoncfg.URL{
					URL: &url.URL{
						Scheme: "https",
						Host:   "example.atlassian.net",
						Path:   "/rest/api/3",
					},
				},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			}

			notifier, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			alert := &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "test",
						"instance":  "vm1",
						"severity":  "critical",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}

			ctx := context.Background()
			groupID := "1"
			ctx = notify.WithGroupKey(ctx, groupID)
			ctx = notify.WithGroupLabels(ctx, alert.Labels)

			alerts := []*types.Alert{alert}
			logger := notifier.logger.With("group_key", groupID)
			data := notify.GetTemplateData(ctx, notifier.tmpl, alerts, logger)

			var tmplErr error
			tmplText := notify.TmplText(notifier.tmpl, data, &tmplErr)
			tmplTextFunc := func(tmpl string) (string, error) {
				return tmplText(tmpl), tmplErr
			}

			issue, err := notifier.prepareIssueRequestBody(ctx, logger, groupID, tmplTextFunc)
			if tc.expectErrSubstring != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectErrSubstring)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, issue.Fields)

			require.NotNil(t, issue.Fields.Description)
			require.JSONEq(t, tc.descriptionTemplate, string(issue.Fields.Description.RawJSONDescription))
		})
	}
}
