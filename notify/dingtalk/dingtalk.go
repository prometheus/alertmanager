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

package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notifier implements a Notifier for dingtalk notifications.
type Notifier struct {
	conf   *config.DingTalkConfig
	tmpl   *template.Template
	logger log.Logger
	client *http.Client
}

type dingtalkMessageContent struct {
	Content string `json:"content"`
}

type dingtalkMessage struct {
	Text dingtalkMessageContent `yaml:"text,omitempty" json:"text,omitempty"`
	ID   string                 `yaml:"chatid,omitempty" json:"chatid,omitempty"`
	Type string                 `yaml:"msgtype,omitempty" json:"msgtype,omitempty"`
}

type response struct {
	Code    int    `json:"errcode"`
	Message string `json:"errmsg"`
	Token   string `json:"access_token"`
	Status  int    `json:"status"`
	Punish  string `json:"punish"`
}

// New returns a new DingTalk notifier.
func New(c *config.DingTalkConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "dingtalk", false, false)
	if err != nil {
		return nil, err
	}

	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
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

	content := tmpl(n.conf.Message)

	// If the dingtalk chatbot required keywords security authenticate. add the keywords to the content.
	if n.conf.Keywords != nil && len(n.conf.Keywords) > 0 {
		keywords := "\n\n[Keywords] "
		for _, k := range n.conf.Keywords {
			keywords = fmt.Sprintf("%s%s, ", keywords, k)
		}

		keywords = strings.TrimSuffix(keywords, ", ")
		content = fmt.Sprintf("%s%s", content, keywords)
	}

	msg := &dingtalkMessage{
		Type: "text",
		Text: dingtalkMessageContent{
			Content: content,
		},
	}

	if err != nil {
		return false, fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	webhook, err := url.Parse(n.conf.Webhook.String())
	if err != nil {
		return false, err
	}

	postMessageURL := config.URL{
		URL: webhook,
	}

	// If the dingtalk chatbot required signature security authenticate,
	// add signature and timestamp to the url.
	if len(n.conf.Secret) > 0 {
		timestamp, sign, err := calcSign(string(n.conf.Secret))
		if err != nil {
			return false, err
		}

		q := postMessageURL.Query()
		q.Set("timestamp", timestamp)
		q.Set("sign", sign)
		postMessageURL.RawQuery = q.Encode()
	}

	resp, err := notify.PostJSON(ctx, n.client, postMessageURL.String(), &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	if resp.StatusCode != 200 {
		return true, fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return true, err
	}
	level.Debug(n.logger).Log("response", string(body), "incident", key)

	var dingResp response
	if err := json.Unmarshal(body, &dingResp); err != nil {
		return true, err
	}

	if dingResp.Code == 0 {
		return false, nil
	}

	// Exceed the active call frequency limit.
	if dingResp.Status != 0 {
		return false, errors.New(dingResp.Punish)
	}

	return false, errors.New(dingResp.Message)
}

func calcSign(secret string) (string, string, error) {

	timestamp := fmt.Sprintf("%d", time.Now().Unix()*1000)
	msg := fmt.Sprintf("%s\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write([]byte(msg))
	if err != nil {
		return "", "", err
	}
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return timestamp, url.QueryEscape(sign), nil
}
