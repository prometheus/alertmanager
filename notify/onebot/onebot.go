// Copyright 2025 Prometheus Team
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

package onebot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
)

// Notifier implements a Notifier for wechat notifications.
type Notifier struct {
	conf   *config.OnebotConfig
	tmpl   *template.Template
	logger *slog.Logger
	client *http.Client
}

// New returns a new Wechat notifier.
func New(c *config.OnebotConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "onebot", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
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
	toUser := tmpl(n.conf.ToUser)
	if err != nil {
		return false, err
	}
	toParty := tmpl(n.conf.ToParty)
	if err != nil {
		return false, err
	}
	msg := tmpl(n.conf.Message)
	if err != nil {
		return false, err
	}
	var messages []map[string]any
	if msg == "" {
		return false, errors.New("message is empty")
	}
	if n.conf.MessageType == "raw" {
		if err := json.Unmarshal([]byte(msg), &messages); err != nil {
			return false, err
		}
	} else {
		messages = append(messages, map[string]any{
			"type": "text",
			"data": map[string]any{
				"text": msg,
			},
		})
	}

	var resp *http.Response
	if toUser != "" {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(map[string]any{
			"user_id": toUser,
			"message": messages,
		}); err != nil {
			return false, err
		}
		url := n.conf.APIURL.Copy()
		url.Path = strings.TrimSuffix(url.Path, "/") + "/send_private_msg"
		resp, err = notify.PostJSON(ctx, n.client, url.String(), &buf)
	} else if toParty != "" {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(map[string]any{
			"group_id": toParty,
			"message":  messages,
		}); err != nil {
			return false, err
		}
		url := n.conf.APIURL.Copy()
		url.Path = strings.TrimSuffix(url.Path, "/") + "/send_group_msg"
		resp, err = notify.PostJSON(ctx, n.client, url.String(), &buf)
	} else {
		return false, errors.New("no group_id or to_user specified")
	}
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)
	if resp.StatusCode != 200 {
		return true, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), fmt.Errorf("unexpected status code %v", resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, err
	}
	n.logger.Debug(string(body), "incident", key)
	var onebotResp map[string]any
	if err := json.Unmarshal(body, &onebotResp); err != nil {
		return true, err
	}
	if onebotResp["status"] == "ok" {
		return false, nil
	}
	return false, errors.New(fmt.Sprintf("%v", onebotResp["message"]))
}
