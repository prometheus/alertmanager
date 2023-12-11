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
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for JIRA notifications.
type Notifier struct {
	conf    *config.JiraConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

func New(c *config.JiraConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
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

	level.Debug(n.logger).Log("alert", key)

	var (
		tmplTextErr error

		alerts       = types.Alerts(as...)
		data         = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText     = notify.TmplText(n.tmpl, data, &tmplTextErr)
		tmplTextFunc = func(tmpl string) (string, error) {
			result := tmplText(tmpl)
			return result, tmplTextErr
		}
	)

	existingIssue, shouldRetry, err := n.searchExistingIssue(tmplTextFunc, key, alerts.Status())
	if err != nil {
		return shouldRetry, err
	}

	// Do not create new issues for resolved alerts
	if existingIssue == nil && alerts.Status() == model.AlertResolved {
		return false, nil
	}

	path := "issue"
	method := http.MethodPost

	if existingIssue == nil {
		level.Debug(n.logger).Log("msg", "create new issue", "alert", key.String())
	} else {
		level.Debug(n.logger).Log("msg", "updating existing issue", "key", existingIssue.Key, "alert", key.String())

		path += "/" + existingIssue.Key
		method = http.MethodPut
	}

	requestBody, err := n.prepareIssueRequestBody(tmplTextFunc)
	if err != nil {
		return false, err
	}

	requestBody.Fields.Labels = append(requestBody.Fields.Labels, key.Hash())

	for _, labelKey := range n.conf.GroupLabels {
		if val, ok := data.GroupLabels[labelKey]; ok {
			requestBody.Fields.Labels = append(requestBody.Fields.Labels, val)
		}
	}

	_, shouldRetry, err = n.doAPIRequest(tmplTextFunc, method, path, requestBody)
	if err != nil {
		return shouldRetry, err
	}

	if existingIssue != nil {
		if n.conf.ResolveTransition != "" && alerts.Status() == model.AlertResolved && existingIssue.Fields.Status.StatusCategory.Key != "done" {
			return n.TransitionIssue(tmplTextFunc, key, existingIssue.Key, n.conf.ResolveTransition)
		} else if n.conf.ReopenTransition != "" && alerts.Status() == model.AlertFiring && existingIssue.Fields.Status.StatusCategory.Key == "done" {
			return n.TransitionIssue(tmplTextFunc, key, existingIssue.Key, n.conf.ReopenTransition)
		}
	}

	return false, nil
}

func (n *Notifier) prepareIssueRequestBody(tmplTextFunc templateFunc) (issue, error) {
	summary, err := tmplTextFunc(n.conf.Summary)
	if err != nil {
		return issue{}, errors.Wrap(err, "template error")
	}

	requestBody := issue{Fields: &issueFields{
		Project:      &issueProject{Key: n.conf.Project},
		Issuetype:    &idNameValue{Name: n.conf.IssueType},
		Summary:      summary,
		Labels:       make([]string, 1+len(n.conf.StaticLabels)),
		CustomFields: n.conf.CustomFields,
	}}

	issueDescriptionString, err := tmplTextFunc(n.conf.Summary)
	if err != nil {
		return issue{}, errors.Wrap(err, "template error")
	}

	if strings.HasSuffix(n.conf.APIURL.Path, "/3") {
		var issueDescription any
		if err := json.Unmarshal([]byte(issueDescriptionString), &issueDescription); err != nil {
			return issue{}, nil
		}
		requestBody.Fields.Description = issueDescription
	} else {
		requestBody.Fields.Description = issueDescriptionString
	}

	if n.conf.StaticLabels != nil {
		requestBody.Fields.Labels = n.conf.StaticLabels
	}

	if n.conf.Components != nil {
		requestBody.Fields.Components = make([]idNameValue, len(n.conf.Components))

		for i, components := range n.conf.Components {
			requestBody.Fields.Components[i] = idNameValue{Name: components}
		}
	}

	priority, err := tmplTextFunc(n.conf.Priority)
	if err != nil {
		return issue{}, errors.Wrap(err, "template error")
	}

	if priority != "" {
		requestBody.Fields.Priority = &idNameValue{Name: priority}
	}

	sort.Strings(requestBody.Fields.Labels)

	return requestBody, nil
}

func (n *Notifier) searchExistingIssue(tmplTextFunc templateFunc, key notify.Key, status model.AlertStatus) (*issue, bool, error) {
	jql := strings.Builder{}

	if n.conf.WontFixResolution != "" {
		jql.WriteString(fmt.Sprintf(`resolution != "%s" and `, n.conf.WontFixResolution))
	}

	// if alert is firing, do not search for closed issues unless reopen transition is defined.
	if n.conf.ReopenTransition == "" {
		if status != model.AlertResolved {
			jql.WriteString(`statusCategory != Done and `)
		}
	} else {
		reopenDuration := int64(time.Duration(n.conf.ReopenDuration).Minutes())
		if reopenDuration != 0 {
			jql.WriteString(fmt.Sprintf(`(resolutiondate is EMPTY OR resolutiondate >= -%dm) and `, reopenDuration))
		}
	}

	jql.WriteString(fmt.Sprintf(`project = "%s" and labels=%q order by status ASC,resolutiondate DESC`, n.conf.Project, key.Hash()))

	requestBody := issueSearch{}
	requestBody.Jql = jql.String()
	requestBody.MaxResults = 2
	requestBody.Fields = []string{"status"}
	requestBody.Expand = []string{}

	level.Debug(n.logger).Log("msg", "search for recent issues", "alert", key.String(), "jql", jql.String())

	responseBody, shouldRetry, err := n.doAPIRequest(tmplTextFunc, http.MethodPost, "search", requestBody)
	if err != nil {
		return nil, shouldRetry, err
	}

	var issueSearchResult issueSearchResult
	err = json.Unmarshal(responseBody, &issueSearchResult)
	if err != nil {
		return nil, false, err
	}

	switch issueSearchResult.Total {
	case 0:
		level.Debug(n.logger).Log("msg", "found no existing issue", "alert", key.String())
		return nil, false, nil
	default:
		level.Warn(n.logger).Log("msg", "more than one issue matched, picking most recently resolved", "alert", key.String(), "picked", issueSearchResult.Issues[0].Key)

		fallthrough
	case 1:
		return &issueSearchResult.Issues[0], false, nil
	}
}

func (n *Notifier) getIssueTransitionByName(tmplTextFunc templateFunc, issueKey, transitionName string) (string, bool, error) {
	path := fmt.Sprintf("issue/%s/transitions", issueKey)

	responseBody, shouldRetry, err := n.doAPIRequest(tmplTextFunc, http.MethodGet, path, nil)
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

func (n *Notifier) TransitionIssue(tmplText templateFunc, key notify.Key, issueKey, transitionName string) (bool, error) {
	transitionID, shouldRetry, err := n.getIssueTransitionByName(tmplText, issueKey, transitionName)
	if err != nil {
		return shouldRetry, err
	}

	requestBody := issue{}
	requestBody.Transition = &idNameValue{ID: transitionID}

	path := fmt.Sprintf("issue/%s/transitions", issueKey)

	level.Debug(n.logger).Log("msg", "transitions jira issue", "alert", key.String(), "key", issueKey, "transition", transitionName)
	_, shouldRetry, err = n.doAPIRequest(tmplText, http.MethodPost, path, requestBody)

	return shouldRetry, err
}

func (n *Notifier) doAPIRequest(tmplTextFunc templateFunc, method, path string, requestBody any) ([]byte, bool, error) {
	var body io.Reader
	if requestBody != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(requestBody); err != nil {
			return nil, false, err
		}

		body = &buf
	}

	url := n.conf.APIURL.Copy()
	url.Path += path

	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return nil, false, err
	}

	var token string
	if n.conf.APIToken != "" {
		token, err = tmplTextFunc(string(n.conf.APIToken))
		if err != nil {
			return nil, false, errors.Wrap(err, "template error")
		}
	} else {
		content, err := os.ReadFile(n.conf.APITokenFile)
		if err != nil {
			return nil, false, errors.Wrap(err, "read key_file error")
		}
		token, err = tmplTextFunc(string(content))
		if err != nil {
			return nil, false, errors.Wrap(err, "template error")
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("X-Force-Accept-Language", "true")
	req.SetBasicAuth(n.conf.APIUsername, strings.TrimSpace(token))

	resp, err := n.client.Do(req)
	defer notify.Drain(resp)

	if err != nil {
		return nil, false, err
	}

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
