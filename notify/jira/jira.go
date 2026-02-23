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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	maxSummaryLenRunes     = 255
	maxDescriptionLenRunes = 32767
)

// Notifier implements a Notifier for JIRA notifications.
type Notifier struct {
	conf    *config.JiraConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

func New(c *config.JiraConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "jira", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
	}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	logger := n.logger.With("group_key", key.String())
	logger.Debug("extracted group key")

	var (
		alerts = types.Alerts(as...)

		tmplTextErr  error
		data         = notify.GetTemplateData(ctx, n.tmpl, as, logger)
		tmplText     = notify.TmplText(n.tmpl, data, &tmplTextErr)
		tmplTextFunc = func(tmpl string) (string, error) {
			return tmplText(tmpl), tmplTextErr
		}

		path   = "issue"
		method = http.MethodPost
	)

	existingIssue, shouldRetry, err := n.searchExistingIssue(ctx, logger, key.Hash(), alerts.HasFiring(), tmplTextFunc)
	if err != nil {
		return shouldRetry, fmt.Errorf("failed to look up existing issues: %w", err)
	}

	if existingIssue == nil {
		// Do not create new issues for resolved alerts
		if alerts.Status() == model.AlertResolved {
			return false, nil
		}

		logger.Debug("create new issue")
	} else {
		path = "issue/" + existingIssue.Key
		method = http.MethodPut
		logger.Debug("updating existing issue", "issue_key", existingIssue.Key, "summary_update_enabled", n.conf.Summary.EnableUpdateValue(), "description_update_enabled", n.conf.Description.EnableUpdateValue())
	}

	requestBody, err := n.prepareIssueRequestBody(ctx, logger, key.Hash(), tmplTextFunc)
	if err != nil {
		return false, err
	}

	if method == http.MethodPut && requestBody.Fields != nil {
		if !n.conf.Description.EnableUpdateValue() {
			requestBody.Fields.Description = nil
		}
		if !n.conf.Summary.EnableUpdateValue() {
			requestBody.Fields.Summary = nil
		}
	}

	_, shouldRetry, err = n.doAPIRequest(ctx, method, path, requestBody)
	if err != nil {
		return shouldRetry, fmt.Errorf("failed to %s request to %q: %w", method, path, err)
	}

	return n.transitionIssue(ctx, logger, existingIssue, alerts.HasFiring())
}

func (n *Notifier) prepareIssueRequestBody(_ context.Context, logger *slog.Logger, groupID string, tmplTextFunc template.TemplateFunc) (issue, error) {
	summary, err := tmplTextFunc(n.conf.Summary.Template)
	if err != nil {
		return issue{}, fmt.Errorf("summary template: %w", err)
	}

	project, err := tmplTextFunc(n.conf.Project)
	if err != nil {
		return issue{}, fmt.Errorf("project template: %w", err)
	}
	issueType, err := tmplTextFunc(n.conf.IssueType)
	if err != nil {
		return issue{}, fmt.Errorf("issue_type template: %w", err)
	}

	fieldsWithStringKeys := make(map[string]any, len(n.conf.Fields))
	for key, value := range n.conf.Fields {
		fieldsWithStringKeys[key], err = template.DeepCopyWithTemplate(value, tmplTextFunc)
		if err != nil {
			return issue{}, fmt.Errorf("fields template: %w", err)
		}
	}

	summary, truncated := notify.TruncateInRunes(summary, maxSummaryLenRunes)
	if truncated {
		logger.Warn("Truncated summary", "max_runes", maxSummaryLenRunes)
	}

	requestBody := issue{Fields: &issueFields{
		Project:   &issueProject{Key: project},
		Issuetype: &idNameValue{Name: issueType},
		Summary:   &summary,
		Labels:    make([]string, 0, len(n.conf.Labels)+1),
		Fields:    fieldsWithStringKeys,
	}}

	issueDescriptionString, err := tmplTextFunc(n.conf.Description.Template)
	if err != nil {
		return issue{}, fmt.Errorf("description template: %w", err)
	}

	issueDescriptionString, truncated = notify.TruncateInRunes(issueDescriptionString, maxDescriptionLenRunes)
	if truncated {
		logger.Warn("Truncated description", "max_runes", maxDescriptionLenRunes)
	}

	var description *jiraDescription
	descriptionCopy := issueDescriptionString
	if isAPIv3Path(n.conf.APIURL.Path) {
		descriptionCopy = strings.TrimSpace(descriptionCopy)
		if descriptionCopy != "" {
			if !json.Valid([]byte(descriptionCopy)) {
				return issue{}, fmt.Errorf("description template: invalid JSON for API v3")
			}
			raw := json.RawMessage(descriptionCopy)
			description = &jiraDescription{
				RawJSONDescription: append(json.RawMessage(nil), raw...),
			}
		}
	} else if descriptionCopy != "" {
		desc := descriptionCopy
		description = &jiraDescription{StringDescription: &desc}
	}

	requestBody.Fields.Description = description

	for i, label := range n.conf.Labels {
		label, err = tmplTextFunc(label)
		if err != nil {
			return issue{}, fmt.Errorf("labels[%d] template: %w", i, err)
		}
		requestBody.Fields.Labels = append(requestBody.Fields.Labels, label)
	}
	requestBody.Fields.Labels = append(requestBody.Fields.Labels, fmt.Sprintf("ALERT{%s}", groupID))
	sort.Strings(requestBody.Fields.Labels)

	priority, err := tmplTextFunc(n.conf.Priority)
	if err != nil {
		return issue{}, fmt.Errorf("priority template: %w", err)
	}

	if priority != "" {
		requestBody.Fields.Priority = &idNameValue{Name: priority}
	}

	return requestBody, nil
}

