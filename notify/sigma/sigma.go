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

package sigma

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
)

// Notifier implements a Notifier for generic sigma.
type Notifier struct {
	conf   *config.SigmaConfig
	tmpl   *template.Template
	logger log.Logger
	client *http.Client
}

// New returns a new Sigma.
func New(conf *config.SigmaConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "sigma", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: l,
		client: client,
	}, nil
}

// Request Message defines the JSON object send to Sigma endpoints.
type Request struct {
	Recipient []string       `json:"recipient"`
	Type      string         `json:"type"`
	Payload   RequestPayload `json:"payload,omitempty"`
}

type RequestPayload struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

type Response struct {
	Id      string `json:"id"`
	Status  string `json:"status"`
	Error   int    `json:"error"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	msg := Request{
		Recipient: n.conf.Recipients,
		Type:      n.conf.NotificationType,
		Payload: RequestPayload{
			Sender: n.conf.SenderName,
			Text:   tmplText(n.conf.Text),
		},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	bodyReader := bytes.NewReader(body)

	req, err := http.NewRequest("POST", n.conf.URL.String(), bodyReader)
	if err != nil {
		return false, errors.Wrap(err, "request error")
	}
	req.Header.Set("Authorization", string(n.conf.APIKey))
	req.Header.Set("Content-NotificationType", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return false, err
	}

	r := Response{}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		n.logger.Log("Sigma error. Type: %s; Code: %s; Message: %+v", n.conf.NotificationType, resp.StatusCode, r)
	}

	return false, nil
}
