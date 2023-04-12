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

package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

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
	conf       *config.TeamsConfig
	tmpl       *template.Template
	logger     log.Logger
	client     *http.Client
	retrier    *notify.Retrier
	webhookURL *config.SecretURL
}

// Message card https://learn.microsoft.com/en-us/outlook/actionable-messages/message-card-reference
type teamsMessage struct {
	Context    string `json:"@context"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	ThemeColor string `json:"themeColor"`
}

func New(c *config.TeamsConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "teams", httpOpts...)
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		conf:       c,
		tmpl:       t,
		logger:     l,
		client:     client,
		retrier:    &notify.Retrier{},
		webhookURL: c.WebhookURL,
	}

	return n, nil
}

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	level.Debug(n.logger).Log("incident", key)

	alerts := types.Alerts(as...)
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

	color := colorGrey
	if alerts.Status() == model.AlertFiring {
		color = colorRed
	}
	if alerts.Status() == model.AlertResolved {
		color = colorGreen
	}

	t := teamsMessage{
		Context:    "http://schema.org/extensions",
		Type:       "MessageCard",
		Title:      title,
		Text:       text,
		ThemeColor: color,
	}

	var payload bytes.Buffer
	if err = json.NewEncoder(&payload).Encode(t); err != nil {
		return false, err
	}

	resp, err := notify.PostJSON(ctx, n.client, n.webhookURL.String(), &payload)
	if err != nil {
		return true, notify.RedactURL(err)
	}

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, err
	}
	return false, nil
}
