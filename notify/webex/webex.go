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
	"github.com/prometheus/alertmanager/types"
	"io"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

const (
	// nolint:godot
	// maxMessageSize represents the maximum message length that Webex supports.
	maxMessageSize = 7439
)

type Notifier struct {
	conf    *config.WebexConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
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
	}

	return n, nil
}

type webhook struct {
	Markdown string `json:"markdown"`
	RoomID   string `json:"roomId,omitempty"`
	ParentId string `json:"parentId,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (context.Context, bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return ctx, false, err
	}

	level.Debug(n.logger).Log("incident", key)

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return ctx, false, err
	}

	message := tmpl(n.conf.Message)
	if err != nil {
		return ctx, false, err
	}

	message, truncated := notify.TruncateInBytes(message, maxMessageSize)
	if truncated {
		level.Debug(n.logger).Log("msg", "message truncated due to exceeding maximum allowed length by webex", "truncated_message", message)
	}

	w := webhook{
		Markdown: message,
		RoomID:   tmpl(n.conf.RoomID),
	}

	ctx = notify.WithThreaded(ctx, n.conf.Threaded)

	handleResp := func(resp *http.Response) (context.Context, error) { return ctx, nil }

	if n.conf.Threaded {
		keyId := "id"
		if id, ok := notify.ThreadedStateKV(ctx, keyId); ok {
			w.ParentId = id
		} else {
			handleResp = func(resp *http.Response) (context.Context, error) {
				jsonStr, err := io.ReadAll(resp.Body)
				if err != nil {
					return ctx, err
				}
				var res struct {
					Id string `json:"id"`
				}
				if err = json.Unmarshal(jsonStr, &res); err != nil {
					return ctx, err
				}
				ctx = notify.WithThreadedStateKV(ctx, keyId, res.Id)
				return ctx, nil
			}
		}
	}

	var payload bytes.Buffer
	if err = json.NewEncoder(&payload).Encode(w); err != nil {
		return ctx, false, err
	}

	resp, err := notify.PostJSON(ctx, n.client, n.conf.APIURL.String(), &payload)
	if err != nil {
		return ctx, true, notify.RedactURL(err)
	}

	ctx, err = handleResp(resp)
	if err != nil {
		return ctx, false, err
	}

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return ctx, shouldRetry, err
	}

	return ctx, false, nil
}
