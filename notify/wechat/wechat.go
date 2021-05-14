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
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
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

// Notifier implements a Notifier for wechat notifications.
type Notifier struct {
	conf *config.WechatConfig
	tmpl *template.Template

	logger log.Logger
	client *http.Client

	accessToken   string
	accessTokenAt time.Time
	existGroup    map[string]bool
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
	ChatId   string               `yaml:"chatid,omitempty" json:"chatid,omitempty"`
}

type weChatMessageContent struct {
	Content string `json:"content"`
}

type weChatResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

type requestChatInfo struct {
	Name     string   `json:"name"`
	UserList []string `json:"userlist"`
	ChatId   string   `json:"chatid"`
	Owner    string   `json:"owner"`
}

// New returns a new Wechat notifier.
func New(c *config.WechatConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "wechat", append(httpOpts, commoncfg.WithHTTP2Disabled())...)
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

	// Refresh AccessToken over 2 hours
	if n.accessToken == "" || time.Since(n.accessTokenAt) > 2*time.Hour {
		parameters := url.Values{}
		parameters.Add("corpsecret", tmpl(string(n.conf.APISecret)))
		parameters.Add("corpid", tmpl(string(n.conf.CorpID)))
		if err != nil {
			return false, fmt.Errorf("templating error: %s", err)
		}

		u := n.conf.APIURL.Copy()
		u.Path += "gettoken"
		u.RawQuery = parameters.Encode()

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

		var wechatToken token
		if err := json.NewDecoder(resp.Body).Decode(&wechatToken); err != nil {
			return false, err
		}

		if wechatToken.AccessToken == "" {
			return false, fmt.Errorf("invalid APISecret for CorpID: %s", n.conf.CorpID)
		}

		// Cache accessToken
		n.accessToken = wechatToken.AccessToken
		n.accessTokenAt = time.Now()
	}
	msg := &weChatMessage{
		Safe: "0",
	}
	postMessageURL := n.conf.APIURL.Copy()

	switch n.conf.MessageType {
	case "groupMarkdown":
		postMessageURL.Path += "appchat/send"
		var users []string
		for _, user := range n.conf.GroupUsers {
			users = append(users, tmpl(user))
		}
		groupTitle := tmpl(n.conf.GroupTitle)
		chat, md5err := n.md5(groupTitle)
		if md5err != nil {
			return false, err
		}
		if err = n.checkCreateGroupChat(ctx, chat, users, groupTitle); err != nil {
			return false, err
		}
		msg.Type = "markdown"
		msg.ChatId = chat
		msg.Markdown = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	case "groupText":
		postMessageURL.Path += "appchat/send"
		var users []string
		for _, user := range n.conf.GroupUsers {
			users = append(users, tmpl(user))
		}
		groupTitle := tmpl(n.conf.GroupTitle)
		chat, md5err := n.md5(groupTitle)
		if md5err != nil {
			return false, err
		}
		if err = n.checkCreateGroupChat(ctx, chat, users, groupTitle); err != nil {
			return false, err
		}
		msg.Type = "text"
		msg.ChatId = chat
		msg.Text = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	case "markdown":
		postMessageURL.Path += "message/send"
		msg.ToUser = tmpl(n.conf.ToUser)
		msg.ToParty = tmpl(n.conf.ToParty)
		msg.Totag = tmpl(n.conf.ToTag)
		msg.AgentID = tmpl(n.conf.AgentID)
		msg.Type = "markdown"
		msg.Markdown = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	default:
		postMessageURL.Path += "message/send"
		msg.ToUser = tmpl(n.conf.ToUser)
		msg.ToParty = tmpl(n.conf.ToParty)
		msg.Totag = tmpl(n.conf.ToTag)
		msg.AgentID = tmpl(n.conf.AgentID)
		msg.Type = "text"
		msg.Text = weChatMessageContent{
			Content: tmpl(n.conf.Message),
		}
	}

	if err != nil {
		return false, fmt.Errorf("templating error: %s", err)
	}

	q := postMessageURL.Query()
	q.Set("access_token", n.accessToken)
	postMessageURL.RawQuery = q.Encode()

	weResp, err := n.sendRequest(ctx, postMessageURL.String(), http.MethodPost, &msg)

	if err != nil {
		level.Info(n.logger).Log("send chat err  ", err)
		return true, err
	}

	// https://work.weixin.qq.com/api/doc#10649
	if weResp.Code == 0 {
		level.Info(n.logger).Log("sendRequest response body", fmt.Sprintf("%v", weResp))
		return false, nil
	}

	// AccessToken is expired
	if weResp.Code == 42001 {
		n.accessToken = ""
		return true, errors.New(weResp.Error)
	}

	return false, errors.New(weResp.Error)
}

func (*Notifier) md5(src interface{}) (string, error) {
	if data, err := json.Marshal(src); err == nil {
		has := md5.Sum(data)
		md5str1 := fmt.Sprintf("%x", has) //将[]byte转成16进制
		return md5str1, nil
	} else {
		return "", err
	}

}

// create group chat if not exists,else get it;  groupTitle is the unique identifier of the group
//https://open.work.weixin.qq.com/api/doc/90000/90135/90245
func (n *Notifier) checkCreateGroupChat(ctx context.Context, chatId string, users []string, groupTitle string) error {

	if len(users) < 2 || len(users) > 2000 {
		return errors.New("the group member min 2 people, max 2000 （群成员id列表。至少2人，至多2000人）")
	}

	if n.existGroup == nil {
		n.existGroup = make(map[string]bool)
	}
	groupTitle = strings.Trim(groupTitle, "\n")
	if n.existGroup[chatId] {
		level.Debug(n.logger).Log("groupTitle:", groupTitle, " chat exists")
		return nil
	}

	chatInfo := requestChatInfo{
		ChatId:   chatId,
		UserList: users,
		Name:     strings.Trim(groupTitle, "\n"),
		Owner:    users[0], //the group owner is the first group member
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&chatInfo); err != nil {
		return err
	}
	postMessageURL := n.conf.APIURL.Copy()
	postMessageURL.Path += "appchat/create"
	q := postMessageURL.Query()
	q.Set("access_token", n.accessToken)
	postMessageURL.RawQuery = q.Encode()
	weResp, err := n.sendRequest(ctx, postMessageURL.String(), http.MethodPost, &chatInfo)
	if err != nil {
		return err
	}
	// https://work.weixin.qq.com/api/doc#10649
	// https://open.work.weixin.qq.com/devtool/query?e=86215
	if weResp.Code == 0 || weResp.Code == 86215 {
		n.existGroup[chatId] = true
		return nil
	}
	// AccessToken is expired
	if weResp.Code == 42001 {
		n.accessToken = ""
		return errors.New(weResp.Error)
	}
	return nil

}

func (n *Notifier) sendRequest(ctx context.Context, url string, methodPost string, request interface{}) (res weChatResponse, err error) {
	var buf bytes.Buffer
	if err = json.NewEncoder(&buf).Encode(request); err != nil {
		return
	}
	req, err := http.NewRequest(methodPost, url, &buf)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req.WithContext(ctx))
	if err != nil {
		return res, notify.RedactURL(err)
	}
	defer notify.Drain(resp)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	level.Debug(n.logger).Log("sendRequest response body", string(body))
	err = json.Unmarshal(body, &res)
	return res, err
}
