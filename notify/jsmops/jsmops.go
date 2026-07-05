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

package jsmops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// maxMessageLenRunes is the maximum message length in runes for JSM Ops alerts.
// Confirmed against swagger.v3.json: Message schema has "maximum": 130.
const maxMessageLenRunes = 130

// Notifier implements a Notifier for JSM Ops notifications.
type Notifier struct {
	conf    *JSMOpsConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new JSM Ops notifier.
func New(c *JSMOpsConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "jsmops", httpOpts...)
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

type jsmOpsCreateMessage struct {
	Alias       string                         `json:"alias"`
	Message     string                         `json:"message"`
	Description string                         `json:"description,omitempty"`
	Details     map[string]string              `json:"details"`
	Source      string                         `json:"source"`
	Responders  []jsmOpsCreateMessageResponder `json:"responders,omitempty"`
	Tags        []string                       `json:"tags,omitempty"`
	Note        string                         `json:"note,omitempty"`
	Priority    string                         `json:"priority,omitempty"`
	Entity      string                         `json:"entity,omitempty"`
	Actions     []string                       `json:"actions,omitempty"`
}

type jsmOpsCreateMessageResponder struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Type     string `json:"type"`
}

type jsmOpsCloseMessage struct {
	Source string `json:"source"`
}

type jsmOpsUpdateMessageMessage struct {
	Message string `json:"message,omitempty"`
}

type jsmOpsUpdateDescriptionMessage struct {
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

// safeSplit splits a string by a separator, filtering out empty strings.
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

// createRequests builds HTTP requests for a list of alerts.
func (n *Notifier) createRequests(ctx context.Context, as ...*types.Alert) ([]*http.Request, bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return nil, false, err
	}
	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	data := notify.GetTemplateData(ctx, n.tmpl, as, logger)

	tmpl := notify.TmplText(n.tmpl, data, &err)

	details := make(map[string]string)

	maps.Copy(details, data.CommonLabels)

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
		closePath := fmt.Sprintf("%s/close", alias)
		closeURL := n.conf.APIURL.JoinPath(n.conf.CloudID, "v1", "alerts", closePath)
		q := closeURL.Query()
		q.Set("identifierType", "alias")
		closeURL.RawQuery = q.Encode()

		msg := &jsmOpsCloseMessage{Source: tmpl(n.conf.Source)}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(msg); err != nil {
			return nil, false, err
		}
		req, err := http.NewRequest("POST", closeURL.String(), &buf)
		if err != nil {
			return nil, true, err
		}
		requests = append(requests, req.WithContext(ctx))
	default:
		message, truncated := notify.TruncateInRunes(tmpl(n.conf.Message), maxMessageLenRunes)
		if truncated {
			logger.Warn("Truncated message", "alert", key, "max_runes", maxMessageLenRunes)
		}

		createURL := n.conf.APIURL.JoinPath(n.conf.CloudID, "v1", "alerts")

		var responders []jsmOpsCreateMessageResponder
		for _, r := range n.conf.Responders {
			responder := jsmOpsCreateMessageResponder{
				ID:       tmpl(r.ID),
				Name:     tmpl(r.Name),
				Username: tmpl(r.Username),
				Type:     tmpl(r.Type),
			}

			if responder == (jsmOpsCreateMessageResponder{}) {
				// Filter out empty responders. This is useful if you want to fill
				// responders dynamically from alert's common labels.
				continue
			}

			if responder.Type == "teams" {
				teams := safeSplit(responder.Name, ",")
				for _, team := range teams {
					newResponder := jsmOpsCreateMessageResponder{
						Name: tmpl(team),
						Type: tmpl("team"),
					}
					responders = append(responders, newResponder)
				}
				continue
			}

			responders = append(responders, responder)
		}

		msg := &jsmOpsCreateMessage{
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
		req, err := http.NewRequest("POST", createURL.String(), &buf)
		if err != nil {
			return nil, true, err
		}
		requests = append(requests, req.WithContext(ctx))

		if n.conf.UpdateAlerts {
			updateMessageURL := n.conf.APIURL.JoinPath(n.conf.CloudID, "v1", "alerts", alias, "message")
			q := updateMessageURL.Query()
			q.Set("identifierType", "alias")
			updateMessageURL.RawQuery = q.Encode()

			updateMsgMsg := &jsmOpsUpdateMessageMessage{
				Message: msg.Message,
			}
			var updateMessageBuf bytes.Buffer
			if err := json.NewEncoder(&updateMessageBuf).Encode(updateMsgMsg); err != nil {
				return nil, false, err
			}
			req, err := http.NewRequest("PATCH", updateMessageURL.String(), &updateMessageBuf)
			if err != nil {
				return nil, true, err
			}
			requests = append(requests, req)

			updateDescriptionURL := n.conf.APIURL.JoinPath(n.conf.CloudID, "v1", "alerts", alias, "description")
			q = updateDescriptionURL.Query()
			q.Set("identifierType", "alias")
			updateDescriptionURL.RawQuery = q.Encode()

			updateDescMsg := &jsmOpsUpdateDescriptionMessage{
				Description: msg.Description,
			}
			var updateDescriptionBuf bytes.Buffer
			if err := json.NewEncoder(&updateDescriptionBuf).Encode(updateDescMsg); err != nil {
				return nil, false, err
			}
			req, err = http.NewRequest("PATCH", updateDescriptionURL.String(), &updateDescriptionBuf)
			if err != nil {
				return nil, true, err
			}
			requests = append(requests, req.WithContext(ctx))
		}
	}

	if err != nil {
		return nil, false, fmt.Errorf("templating error: %w", err)
	}

	for _, req := range requests {
		req.Header.Set("Content-Type", "application/json")
	}

	return requests, true, nil
}
