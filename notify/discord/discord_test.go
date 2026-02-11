// Copyright 2021 Prometheus Team
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

package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

// This is a test URL that has been modified to not be valid.
var testWebhookURL, _ = url.Parse("https://discord.com/api/webhooks/971139602272503183/78ZWZ4V3xwZUBKRFF-G9m1nRtDtNTChl_WzW6Q4kxShjSB02oLSiPTPa8TS2tTGO9EYf")

func TestDiscordRetry(t *testing.T) {
	notifier, err := New(
		&config.DiscordConfig{
			WebhookURL: &amcommoncfg.SecretURL{URL: testWebhookURL},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "retry - error on status %d", statusCode)
	}
}

func TestDiscordTemplating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		out := make(map[string]any)
		err := dec.Decode(&out)
		if err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	for _, tc := range []struct {
		title string
		cfg   *config.DiscordConfig

		retry  bool
		errMsg string
	}{
		{
			title: "full-blown message",
			cfg: &config.DiscordConfig{
				Title:   `{{ template "discord.default.title" . }}`,
				Message: `{{ template "discord.default.message" . }}`,
			},
			retry: false,
		},
		{
			title: "title with templating errors",
			cfg: &config.DiscordConfig{
				Title: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "message with templating errors",
			cfg: &config.DiscordConfig{
				Title:   `{{ template "discord.default.title" . }}`,
				Message: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.WebhookURL = &amcommoncfg.SecretURL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			ok, err := pd.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
			require.Equal(t, tc.retry, ok)
		})
	}
}

func TestDiscordRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.DiscordConfig{
			WebhookURL: &amcommoncfg.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestDiscordReadingURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "webhook_url")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.DiscordConfig{
			WebhookURLFile: f.Name(),
			HTTPConfig:     &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestDiscord_Notify(t *testing.T) {
	// Create a fake HTTP server to simulate the Discord webhook
	var resp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request as a string
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "reading request body failed")
		// Store the request body in the response
		resp = string(body)

		w.WriteHeader(http.StatusOK)
	}))

	// Create a temporary file to simulate the WebhookURLFile
	tempFile, err := os.CreateTemp(t.TempDir(), "webhook_url")
	require.NoError(t, err)

	// Write the fake webhook URL to the temp file
	_, err = tempFile.WriteString(srv.URL)
	require.NoError(t, err)

	// Create a DiscordConfig with the WebhookURLFile set
	cfg := &config.DiscordConfig{
		WebhookURLFile: tempFile.Name(),
		HTTPConfig:     &commoncfg.HTTPClientConfig{},
		Title:          "Test Title",
		Message:        "Test Message",
		Content:        "Test Content",
		Username:       "Test Username",
		AvatarURL:      "http://example.com/avatar.png",
	}

	// Create a new Discord notifier
	notifier, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	// Create a context and alerts
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"lbl1": "val1",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}

	// Call the Notify method
	ok, err := notifier.Notify(ctx, alerts...)
	require.NoError(t, err)
	require.False(t, ok)

	require.JSONEq(t, `{"content":"Test Content","embeds":[{"title":"Test Title","description":"Test Message","color":10038562}],"username":"Test Username","avatar_url":"http://example.com/avatar.png"}`, resp)
}
