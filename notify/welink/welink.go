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

package welink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
)

// welink supports 500 chars max - from https://open.welink.huaweicloud.com/docs/#/990hh0/whokyc/mmkx2n
const maxMessageLenRunes = 500

type Notifier struct {
	conf    *config.WeLinkConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier

	Token   string
	Channel string
}

type welinkMessage struct {
	Content     welinkContent `yaml:"context" json:"context"`
	MessageType string        `yaml:"messageType" json:"messageType"`
	TimeStamp   int64         `yaml:"timeStamp" json:"timeStamp"`
	UUID        string        `yaml:"uuid" json:"uuid"`
	IsAt        bool          `yaml:"isAt,omitempty" json:"isAt,omitempty"`
	IsAtAll     bool          `yaml:"isAtAll,omitempty" json:"isAtAll,omitempty"`
	AtAccounts  []string      `yaml:"atAccounts,omitempty" json:"atAccounts,omitempty"`
}

type welinkContent struct {
	Text string `json:"text"`
}

type welinkResponse struct {
	Code    string `json:"code"`
	Data    string `json:"data"`
	Message string `json:"message"`
}

func New(c *config.WeLinkConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "welink", httpOpts...)
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

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
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

	// Cache accessToken
	n.Token = n.conf.Token
	n.Channel = n.conf.Channel

	// message lenth
	messageText, truncated := notify.TruncateInRunes(tmpl(n.conf.Content.Text), maxMessageLenRunes)
	if truncated {
		level.Warn(n.logger).Log("msg", "Truncated message", "alert", key, "max_runes", maxMessageLenRunes)
	}

	// init message
	msg := &welinkMessage{
		MessageType: n.conf.MessageType,
		UUID:        uuid.New().String(),
		TimeStamp:   time.Now().UnixMilli(), // microseconds
		Content: welinkContent{
			Text: messageText,
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	postMessageURL := n.conf.APIUrl.Copy()
	postMessageURL.Path += "api/werobot/v1/webhook/send"
	q := postMessageURL.Query()
	q.Set("token", n.Token)
	q.Set("channel", n.Channel)
	postMessageURL.RawQuery = q.Encode()

	resp, err := notify.PostJSON(ctx, n.client, postMessageURL.String(), &buf)
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
	level.Debug(n.logger).Log("response", string(body), "incident", key)

	var weResp welinkResponse
	if err := json.Unmarshal(body, &weResp); err != nil {
		return true, err
	}

	// See: https://open.welink.huaweicloud.com/docs/#/990hh0/whokyc/mmkx2n
	// 0	    服务正常
	// 58404	机器人资源不存在
	// 58500	服务异常
	// 58601	参数错误
	// 58602	机器人未启用
	if weResp.Code == "0" {
		return false, nil
	}

	return false, errors.New(weResp.Message)
}
