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

package msteams

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

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	colorRed   = "8C1A1A"
	colorGreen = "2DC72D"
	colorGrey  = "808080"
)

type Notifier struct {
	conf         *config.MSTeamsConfig
	tmpl         *template.Template
	logger       *slog.Logger
	client       *http.Client
	retrier      *notify.Retrier
	webhookURL   *amcommoncfg.SecretURL
	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// Message card reference can be found at https://learn.microsoft.com/en-us/outlook/actionable-messages/message-card-reference.
type teamsMessage struct {
	Context    string `json:"@context"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Text       string `json:"text"`
	ThemeColor string `json:"themeColor"`
}

// New returns a new notifier that uses the Microsoft Teams Webhook API.
func New(c *config.MSTeamsConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "msteams", httpOpts...)
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

	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	data := notify.GetTemplateData(ctx, n.tmpl, as, logger)
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
	summary := tmpl(n.conf.Summary)
	if err != nil {
		return false, err
	}

	alerts := types.Alerts(as...)
	color := colorGrey
	switch alerts.Status() {
	case model.AlertFiring:
		color = colorRed
	case model.AlertResolved:
		color = colorGreen
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

	t := teamsMessage{
		Context:    "http://schema.org/extensions",
		Type:       "MessageCard",
		Title:      title,
		Summary:    summary,
		Text:       text,
		ThemeColor: color,
	}

	var payload bytes.Buffer
	if err = json.NewEncoder(&payload).Encode(t); err != nil {
		return false, err
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
