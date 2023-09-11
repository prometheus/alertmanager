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

package jsm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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

const maxMessageLenRunes = 130

// Notifier implements a Notifier for JSM notifications.
type Notifier struct {
	conf    *config.JSMConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new JSM notifier.
func New(c *config.JSMConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "jsm", httpOpts...)
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

type jsmCreateMessage struct {
	Alias       string                           `json:"alias"`
	Message     string                           `json:"message"`
	Description string                           `json:"description,omitempty"`
	Details     map[string]string                `json:"details"`
	Source      string                           `json:"source"`
	Responders  []jsmCreateMessageResponder `json:"responders,omitempty"`
	Tags        []string                         `json:"tags,omitempty"`
	Note        string                           `json:"note,omitempty"`
	Priority    string                           `json:"priority,omitempty"`
	Entity      string                           `json:"entity,omitempty"`
	Actions     []string                         `json:"actions,omitempty"`
}

type jsmCreateMessageResponder struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Type     string `json:"type"` // team, user, escalation, schedule etc.
}

type jsmCloseMessage struct {
	Source string `json:"source"`
}

type jsmUpdateMessageMessage struct {
	Message string `json:"message,omitempty"`
}

type jsmUpdateDescriptionMessage struct {
	Description string `json:"description,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	requests, retry, err := n.createRequests(ctx, as...)
	if err != nil {
		return retry, err
	}

	for _, req := range requests {
		req.Header.Set("User-Agent", notify.UserAgentHeader)
		resp, err := n.client.Do(req)
		if err != nil {
			return true, err
		}
		shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
		notify.Drain(resp)
		if err != nil {
			return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
		}
	}
	return true, nil
}

// Like Split but filter out empty strings.
func safeSplit(s, sep string) []string {
	a := strings.Split(strings.TrimSpace(s), sep)
	b := a[:0]
	for _, x := range a {
		if x != "" {
			b = append(b, x)
		}
	}
	return b
}

// Create requests for a list of alerts.
func (n *Notifier) createRequests(ctx context.Context, as ...*types.Alert) ([]*http.Request, bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return nil, false, err
	}
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)

	level.Debug(n.logger).Log("alert", key)

	tmpl := notify.TmplText(n.tmpl, data, &err)

	details := make(map[string]string)

	for k, v := range data.CommonLabels {
		details[k] = v
	}

	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	requests := []*http.Request{}

	var (
		alias  = key.Hash()
		alerts = types.Alerts(as...)
	)
	switch alerts.Status() {
	case model.AlertResolved:
		resolvedEndpointURL := n.conf.APIURL.Copy()
		resolvedEndpointURL.Path += fmt.Sprintf("v2/alerts/%s/close", alias)
		q := resolvedEndpointURL.Query()
		q.Set("identifierType", "alias")
		resolvedEndpointURL.RawQuery = q.Encode()
		msg := &jsmCloseMessage{Source: tmpl(n.conf.Source)}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(msg); err != nil {
			return nil, false, err
		}
		req, err := http.NewRequest("POST", resolvedEndpointURL.String(), &buf)
		if err != nil {
			return nil, true, err
		}
		requests = append(requests, req.WithContext(ctx))
	default:
		message, truncated := notify.TruncateInRunes(tmpl(n.conf.Message), maxMessageLenRunes)
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated message", "alert", key, "max_runes", maxMessageLenRunes)
		}

		createEndpointURL := n.conf.APIURL.Copy()
		createEndpointURL.Path += "v2/alerts"

		var responders []jsmCreateMessageResponder
		for _, r := range n.conf.Responders {
			responder := jsmCreateMessageResponder{
				ID:       tmpl(r.ID),
				Name:     tmpl(r.Name),
				Username: tmpl(r.Username),
				Type:     tmpl(r.Type),
			}

			if responder == (jsmCreateMessageResponder{}) {
				// Filter out empty responders. This is useful if you want to fill
				// responders dynamically from alert's common labels.
				continue
			}

			if responder.Type == "teams" {
				teams := safeSplit(responder.Name, ",")
				for _, team := range teams {
					newResponder := jsmCreateMessageResponder{
						Name: tmpl(team),
						Type: tmpl("team"),
					}
					responders = append(responders, newResponder)
				}
				continue
			}

			responders = append(responders, responder)
		}

		msg := &jsmCreateMessage{
			Alias:       alias,
			Message:     message,
			Description: tmpl(n.conf.Description),
			Details:     details,
			Source:      tmpl(n.conf.Source),
			Responders:  responders,
			Tags:        safeSplit(tmpl(n.conf.Tags), ","),
			Note:        tmpl(n.conf.Note),
			Priority:    tmpl(n.conf.Priority),
			Entity:      tmpl(n.conf.Entity),
			Actions:     safeSplit(tmpl(n.conf.Actions), ","),
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(msg); err != nil {
			return nil, false, err
		}
		req, err := http.NewRequest("POST", createEndpointURL.String(), &buf)
		if err != nil {
			return nil, true, err
		}
		requests = append(requests, req.WithContext(ctx))

		if n.conf.UpdateAlerts {
			updateMessageEndpointURL := n.conf.APIURL.Copy()
			updateMessageEndpointURL.Path += fmt.Sprintf("v2/alerts/%s/message", alias)
			q := updateMessageEndpointURL.Query()
			q.Set("identifierType", "alias")
			updateMessageEndpointURL.RawQuery = q.Encode()
			updateMsgMsg := &jsmUpdateMessageMessage{
				Message: msg.Message,
			}
			var updateMessageBuf bytes.Buffer
			if err := json.NewEncoder(&updateMessageBuf).Encode(updateMsgMsg); err != nil {
				return nil, false, err
			}
			req, err := http.NewRequest("PUT", updateMessageEndpointURL.String(), &updateMessageBuf)
			if err != nil {
				return nil, true, err
			}
			requests = append(requests, req)

			updateDescriptionEndpointURL := n.conf.APIURL.Copy()
			updateDescriptionEndpointURL.Path += fmt.Sprintf("v2/alerts/%s/description", alias)
			q = updateDescriptionEndpointURL.Query()
			q.Set("identifierType", "alias")
			updateDescriptionEndpointURL.RawQuery = q.Encode()
			updateDescMsg := &jsmUpdateDescriptionMessage{
				Description: msg.Description,
			}

			var updateDescriptionBuf bytes.Buffer
			if err := json.NewEncoder(&updateDescriptionBuf).Encode(updateDescMsg); err != nil {
				return nil, false, err
			}
			req, err = http.NewRequest("PUT", updateDescriptionEndpointURL.String(), &updateDescriptionBuf)
			if err != nil {
				return nil, true, err
			}
			requests = append(requests, req.WithContext(ctx))
		}
	}

	var apiKey string
	if n.conf.APIKey != "" {
		apiKey = tmpl(string(n.conf.APIKey))
	} else {
		content, err := os.ReadFile(n.conf.APIKeyFile)
		if err != nil {
			return nil, false, errors.Wrap(err, "read key_file error")
		}
		apiKey = tmpl(string(content))
	}

	if err != nil {
		return nil, false, errors.Wrap(err, "templating error")
	}

	for _, req := range requests {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("GenieKey %s", apiKey))
	}

	return requests, true, nil
}
