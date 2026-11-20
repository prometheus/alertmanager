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
	"os"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestWechatRedactedURLOnInitialAuthentication(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret_key"
	notifier, err := New(
		&config.WechatConfig{
			APIURL:     &amcommoncfg.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  commoncfg.Secret(secret),
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestWechatRedactedURLOnNotify(t *testing.T) {
	secret, token := "secret", "token"
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":"%s"}`, token)
	})
	defer fn()

	notifier, err := New(
		&config.WechatConfig{
			APIURL:     &amcommoncfg.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  commoncfg.Secret(secret),
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret, token)
}

func TestWechatMessageTypeSelector(t *testing.T) {
	secret, token := "secret", "token"
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":"%s"}`, token)
	})
	defer fn()

	notifier, err := New(
		&config.WechatConfig{
			APIURL:      &amcommoncfg.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			CorpID:      "corpid",
			APISecret:   commoncfg.Secret(secret),
			MessageType: "markdown",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret, token)
}

func TestGetApiSecretFromSecret(t *testing.T) {
	n := &Notifier{conf: &config.WechatConfig{APISecret: commoncfg.Secret("shhh")}}
	s, err := n.getApiSecret()
	require.NoError(t, err)
	require.Equal(t, "shhh", s)
}

func TestGetApiSecretFromFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "wechat-secret-*")
	require.NoError(t, err)
	secretContent := "file-secret\n"
	_, err = tmpFile.WriteString(secretContent)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	n := &Notifier{conf: &config.WechatConfig{APISecretFile: tmpFile.Name()}}
	s, err := n.getApiSecret()
	require.NoError(t, err)
	require.Equal(t, "file-secret", s)
}

func TestGetApiSecretFromMissingFile(t *testing.T) {
	n := &Notifier{conf: &config.WechatConfig{APISecretFile: "/non/existent/wechat-secret.txt"}}
	s, err := n.getApiSecret()
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, "/non/existent/wechat-secret.txt", pathErr.Path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, s)
}
