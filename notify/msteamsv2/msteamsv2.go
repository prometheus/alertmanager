// Copyright 2024 Prometheus Team
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

package msteamsv2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	colorRed   = "Attention"
	colorGreen = "Good"
	colorGrey  = "Warning"
)

type Notifier struct {
	conf         *config.MSTeamsV2Config
	tmpl         *template.Template
	logger       *slog.Logger
	client       *http.Client
	retrier      *notify.Retrier
	webhookURL   *config.SecretURL
	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

type Action struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// https://learn.microsoft.com/en-us/connectors/teams/?tabs=text1#adaptivecarditemschema
type Content struct {
	Schema  string   `json:"$schema"`
	Type    string   `json:"type"`
	Version string   `json:"version"`
	Body    []Body   `json:"body"`
	Msteams Msteams  `json:"msteams,omitempty"`
	Actions []Action `json:"actions,omitempty"`
}

type Item struct {
	Type   string `json:"type"`
	Weight string `json:"weight,omitempty"`
	Size   string `json:"size,omitempty"`
	Wrap   bool   `json:"wrap,omitempty"`
	Style  string `json:"style,omitempty"`
	Color  string `json:"color,omitempty"`
	Text   string `json:"text"`
}

type Column struct {
	Type  string `json:"type"`
	Width string `json:"width"`
	Items []Item `json:"items"`
}

type Fact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

type Body struct {
	Type    string   `json:"type"`
	Text    string   `json:"text"`
	Weight  string   `json:"weight,omitempty"`
	Size    string   `json:"size,omitempty"`
	Wrap    bool     `json:"wrap,omitempty"`
	Style   string   `json:"style,omitempty"`
	Color   string   `json:"color,omitempty"`
	Columns []Column `json:"columns,omitempty"`
	Facts   []Fact   `json:"facts,omitempty"`
}

type Msteams struct {
	Width string `json:"width"`
}

type Attachment struct {
	ContentType string  `json:"contentType"`
	ContentURL  *string `json:"contentUrl"` // Use a pointer to handle null values
	Content     Content `json:"content"`
}

type teamsMessage struct {
	Type        string       `json:"type"`
	Attachments []Attachment `json:"attachments"`
}

// New returns a new notifier that uses the Microsoft Teams Power Platform connector.
func New(c *config.MSTeamsV2Config, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "msteamsv2", httpOpts...)
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		conf:         c,
		tmpl:         t,
		logger:       l,
		client:       client,
		retrier:      &notify.Retrier{},
		webhookURL:   c.WebhookURL,
		postJSONFunc: notify.PostJSON,
	}

	return n, nil
}

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	n.logger.Debug("extracted group key", "key", key)

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	title := tmpl(n.conf.Title)
	if err != nil {
		return false, err
	}
	text := tmpl(n.conf.Text)
	if err != nil {
		return false, err
	}
	card := tmpl(n.conf.Card)
	if err != nil {
		return false, err
	}

	alerts := types.Alerts(as...)
	// summary := ""
	color := colorGrey
	status := "unknown"
	statusIcon := "⚠"

	switch alerts.Status() {
	case model.AlertFiring:
		color = colorRed
		status = "firing"
		statusIcon = "🔥"
	case model.AlertResolved:
		color = colorGreen
		status = "resolved"
		statusIcon = "✅"
	}

	var url string
	if n.conf.WebhookURL != nil {
		url = n.conf.WebhookURL.String()
	} else {
		content, err := os.ReadFile(n.conf.WebhookURLFile)
		if err != nil {
			return false, fmt.Errorf("read webhook_url_file: %w", err)
		}
		url = strings.TrimSpace(string(content))
	}

	// If the card is empty, use title and text otherwise use card.
	var payload bytes.Buffer
	if card == "" {
		// A message as referenced in https://learn.microsoft.com/en-us/connectors/teams/?tabs=text1%2Cdotnet#request-body-schema
		t := teamsMessage{
			Type: "message",
			Attachments: []Attachment{
				{
					ContentType: "application/vnd.microsoft.card.adaptive",
					ContentURL:  nil,
					Content: Content{
						Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
						Type:    "AdaptiveCard",
						Version: "1.4",
						Msteams: Msteams{
							Width: "Full"},
						Body: []Body{
							{
								Type:  "ColumnSet",
								Style: color,
								Columns: []Column{
									{
										Type:  "Column",
										Width: "stretch",
										Items: []Item{
											{
												Type:   "TextBlock",
												Weight: "Bolder",
												Size:   "ExtraLarge",
												Color:  color,
												Text:   fmt.Sprintf("%s %s", statusIcon, title),
											},
											{
												Type:   "TextBlock",
												Weight: "Bolder",
												Size:   "ExtraLarge",
												Text:   text,
												Wrap:   true,
											},
										},
									},
								},
							},
							{
								Type: "FactSet",
								Facts: []Fact{
									{
										Title: "Status",
										Value: fmt.Sprintf("%s %s", status, statusIcon),
									},
									{
										Title: "Alert",
										Value: extractKV(data.CommonLabels, "alertname"),
									},
									{
										Title: "Summary",
										Value: extractKV(data.CommonAnnotations, "summary"),
									},
									{
										Title: "Severity",
										Value: renderSeverity(data.CommonLabels["severity"]),
									},
									{
										Title: "In Host",
										Value: extractKV(data.CommonLabels, "instance"),
									},
									{
										Title: "Description",
										Value: extractKV(data.CommonAnnotations, "description"),
									},
									{
										Title: "Common Labels",
										Value: renderCommonLabels(data.CommonLabels),
									},
									{
										Title: "Common Annotations",
										Value: renderCommonAnnotations(data.CommonAnnotations),
									},
								},
							},
						},
						Actions: []Action{
							{
								Type:  "Action.OpenUrl",
								Title: "View details",
								URL:   extractKV(data.CommonAnnotations, "runbook_url"),
							},
						},
					},
				},
			},
		}

		// Check if summary exists in CommonLabels

		if err = json.NewEncoder(&payload).Encode(t); err != nil {
			return false, err
		}
	} else {
		// Transform card string into object
		var jsonMap map[string]interface{}
		json.Unmarshal([]byte(card), &jsonMap)
		n.logger.Debug("jsonMap", "jsonMap", jsonMap)

		if err = json.NewEncoder(&payload).Encode(jsonMap); err != nil {
			return false, err
		}
	}

	resp, err := n.postJSONFunc(ctx, n.client, url, &payload)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/connectors-using?tabs=cURL#rate-limiting-for-connectors
	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	return shouldRetry, err
}

func renderSeverity(severity string) string {
	switch severity {
	case "critical":
		return fmt.Sprintf("%s %s", severity, "❌")
	case "error":
		return fmt.Sprintf("%s %s", severity, "❗️")
	case "warning":
		return fmt.Sprintf("%s %s", severity, "⚠️")
	case "info":
		return fmt.Sprintf("%s %s", severity, "ℹ️")
	default:
		return fmt.Sprintf("%s %s", severity, "ℹ❓")
	}
}

func renderCommonLabels(commonLabels template.KV) string {
	removeList := []string{"alertname", "instance", "severity"}

	return commonLabels.Remove(removeList).String()
}

func renderCommonAnnotations(commonLabels template.KV) string {
	removeList := []string{"summary", "description"}

	return commonLabels.Remove(removeList).String()
}

func extractKV(kv template.KV, key string) string {
	if v, ok := kv[key]; ok {
		return v
	}
	return ""
}
