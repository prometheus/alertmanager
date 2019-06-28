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

package msteams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for Microsoft Teams notifications.
type Notifier struct {
	conf   *config.MsTeamsConfig
	tmpl   *template.Template
	logger log.Logger
	client *http.Client
}

// New returns a new MsTeams notification handler.
func New(c *config.MsTeamsConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "msteams")
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:   c,
		tmpl:   t,
		logger: l,
		client: client,
	}, nil
}

// request is the request for sending a MsTeams notification.
type request struct {
	Type       string                   `yaml:"@type,omitempty" json:"@type,omitempty"`
	Context    string                   `yaml:"@context,omitempty" json:"@context,omitempty"`
	ThemeColor string                   `yaml:"themeColor,omitempty" json:"themeColor,omitempty"`
	Summary    string                   `yaml:"summary,omitempty" json:"summary,omitempty"`
	Sections   []config.MsTeamsSections `yaml:"sections,omitempty" json:"sections,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	var numSections = len(n.conf.Sections)
	var secs = make([]config.MsTeamsSections, numSections)
	if numSections > 0 {
		for idx, sec := range n.conf.Sections {
			secs[idx] = config.MsTeamsSections{
				ActivityTitle:    tmplText(sec.ActivityTitle),
				ActivityImage:    tmplText(sec.ActivityImage),
				ActivitySubtitle: tmplText(sec.ActivitySubtitle),
			}
		}
	}

	req := &request{
		Type:       tmplText(n.conf.Type),
		Context:    tmplText(n.conf.Context),
		ThemeColor: tmplText(n.conf.ThemeColor),
		Summary:    tmplText(n.conf.Summary),
		Sections:   secs,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	u := n.conf.APIURL.String()
	resp, err := notify.PostJSON(ctx, n.client, u, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	return n.retry(resp.StatusCode, resp.Body)
}

func (n *Notifier) retry(statusCode int, body io.Reader) (bool, error) {
	if statusCode/100 == 2 {
		return false, nil
	}

	err := fmt.Errorf("unexpected status code %v", statusCode)
	if body != nil {
		if bs, errRead := ioutil.ReadAll(body); errRead == nil {
			err = fmt.Errorf("%s: %q", err, string(bs))
		}
	}
	return statusCode/100 == 5, err
}
