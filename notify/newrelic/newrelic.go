// Copyright 2021 Prometheus Team
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

package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for NewRelic notifications.
type Notifier struct {
	conf    *config.NewRelicConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new NewRelic notifier.
func New(c *config.NewRelicConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "newrelic", append(httpOpts, commoncfg.WithHTTP2Disabled())...)
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

type NewRelicCreateMessage struct {
	NriDomain   string            `json:"nridomain"`
	Alias       string            `json:"alias"`
	Message     string            `json:"message"`
	Description string            `json:"description,omitempty"`
	Details     map[string]string `json:"details"`
	Source      string            `json:"source"`
	Tags        []string          `json:"tags,omitempty"`
	Note        string            `json:"note,omitempty"`
	Priority    string            `json:"priority,omitempty"`
}

type NewRelicCloseMessage struct {
	Source string `json:"source"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	req, retry, err := n.createRequest(ctx, as...)
	if err != nil {
		return retry, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return true, err
	}
	defer notify.Drain(resp)

	return n.retrier.Check(resp.StatusCode, resp.Body)
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
func (n *Notifier) createRequest(ctx context.Context, as ...*types.Alert) (*http.Request, bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return nil, false, err
	}
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	level.Debug(n.logger).Log("incident", key)

	tmpl := notify.TmplText(n.tmpl, data, &err)

	details := make(map[string]string)

	for k, v := range data.CommonLabels {
		details[k] = v
	}

	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}
	var (
		msg    interface{}
		apiURL = n.conf.APIURL.Copy()
		alias  = key.Hash()
	)

	message, truncated := notify.TruncateInRunes(tmpl(n.conf.Message), 130)
	if truncated {
		level.Debug(n.logger).Log("msg", "truncated message", "truncated_message", message, "incident", key)
	}
	msg = &NewRelicCreateMessage{
		NriDomain:   "NewRelicReceiver",
		Alias:       alias,
		Message:     message,
		Description: tmpl(n.conf.Description),
		Details:     details,
		Source:      tmpl(n.conf.Source),
		Tags:        safeSplit(string(tmpl(n.conf.Tags)), ","),
		Note:        tmpl(n.conf.Note),
		Priority:    tmpl(n.conf.Priority),
	}

	if err != nil {
		return nil, false, errors.Wrap(err, "templating error")
	}

	apiKey := tmpl(string(n.conf.APIKey))

	if err != nil {
		return nil, false, errors.Wrap(err, "templating error")
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest("POST", apiURL.String(), &buf)
	if err != nil {
		return nil, true, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", apiKey)

	return req.WithContext(ctx), true, nil
}
