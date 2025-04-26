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

package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Notifier implements a Notifier for feishu notifications.
type Notifier struct {
	conf   *config.FeishuConfig
	tmpl   *template.Template
	logger *slog.Logger
	client *http.Client

	authToken   string
	authTokenAt time.Time
}

type feishuMessager struct {
	ReceiveID string `json:"receive_id"`
	MsgType   string `json:"msg_type"`
	Content   string `json:"content"`
	UUID      string `json:"uuid"`
}

type feishuAuthRequest struct {
	APPID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type feishuAuthResponse struct {
	Code              uint   `json:"code"`
	Msg               string `json:"msg"`
	AppAccessToken    string `json:"app_access_token"`
	Expire            int64  `json:"expire"`
	TenantAccessToken string `json:"tenant_access_token"`
}

func New(c *config.FeishuConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "wechat", httpOpts...)
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

	if n.authToken == "" || time.Since(n.authTokenAt) > 2*time.Hour {
		authBody := feishuAuthRequest{
			APPID:     n.conf.APPID,
			AppSecret: tmpl(string(n.conf.APPSecret)),
		}
		authURL := fmt.Sprintf("%s%s", n.conf.APIURL.URL, "/auth/v3/app_access_token/internal")

		var authBuff bytes.Buffer
		if err := json.NewEncoder(&authBuff).Encode(authBody); err != nil {
			return false, err
		}
		resp, err := notify.PostJSON(ctx, n.client, authURL, &authBuff)
		if err != nil {
			return false, err
		}
		defer notify.Drain(resp)

		if resp.StatusCode != 200 {
			return false, nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		var feishuAuthResponse feishuAuthResponse
		if err := json.Unmarshal(body, &feishuAuthResponse); err != nil {
			return false, err
		}
		n.authToken = feishuAuthResponse.TenantAccessToken
	}

	// Create message uuid

	content := map[string]interface{}{
		"text": tmpl(n.conf.Message),
	}
	contentStr, _ := json.Marshal(content)
	msg := &feishuMessager{
		MsgType: "text",
		Content: string(contentStr),
	}

	postMessageURL := n.conf.APIURL.Copy()
	postMessageURL.Path = postMessageURL.Path + "/im/v1/messages"
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", n.authToken),
	}
	// Send message to user
	if n.conf.ToUser != "" {
		url := postMessageURL.Copy()

		q := url.Query()
		q.Set("receive_id_type", "open_id")
		url.RawQuery = q.Encode()

		msg.ReceiveID = n.conf.ToUser
		id, err := uuid.NewUUID()
		msg.UUID = id.String()

		var buff bytes.Buffer
		if err := json.NewEncoder(&buff).Encode(msg); err != nil {
			return true, err
		}
		resp, err := notify.PostTextAddHeaders(ctx, n.client, url.String(), headers, &buff)

		if err != nil {
			return true, err
		}
		defer notify.Drain(resp)
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			return true, errors.New(fmt.Sprintf(`feishu send to open_id response： %s`, string(body)))
		}
	}
	// Send message to group
	if n.conf.ToChat != "" {
		url := postMessageURL.Copy()

		q := url.Query()
		q.Set("receive_id_type", "chat_id")
		url.RawQuery = q.Encode()

		msg.ReceiveID = n.conf.ToChat
		u, err := uuid.NewUUID()
		msg.UUID = u.String()

		var buff bytes.Buffer
		if err := json.NewEncoder(&buff).Encode(msg); err != nil {
			return true, err
		}
		resp, err := notify.PostTextAddHeaders(ctx, n.client, url.String(), headers, &buff)

		if err != nil {
			return true, err
		}
		defer notify.Drain(resp)
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			return true, errors.New(fmt.Sprintf(`feishu send to chat id response： %s`, string(body)))
		}
	}

	return true, nil
}
