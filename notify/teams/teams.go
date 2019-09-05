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

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for Teams notifications.
type Notifier struct {
	conf    *config.TeamsConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new Teams notification handler.
func New(c *config.TeamsConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "teams", false)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{},
	}, nil
}

type card struct {
	Context    string    `json:"@context,omitempty"`
	Type       string    `json:"@type,omitempty"`
	Title      string    `json:"title,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	ThemeColor string    `json:"themeColor,omitempty"`
	Sections   []section `json:"sections"`
	Actions    []action  `json:"potentialAction"`
}

type section struct {
	ActivityTitle    string `json:"activityTitle,omitempty"`
	ActivitySubtitle string `json:"activitySubtitle,omitempty"`
	ActivityImage    string `json:"activityImage,omitempty"`
	Text             string `json:"text,omitempty"`
}

type action struct {
	Type    string   `json:"@type,omitempty"`
	Name    string   `json:"name,omitempty"`
	Targets []target `json:"targets"`
}

type target struct {
	Os  string `json:"os,omitempty"`
	Uri string `json:"uri,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	t := &target{
		Os:  "default",
		Uri: tmplText(n.conf.Link),
	}
	a := &action{
		Type:    "OpenUri",
		Name:    "View in AlertManager",
		Targets: []target{*t},
	}
	req := &card{
		Context:    "http://schema.org/extensions",
		Type:       "MessageCard",
		ThemeColor: "0078D7",
		Title:      tmplText(n.conf.Title),
		Summary:    tmplText(n.conf.Summary),
		Actions:    []action{*a},
	}
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	u := n.conf.WEBHOOKURL.String()
	resp, err := notify.PostJSON(ctx, n.client, u, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// Only 5xx response codes are recoverable and 2xx codes are successful.
	// https://api.slack.com/incoming-webhooks#handling_errors
	// https://api.slack.com/changelog/2016-05-17-changes-to-errors-for-incoming-webhooks
	return n.retrier.Check(resp.StatusCode, resp.Body)
}
