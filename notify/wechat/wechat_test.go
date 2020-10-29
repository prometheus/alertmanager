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
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

// TestNotify can test the entire notification process as long as you fill in
// the correct configuration: secret and WechatConfig.
// You can get a full description of the WechatConfig's fields on
// https://work.weixin.qq.com/api/doc/90001/90143/91199
func TestNotify(t *testing.T) {
	secret := "secret"
	u, err := url.Parse("https://qyapi.weixin.qq.com/cgi-bin/")
	require.NoError(t, err)
	notifier, err := New(
		&config.WechatConfig{
			APIURL:      &config.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			ToUser:      "binacs|user1|user2",
			ToParty:     "patry1|party2|party3",
			ToTag:       "tag1|tag2|tag3",
			AgentID:     "agentID",
			CorpID:      "corpID",
			MessageType: "text",
			Message:     "binacs: send from prometheus/alertmanager unit test at " + time.Now().Format("2006-1-2 15:4:5"),
			APISecret:   config.Secret(secret),
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.NotifyWithSecret(t, notifier)
}

func TestWechatRedactedURLOnInitialAuthentication(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret_key"
	notifier, err := New(
		&config.WechatConfig{
			APIURL:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  config.Secret(secret),
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, secret)
}

func TestWechatRedactedURLOnNotify(t *testing.T) {
	secret, token := "secret", "token"
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":"%s"}`, token)
	})
	defer fn()

	notifier, err := New(
		&config.WechatConfig{
			APIURL:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  config.Secret(secret),
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, secret, token)
}

func TestWechatMessageTypeSelector(t *testing.T) {
	secret, token := "secret", "token"
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":"%s"}`, token)
	})
	defer fn()

	notifier, err := New(
		&config.WechatConfig{
			APIURL:      &config.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			CorpID:      "corpid",
			APISecret:   config.Secret(secret),
			MessageType: "markdown",
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, secret, token)
}