func (n *Notifier) searchExistingIssue(ctx context.Context, logger *slog.Logger, groupID string, firing bool, tmplTextFunc template.TemplateFunc) (*issue, bool, error) {
	jql := strings.Builder{}

	if n.conf.WontFixResolution != "" {
		fmt.Fprintf(&jql, `resolution != %q and `, n.conf.WontFixResolution)
	}

	// If the group is firing, search for open issues. If a reopen transition is
	// defined, also search for issues that were closed within the reopen duration.
	if firing {
		reopenDuration := int64(time.Duration(n.conf.ReopenDuration).Minutes())
		if n.conf.ReopenTransition != "" && reopenDuration > 0 {
			fmt.Fprintf(&jql, `(resolutiondate is EMPTY OR resolutiondate >= -%dm) and `, reopenDuration)
		} else {
			jql.WriteString(`statusCategory != Done and `)
		}
	} else {
		jql.WriteString(`statusCategory != Done and `)
	}

	alertLabel := fmt.Sprintf("ALERT{%s}", groupID)
	project, err := tmplTextFunc(n.conf.Project)
	if err != nil {
		return nil, false, fmt.Errorf("invalid project template or value: %w", err)
	}
	fmt.Fprintf(&jql, `project=%q and labels=%q order by status ASC,resolutiondate DESC`, project, alertLabel)

	requestBody, searchPath := n.prepareSearchRequest(jql.String())

	logger.Debug("search for recent issues", "jql", jql.String())

	responseBody, shouldRetry, err := n.doAPIRequestFullPath(ctx, http.MethodPost, searchPath, requestBody)
	if err != nil {
		return nil, shouldRetry, fmt.Errorf("HTTP request to JIRA API: %w", err)
	}

	var issueSearchResult issueSearchResult
	err = json.Unmarshal(responseBody, &issueSearchResult)
	if err != nil {
		return nil, false, err
	}

	issuesCount := len(issueSearchResult.Issues)
	if issuesCount == 0 {
		logger.Debug("found no existing issue")
		return nil, false, nil
	}

	if issuesCount > 1 {
		logger.Warn("more than one issue matched, selecting the most recently resolved", "selected_issue", issueSearchResult.Issues[0].Key)
	}

	return &issueSearchResult.Issues[0], false, nil
}

