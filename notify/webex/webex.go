// Copyright 2022 Prometheus Team
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

package webex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	// maxMessageSize represents the maximum message length that Webex supports.
	maxMessageSize = 7439
)

type PreSendHookFunc func(context.Context, Payload, []*types.Alert) (io.Reader, error)

func WithPreSendHook(n *Notifier, err error) func(f PreSendHookFunc) (*Notifier, error) {
	return func(f PreSendHookFunc) (*Notifier, error) {
		if err != nil {
			return n, err
		}
		m := *n
		m.serializeBody = f
		return &m, nil
	}
}

type Notifier struct {
	conf    *config.WebexConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier

	serializeBody PreSendHookFunc
}

// New returns a new Webex notifier.
func New(c *config.WebexConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "webex", httpOpts...)
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{},
		serializeBody: func(_ context.Context, payload Payload, _ []*types.Alert) (io.Reader, error) {
			var buffer bytes.Buffer
			if err := json.NewEncoder(&buffer).Encode(payload); err != nil {
				return nil, err
			}
			return &buffer, nil
		},
	}

	return n, nil
}

type Payload struct {
	Markdown string `json:"markdown"`
	RoomID   string `json:"roomId,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	level.Debug(n.logger).Log("incident", key)

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	message := tmpl(n.conf.Message)
	if err != nil {
		return false, err
	}

	message, truncated := notify.TruncateInBytes(message, maxMessageSize)
	if truncated {
		level.Debug(n.logger).Log("msg", "message truncated due to exceeding maximum allowed length by webex", "truncated_message", message)
	}

	w := Payload{
		Markdown: message,
		RoomID:   n.conf.RoomID,
	}

	var payload io.Reader
	payload, err = n.serializeBody(ctx, w, as)
	if err != nil {
		return false, err
	}

	resp, err := notify.PostJSON(ctx, n.client, n.conf.APIURL.String(), payload)
	if err != nil {
		return true, notify.RedactURL(err)
	}

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, err
	}

	return false, nil
}
