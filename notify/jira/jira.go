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
	"github.com/trivago/tgo/tcontainer"

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
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "jira", httpOpts...)
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

		logger.Debug("updating existing issue", "issue_key", existingIssue.Key)
	}

	requestBody, err := n.prepareIssueRequestBody(ctx, logger, key.Hash(), tmplTextFunc)
	if err != nil {
		return false, err
	}

	_, shouldRetry, err = n.doAPIRequest(ctx, method, path, requestBody)
	if err != nil {
		return shouldRetry, fmt.Errorf("failed to %s request to %q: %w", method, path, err)
	}

	return n.transitionIssue(ctx, logger, existingIssue, alerts.HasFiring())
}

func (n *Notifier) prepareIssueRequestBody(_ context.Context, logger *slog.Logger, groupID string, tmplTextFunc templateFunc) (issue, error) {
	summary, err := tmplTextFunc(n.conf.Summary)
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

	// Recursively convert any maps to map[string]interface{}, filtering out all non-string keys, so the json encoder
	// doesn't blow up when marshaling JIRA requests.
	fieldsWithStringKeys, err := tcontainer.ConvertToMarshalMap(n.conf.Fields, func(v string) string { return v })
	if err != nil {
		return issue{}, fmt.Errorf("convertToMarshalMap: %w", err)
	}

	summary, truncated := notify.TruncateInRunes(summary, maxSummaryLenRunes)
	if truncated {
		logger.Warn("Truncated summary", "max_runes", maxSummaryLenRunes)
	}

	requestBody := issue{Fields: &issueFields{
		Project:   &issueProject{Key: project},
		Issuetype: &idNameValue{Name: issueType},
		Summary:   summary,
		Labels:    make([]string, 0, len(n.conf.Labels)+1),
		Fields:    fieldsWithStringKeys,
	}}

	issueDescriptionString, err := tmplTextFunc(n.conf.Description)
	if err != nil {
		return issue{}, fmt.Errorf("description template: %w", err)
	}

	issueDescriptionString, truncated = notify.TruncateInRunes(issueDescriptionString, maxDescriptionLenRunes)
	if truncated {
		logger.Warn("Truncated description", "max_runes", maxDescriptionLenRunes)
	}

	requestBody.Fields.Description = issueDescriptionString
	if strings.HasSuffix(n.conf.APIURL.Path, "/3") {
		var issueDescription any
		if err := json.Unmarshal([]byte(issueDescriptionString), &issueDescription); err != nil {
			return issue{}, fmt.Errorf("description unmarshaling: %w", err)
		}
		requestBody.Fields.Description = issueDescription
	}

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

func (n *Notifier) searchExistingIssue(ctx context.Context, logger *slog.Logger, groupID string, firing bool, tmplTextFunc templateFunc) (*issue, bool, error) {
	jql := strings.Builder{}

	if n.conf.WontFixResolution != "" {
		jql.WriteString(fmt.Sprintf(`resolution != %q and `, n.conf.WontFixResolution))
	}

	// If the group is firing, search for open issues. If a reopen transition is
	// defined, also search for issues that were closed within the reopen duration.
	if firing {
		reopenDuration := int64(time.Duration(n.conf.ReopenDuration).Minutes())
		if n.conf.ReopenTransition != "" && reopenDuration > 0 {
			jql.WriteString(fmt.Sprintf(`(resolutiondate is EMPTY OR resolutiondate >= -%dm) and `, reopenDuration))
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
	jql.WriteString(fmt.Sprintf(`project=%q and labels=%q order by status ASC,resolutiondate DESC`, project, alertLabel))

	requestBody := issueSearch{
		JQL:        jql.String(),
		MaxResults: 2,
		Fields:     []string{"status"},
		Expand:     []string{},
	}

	logger.Debug("search for recent issues", "jql", requestBody.JQL)

	responseBody, shouldRetry, err := n.doAPIRequest(ctx, http.MethodPost, "search", requestBody)
	if err != nil {
		return nil, shouldRetry, fmt.Errorf("HTTP request to JIRA API: %w", err)
	}

	var issueSearchResult issueSearchResult
	err = json.Unmarshal(responseBody, &issueSearchResult)
	if err != nil {
		return nil, false, err
	}

	if issueSearchResult.Total == 0 {
		logger.Debug("found no existing issue")
		return nil, false, nil
	}

	if issueSearchResult.Total > 1 {
		logger.Warn("more than one issue matched, selecting the most recently resolved", "selected_issue", issueSearchResult.Issues[0].Key)
	}

	return &issueSearchResult.Issues[0], false, nil
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
	var body io.Reader
	if requestBody != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(requestBody); err != nil {
			return nil, false, err
		}

		body = &buf
	}

	url := n.conf.APIURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, url.String(), body)
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