// prepareSearchRequest builds the request body and search path for Jira issue search.
//
// Atlassian announced (see https://developer.atlassian.com/changelog/#CHANGE-2046) that
// the legacy /search endpoint is no longer available on Jira Cloud. The replacement
// endpoint (/rest/api/3/search/jql) is currently not available in Jira Data Center.
//
// Selection logic:
//   - If APIType is "datacenter", always use the v2 /search endpoint.
//   - If APIType is "cloud", or if APIType is "auto" and the host ends with
//     "atlassian.net", use the v3 /search/jql endpoint.
//   - Otherwise (APIType is "auto" without an atlassian.net host),
//     use the v2 /search endpoint.
func (n *Notifier) prepareSearchRequest(jql string) (issueSearch, string) {
	requestBody := issueSearch{
		JQL:        jql,
		MaxResults: 2,
		Fields:     []string{"status"},
	}

	if n.conf.APIType == "datacenter" {
		searchPath := n.conf.APIURL.JoinPath("/search").String()
		return requestBody, searchPath
	}

	if n.conf.APIType == "cloud" || n.conf.APIType == "auto" && strings.HasSuffix(n.conf.APIURL.Host, "atlassian.net") {
		searchPath := strings.Replace(n.conf.APIURL.JoinPath("/search/jql").String(), "/rest/api/2/", "/rest/api/3/", 1)
		return requestBody, searchPath
	}

	searchPath := n.conf.APIURL.JoinPath("/search").String()
	return requestBody, searchPath
}

func (n *Notifier) getIssueTransitionByName(ctx context.Context, issueKey, transitionName string) (string, bool, error) {
	path := fmt.Sprintf("issue/%s/transitions", issueKey)

	responseBody, shouldRetry, err := n.doAPIRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", shouldRetry, err
	}

	var issueTransitions issueTransitions
	err = json.Unmarshal(responseBody, &issueTransitions)
	if err != nil {
		return "", false, err
	}

	for _, issueTransition := range issueTransitions.Transitions {
		if issueTransition.Name == transitionName {
			return issueTransition.ID, false, nil
		}
	}

	return "", false, fmt.Errorf("can't find transition %s for issue %s", transitionName, issueKey)
}

func (n *Notifier) transitionIssue(ctx context.Context, logger *slog.Logger, i *issue, firing bool) (bool, error) {
	if i == nil || i.Key == "" || i.Fields == nil || i.Fields.Status == nil {
		return false, nil
	}

	var transition string
	if firing {
		if i.Fields.Status.StatusCategory.Key != "done" {
			return false, nil
		}

		transition = n.conf.ReopenTransition
	} else {
		if i.Fields.Status.StatusCategory.Key == "done" {
			return false, nil
		}

		transition = n.conf.ResolveTransition
	}

	transitionID, shouldRetry, err := n.getIssueTransitionByName(ctx, i.Key, transition)
	if err != nil {
		return shouldRetry, err
	}

	requestBody := issue{
		Transition: &idNameValue{
			ID: transitionID,
		},
	}

	path := fmt.Sprintf("issue/%s/transitions", i.Key)

	logger.Debug("transitions jira issue", "issue_key", i.Key, "transition", transition)
	_, shouldRetry, err = n.doAPIRequest(ctx, http.MethodPost, path, requestBody)

	return shouldRetry, err
}

func (n *Notifier) doAPIRequest(ctx context.Context, method, path string, requestBody any) ([]byte, bool, error) {
	url := n.conf.APIURL.JoinPath(path)
	return n.doAPIRequestFullPath(ctx, method, url.String(), requestBody)
}

func (n *Notifier) doAPIRequestFullPath(ctx context.Context, method, path string, requestBody any) ([]byte, bool, error) {
	var body io.Reader
	if requestBody != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(requestBody); err != nil {
			return nil, false, err
		}

		body = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, false, err
	}

	defer notify.Drain(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	shouldRetry, err := n.retrier.Check(resp.StatusCode, bytes.NewReader(responseBody))
	if err != nil {
		return nil, shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	return responseBody, false, nil
}

func isAPIv3Path(path string) bool {
	return strings.HasSuffix(strings.TrimRight(path, "/"), "/3")
}
