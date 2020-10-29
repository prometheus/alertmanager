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

package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// To renew the token before it expires, we reduced the token validity time to 90%
const tokenValidityCoefficient int = 90

func calcValidityTime(expireIn int) time.Duration {
	return time.Duration(expireIn*tokenValidityCoefficient/100) * time.Second
}

// Notifier implements a Notifier for wechat notifications.
type Notifier struct {
	conf   *config.WechatConfig
	tmpl   *template.Template
	logger log.Logger
	client *http.Client

	accessToken string
	expireAt    time.Time
}

type weChatMessage struct {
	ToUser   string               `yaml:"touser,omitempty" json:"touser,omitempty"`
	ToParty  string               `yaml:"toparty,omitempty" json:"toparty,omitempty"`
	Totag    string               `yaml:"totag,omitempty" json:"totag,omitempty"`
	AgentID  string               `yaml:"agentid,omitempty" json:"agentid,omitempty"`
	Safe     string               `yaml:"safe,omitempty" json:"safe,omitempty"`
	Type     string               `yaml:"msgtype,omitempty" json:"msgtype,omitempty"`
	Text     weChatMessageContent `yaml:"text,omitempty" json:"text,omitempty"`
	Markdown weChatMessageContent `yaml:"markdown,omitempty" json:"markdown,omitempty"`
}

type weChatMessageContent struct {
	Content string `json:"content"`
}

type weChatResponse struct {
	Code  int    `json:"errcode"`
	Error string `json:"errmsg"`
}

// tokenResponse is the AccessToken and ExpiresIn with corpid and corpsecret.
type tokenResponse struct {
	weChatResponse
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// New returns a new Wechat notifier.
func New(c *config.WechatConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "wechat", false, false)
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
	// Refresh AccessToken
	if n.accessToken == "" || time.Now().After(n.expireAt) {
		parameters := url.Values{}
		parameters.Add("corpsecret", tmpl(string(n.conf.APISecret)))
		parameters.Add("corpid", tmpl(string(n.conf.CorpID)))
		if err != nil {
			return false, fmt.Errorf("templating error: %s", err)
		}

		u := n.conf.APIURL.Copy()
		u.Path += "gettoken"
		u.RawQuery = parameters.Encode()

		// Record the time before we send request
		startTime := time.Now()
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return true, err
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := n.client.Do(req.WithContext(ctx))
		if err != nil {
			return true, notify.RedactURL(err)
		}
		defer notify.Drain(resp)

		var wechatToken tokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&wechatToken); err != nil {
			return false, err
		}

		if wechatToken.AccessToken == "" {
			return false, fmt.Errorf("invalid APISecret for CorpID: %s", n.conf.CorpID)
		}

		// Cache accessToken
		n.accessToken = wechatToken.AccessToken
		n.expireAt = startTime.Add(calcValidityTime(wechatToken.ExpiresIn))
	}

	msg := &weChatMessage{
		ToUser:  tmpl(n.conf.ToUser),
		ToParty: tmpl(n.conf.ToParty),
		Totag:   tmpl(n.conf.ToTag),
		AgentID: tmpl(n.conf.AgentID),
		Type:    n.conf.MessageType,
		Safe:    "0",
	}

	if msg.Type == "markdown" {
		msg.Markdown = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	} else {
		msg.Text = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	}
	if err != nil {
		return false, fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	parameters := url.Values{}
	parameters.Add("access_token", n.accessToken)
	u := n.conf.APIURL.Copy()
	u.Path += "message/send"
	u.RawQuery = parameters.Encode()

	resp, err := notify.PostJSON(ctx, n.client, u.String(), &buf)
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

	var weResp weChatResponse
	if err := json.Unmarshal(body, &weResp); err != nil {
		return true, err
	}

	// https://work.weixin.qq.com/api/doc#10649
	if weResp.Code == 0 {
		return false, nil
	}

	// AccessToken is expired
	if weResp.Code == 42001 {
		n.accessToken = ""
		return true, errors.New(weResp.Error)
	}

	return false, errors.New(weResp.Error)
}
