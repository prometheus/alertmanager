// Copyright 2022 Prometheus Team
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

package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestTelegramUnmarshal(t *testing.T) {
	in := `
route:
  receiver: test
receivers:
- name: test
  telegram_configs:
  - chat_id: 1234
    bot_token: secret
    message_thread_id: 1357
`
	var c config.Config
	err := yaml.Unmarshal([]byte(in), &c)
	require.NoError(t, err)

	require.Len(t, c.Receivers, 1)
	require.Len(t, c.Receivers[0].TelegramConfigs, 1)

	require.Equal(t, "https://api.telegram.org", c.Receivers[0].TelegramConfigs[0].APIUrl.String())
	require.Equal(t, commoncfg.Secret("secret"), c.Receivers[0].TelegramConfigs[0].BotToken)
	require.Equal(t, int64(1234), c.Receivers[0].TelegramConfigs[0].ChatID)
	require.Equal(t, 1357, c.Receivers[0].TelegramConfigs[0].MessageThreadID)
	require.Equal(t, "HTML", c.Receivers[0].TelegramConfigs[0].ParseMode)
}

func TestTelegramRetry(t *testing.T) {
	// Fake url for testing purposes
	fakeURL := amcommoncfg.URL{
		URL: &url.URL{
			Scheme: "https",
			Host:   "FAKE_API",
		},
	}
	notifier, err := New(
		&config.TelegramConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIUrl:     &fakeURL,
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "error on status %d", statusCode)
	}
}

func TestTelegramNotify(t *testing.T) {
	token := "secret"

	fileWithToken, err := os.CreateTemp(t.TempDir(), "telegram-bot-token")
	require.NoError(t, err, "creating temp file failed")
	_, err = fileWithToken.WriteString(token)
	require.NoError(t, err, "writing to temp file failed")

	for _, tc := range []struct {
		name    string
		cfg     config.TelegramConfig
		expText string
	}{
		{
			name: "No escaping by default",
			cfg: config.TelegramConfig{
				Message:    "<code>x < y</code>",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: "<code>x < y</code>",
		},
		{
			name: "Characters escaped in HTML mode",
			cfg: config.TelegramConfig{
				ParseMode:  "HTML",
				Message:    "<code>x < y</code>",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: "<code>x &lt; y</code>",
		},
		{
			name: "Bot token from file",
			cfg: config.TelegramConfig{
				Message:      "test",
				HTTPConfig:   &commoncfg.HTTPClientConfig{},
				BotTokenFile: fileWithToken.Name(),
			},
			expText: "test",
		},
		{
			name: "HTML mode with too-large message",
			cfg: config.TelegramConfig{
				ParseMode:  "HTML",
				Message:    strings.Repeat("x", 5000),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "Default mode with too-large message",
			cfg: config.TelegramConfig{
				Message:    strings.Repeat("y", 5000),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("y", maxMessageLenRunes-1) + "…",
		},
		{
			name: "HTML mode with message smaller than limit",
			cfg: config.TelegramConfig{
				ParseMode:  "HTML",
				Message:    strings.Repeat("a", 100),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("a", 100),
		},
		{
			name: "Default mode with message smaller than limit",
			cfg: config.TelegramConfig{
				Message:    strings.Repeat("b", 100),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("b", 100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/bot"+token+"/sendMessage", r.URL.Path)
				var err error
				out, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Write([]byte(`{"ok":true,"result":{"chat":{}}}`))
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			tc.cfg.APIUrl = &amcommoncfg.URL{URL: u}

			notifier, err := New(&tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "1")

			retry, err := notifier.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
							"lbl3": "val3",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)

			require.False(t, retry)
			require.NoError(t, err)

			req := map[string]string{}
			err = json.Unmarshal(out, &req)
			require.NoError(t, err)
			require.Equal(t, tc.expText, req["text"])
		})
	}
}

// TestTelegramNotifyRedactURL verifies that notify.RedactURL is applied to the
// error returned by client.Send, so the bot token is never exposed in logs.
func TestTelegramNotifyRedactURL(t *testing.T) {
	token := "secret"

	t.Run("transport error redacts URL", func(t *testing.T) {
		// Point at a closed server so telebot returns a *url.Error with the full URL.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		u, _ := url.Parse(srv.URL)
		srv.Close() // closed immediately — next dial will fail

		notifier, err := New(
			&config.TelegramConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				APIUrl:     &amcommoncfg.URL{URL: u},
				BotToken:   commoncfg.Secret(token),
			},
			test.CreateTmpl(t),
			promslog.NewNopLogger(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = notify.WithGroupKey(ctx, "1")

		retry, err := notifier.Notify(ctx, &types.Alert{
			Alert: model.Alert{Labels: model.LabelSet{"alertname": "test"}},
		})
		require.True(t, retry)
		require.Error(t, err)
		// The token must not appear in the error string.
		require.NotContains(t, err.Error(), token, "bot token leaked in transport error")
		// The URL should be redacted.
		require.Contains(t, err.Error(), "<redacted>")
	})

	t.Run("Telegram API error passes through without token", func(t *testing.T) {
		// Return a Telegram API error response — telebot wraps this as its own
		// error type (not *url.Error), so RedactURL is a no-op, but the token
		// is not in the error string either.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found","error_code":400}`))
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		notifier, err := New(
			&config.TelegramConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				APIUrl:     &amcommoncfg.URL{URL: u},
				BotToken:   commoncfg.Secret(token),
			},
			test.CreateTmpl(t),
			promslog.NewNopLogger(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = notify.WithGroupKey(ctx, "1")

		retry, err := notifier.Notify(ctx, &types.Alert{
			Alert: model.Alert{Labels: model.LabelSet{"alertname": "test"}},
		})
		require.True(t, retry)
		require.Error(t, err)
		require.NotContains(t, err.Error(), token, "bot token leaked in API error")
	})
}
}
