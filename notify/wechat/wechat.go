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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

// Notifier implements a Notifier for wechat notifications.
type Notifier struct {
	conf   *WechatConfig
	tmpl   *template.Template
	logger *slog.Logger
	client *http.Client

	// Notify is called concurrently by the aggregation groups of a receiver,
	// so the cached access token is guarded by a mutex.
	mtx           sync.Mutex
	accessToken   string
	accessTokenAt time.Time

	// refreshMtx serializes token requests so a burst of notifications that
	// all find the cache empty only asks the API once. It is never held while
	// sending a message.
	refreshMtx sync.Mutex
}

// token is the AccessToken with corpid and corpsecret.
type token struct {
	AccessToken string `json:"access_token"`
}

type weChatMessage struct {
	Text     weChatMessageContent `yaml:"text,omitempty" json:"text,omitempty"`
	ToUser   string               `yaml:"touser,omitempty" json:"touser,omitempty"`
	ToParty  string               `yaml:"toparty,omitempty" json:"toparty,omitempty"`
	Totag    string               `yaml:"totag,omitempty" json:"totag,omitempty"`
	AgentID  string               `yaml:"agentid,omitempty" json:"agentid,omitempty"`
	Safe     string               `yaml:"safe,omitempty" json:"safe,omitempty"`
	Type     string               `yaml:"msgtype,omitempty" json:"msgtype,omitempty"`
	Markdown weChatMessageContent `yaml:"markdown,omitempty" json:"markdown,omitempty"`
}

type weChatMessageContent struct {
	Content string `json:"content"`
}

type weChatResponse struct {
	Code  int    `json:"errcode"`
	Error string `json:"errmsg"`
}

// New returns a new Wechat notifier.
func New(c *WechatConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "wechat", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*alert.Alert) notify.NotifyVerdict {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}

	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	data := notify.GetTemplateData(ctx, n.tmpl, as, logger)

	var tmplErr error
	tmpl := notify.TmplText(n.tmpl, data, &tmplErr)

	// Refresh AccessToken over 2 hours
	accessToken := n.cachedToken()
	if accessToken == "" {
		var verdict notify.NotifyVerdict
		var ok bool
		accessToken, verdict, ok = n.refreshToken(ctx, tmpl, &tmplErr)
		if !ok {
			return verdict
		}
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
	if tmplErr != nil {
		return notify.Unrecoverable(fmt.Errorf("templating error: %w", tmplErr), notify.TemplateErrorReason)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}

	postMessageURL := n.conf.APIURL.Copy()
	postMessageURL.Path += "message/send"
	q := postMessageURL.Query()
	q.Set("access_token", accessToken)
	postMessageURL.RawQuery = q.Encode()

	resp, err := notify.PostJSON(ctx, n.client, postMessageURL.String(), &buf)
	if err != nil {
		return notify.Retry(0, notify.RedactURL(err), notify.DefaultReason)
	}
	defer notify.Drain(resp)

	if resp.StatusCode != 200 {
		return notify.Retry(0, fmt.Errorf("unexpected status code %v", resp.StatusCode), notify.GetFailureReasonFromStatusCode(resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}
	logger.Debug(string(body))

	var weResp weChatResponse
	if err := json.Unmarshal(body, &weResp); err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}

	// https://work.weixin.qq.com/api/doc#10649
	if weResp.Code == 0 {
		return notify.Success()
	}

	// AccessToken is expired
	if weResp.Code == 42001 {
		n.invalidateToken(accessToken)
		return notify.Retry(0, errors.New(weResp.Error), notify.DefaultReason)
	}

	return notify.Unrecoverable(errors.New(weResp.Error), notify.DefaultReason)
}

// refreshToken fetches a new access token. Concurrent callers that all missed
// the cache wait for each other, so only the first one asks the API and the
// rest pick up the token it stored. On failure ok is false and verdict says why.
func (n *Notifier) refreshToken(ctx context.Context, tmpl func(string) string, tmplErr *error) (accessToken string, verdict notify.NotifyVerdict, ok bool) {
	n.refreshMtx.Lock()
	defer n.refreshMtx.Unlock()

	if t := n.cachedToken(); t != "" {
		return t, notify.NotifyVerdict{}, true
	}

	parameters := url.Values{}
	apiSecret, err := n.getApiSecret()
	if err != nil {
		return "", notify.Unrecoverable(err, notify.DefaultReason), false
	}
	parameters.Add("corpsecret", tmpl(apiSecret))
	parameters.Add("corpid", tmpl(string(n.conf.CorpID)))
	if *tmplErr != nil {
		return "", notify.Unrecoverable(fmt.Errorf("templating error: %w", *tmplErr), notify.TemplateErrorReason), false
	}

	u := n.conf.APIURL.Copy()
	u.Path += "gettoken"
	u.RawQuery = parameters.Encode()

	resp, err := notify.Get(ctx, n.client, u.String())
	if err != nil {
		return "", notify.Retry(0, notify.RedactURL(err), notify.DefaultReason), false
	}
	defer notify.Drain(resp)

	var wechatToken token
	if err := json.NewDecoder(resp.Body).Decode(&wechatToken); err != nil {
		return "", notify.Unrecoverable(err, notify.DefaultReason), false
	}

	if wechatToken.AccessToken == "" {
		return "", notify.Unrecoverable(fmt.Errorf("invalid APISecret for CorpID: %s", n.conf.CorpID), notify.DefaultReason), false
	}

	// Cache accessToken
	n.storeToken(wechatToken.AccessToken)
	return wechatToken.AccessToken, notify.NotifyVerdict{}, true
}

// cachedToken returns the cached access token, or an empty string if there is
// none or it is older than 2 hours.
func (n *Notifier) cachedToken() string {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if time.Since(n.accessTokenAt) > 2*time.Hour {
		return ""
	}
	return n.accessToken
}

func (n *Notifier) storeToken(token string) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.accessToken = token
	n.accessTokenAt = time.Now()
}

// invalidateToken drops the cached token if it is still the given one. Another
// notification may already have replaced an expired token with a fresh one.
func (n *Notifier) invalidateToken(token string) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if n.accessToken == token {
		n.accessToken = ""
	}
}

func (n *Notifier) getApiSecret() (string, error) {
	if len(n.conf.APISecretFile) > 0 {
		content, err := os.ReadFile(n.conf.APISecretFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	return string(n.conf.APISecret), nil
}
