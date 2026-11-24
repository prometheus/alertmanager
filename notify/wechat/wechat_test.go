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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestWechatRedactedURLOnInitialAuthentication(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret_key"
	notifier, err := New(
		&WechatConfig{
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
		&WechatConfig{
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
		&WechatConfig{
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

func TestWechatTokenRequestTemplateError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		corpID    string
		apiSecret commoncfg.Secret
	}{
		{
			name:      "corp id",
			corpID:    `{{ template "missing" . }}`,
			apiSecret: commoncfg.Secret("secret"),
		},
		{
			name:      "api secret",
			corpID:    "corpid",
			apiSecret: commoncfg.Secret(`{{ template "missing" . }}`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, `{"errcode":40013,"errmsg":"invalid corpid"}`)
			}))
			defer srv.Close()
			u, err := url.Parse(srv.URL + "/")
			require.NoError(t, err)

			notifier, err := New(
				&WechatConfig{
					APIURL:     &amcommoncfg.URL{URL: u},
					HTTPConfig: &commoncfg.HTTPClientConfig{},
					CorpID:     tc.corpID,
					APISecret:  tc.apiSecret,
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "1")

			verdict := notifier.Notify(ctx, alert.New(model.Alert{Labels: model.LabelSet{"alertname": "test"}}, time.Time{}, false))
			require.False(t, verdict.ShouldRetry())
			require.ErrorContains(t, verdict.Err(), "templating error")
			require.Zero(t, requests.Load(), "no token request should be sent when templating fails")
		})
	}
}

func TestGetApiSecretFromSecret(t *testing.T) {
	n := &Notifier{conf: &WechatConfig{APISecret: commoncfg.Secret("shhh")}}
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

	n := &Notifier{conf: &WechatConfig{APISecretFile: tmpFile.Name()}}
	s, err := n.getApiSecret()
	require.NoError(t, err)
	require.Equal(t, "file-secret", s)
}

func TestGetApiSecretFromMissingFile(t *testing.T) {
	n := &Notifier{conf: &WechatConfig{APISecretFile: "/non/existent/wechat-secret.txt"}}
	s, err := n.getApiSecret()
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, "/non/existent/wechat-secret.txt", pathErr.Path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, s)
}

func TestWechatNotifyConcurrently(t *testing.T) {
	var tokenRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/gettoken"):
			tokenRequests.Add(1)
			fmt.Fprint(w, `{"access_token":"token"}`)
		case r.URL.Query().Get("access_token") == "token":
			fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
		default:
			fmt.Fprint(w, `{"errcode":40014,"errmsg":"invalid access_token"}`)
		}
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)

	notifier, err := New(
		&WechatConfig{
			APIURL:     &amcommoncfg.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  commoncfg.Secret("secret"),
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = notify.WithGroupKey(ctx, "1")
	testAlert := alert.New(model.Alert{Labels: model.LabelSet{"alertname": "test"}}, time.Time{}, false)

	// Every aggregation group notifies through the same integration from its
	// own goroutine, so Notify has to be safe to call concurrently. Run it
	// under the race detector.
	const workers = 8
	verdicts := make(chan notify.NotifyVerdict, workers)
	for range workers {
		go func() {
			verdicts <- notifier.Notify(ctx, testAlert)
		}()
	}
	for range workers {
		verdict := <-verdicts
		require.NoError(t, verdict.Err())
	}

	// Once the token is cached, later notifications reuse it.
	before := tokenRequests.Load()
	require.NoError(t, notifier.Notify(ctx, testAlert).Err())
	require.Equal(t, before, tokenRequests.Load())
}

func TestWechatInvalidateToken(t *testing.T) {
	n := &Notifier{}
	n.storeToken("fresh")

	// A notification that was sent with an older token must not throw away
	// the token another notification has fetched in the meantime.
	n.invalidateToken("stale")
	require.Equal(t, "fresh", n.cachedToken())

	n.invalidateToken("fresh")
	require.Empty(t, n.cachedToken())
}

func TestWechatCachedTokenExpires(t *testing.T) {
	n := &Notifier{}
	n.storeToken("token")
	require.Equal(t, "token", n.cachedToken())

	n.mtx.Lock()
	n.accessTokenAt = time.Now().Add(-3 * time.Hour)
	n.mtx.Unlock()
	require.Empty(t, n.cachedToken())
}
