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

package zeustelegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

func TestZeusTelegramUnmarshal(t *testing.T) {
	in := `
route:
  receiver: test
receivers:
- name: test
  zeustelegram_configs:
  - chat_id: 1234
    bot_token: secret
    api_url: https://zeus.example.com
    sensitive_data: ["password", "token"]
    event_id: "{{ .GroupLabels.alertname }}"
    severity: "{{ .CommonLabels.severity }}"
`

	var c config.Config
	err := yaml.Unmarshal([]byte(in), &c)
	require.NoError(t, err)

	require.Len(t, c.Receivers, 1)
	require.Len(t, c.Receivers[0].ZeusTelegramConfigs, 1)

	cfg := c.Receivers[0].ZeusTelegramConfigs[0]
	require.Equal(t, "https://zeus.example.com", cfg.APIUrl.String())
	require.Equal(t, config.Secret("secret"), cfg.BotToken)
	require.Equal(t, int64(1234), cfg.ChatID)
	require.Equal(t, []string{"password", "token"}, cfg.SensitiveData)
	require.Equal(t, "{{ .GroupLabels.alertname }}", cfg.EventId)
	require.Equal(t, "{{ .CommonLabels.severity }}", cfg.Severity)
}

func TestZeusTelegramRetry(t *testing.T) {
	notifier, err := New(
		&config.ZeusTelegramConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIUrl:     &config.URL{URL: &url.URL{Scheme: "https", Host: "zeus.example.com"}},
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

func TestZeusTelegramNotify(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.ZeusTelegramConfig
		statusCode   int
		responseBody string
		expectedMsg  zeusTelegramMessage
		expectError  bool
		expectRetry  bool
		mockPostJSON func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
	}{
		{
			name: "Successful notification",
			cfg: config.ZeusTelegramConfig{
				BotToken:  "secret",
				ChatID:    1234,
				APIUrl:    &config.URL{URL: &url.URL{Scheme: "https", Host: "api.zeus.com"}},
				EventId:   "test-event",
				Severity:  "critical",
				Text:      "Test message",
				ParseMode: "HTML",
			},
			statusCode:   http.StatusOK,
			responseBody: `{"status":"ok"}`,
			expectedMsg: zeusTelegramMessage{
				BotToken:  "secret",
				ChatID:    1234,
				EventId:   "test-event",
				Severity:  "critical",
				Text:      "Test message",
				ParseMode: "HTML",
			},
		},
		{
			name: "Server error with retry",
			cfg: config.ZeusTelegramConfig{
				BotToken: "secret",
				ChatID:   1234,
				APIUrl:   &config.URL{URL: &url.URL{Scheme: "https", Host: "api.zeus.com"}},
			},
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"error":"internal server error"}`,
			expectError:  true,
			expectRetry:  true,
		},
		{
			name: "HTTP request error",
			cfg: config.ZeusTelegramConfig{
				BotToken: "secret",
				ChatID:   1234,
				APIUrl:   &config.URL{URL: &url.URL{Scheme: "https", Host: "api.zeus.com"}},
			},
			mockPostJSON: func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
				return nil, errors.New("connection error")
			},
			expectError: true,
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedMsg zeusTelegramMessage

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				err := json.NewDecoder(r.Body).Decode(&receivedMsg)
				require.NoError(t, err)

				w.WriteHeader(tt.statusCode)
				_, err = w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()

			if tt.cfg.APIUrl == nil {
				u, _ := url.Parse(server.URL)
				tt.cfg.APIUrl = &config.URL{URL: u}
			}

			notifier, err := New(&tt.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			if tt.mockPostJSON != nil {
				notifier.postJSONFunc = tt.mockPostJSON
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "test-group")

			retry, err := notifier.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"alertname": "TestAlert",
							"severity":  "critical",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedMsg, receivedMsg)
			}

			require.Equal(t, tt.expectRetry, retry)
		})
	}
}

func TestNotifyWithTemplate(t *testing.T) {
	tmpl := `{{ define "zeus.subject" }}Alert: {{ .CommonLabels.alertname }}{{ end }}`
	templates, err := template.FromGlobs([]string{}, tmpl)
	require.NoError(t, err)

	cfg := config.ZeusTelegramConfig{
		BotToken: "secret",
		ChatID:   1234,
		APIUrl:   &config.URL{URL: &url.URL{Scheme: "https", Host: "api.zeus.com"}},
		Subject:  `{{ template "zeus.subject" . }}`,
	}

	var receivedMsg zeusTelegramMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		err := json.NewDecoder(r.Body).Decode(&receivedMsg)
		require.NoError(t, err)
		_, err = w.Write([]byte(`{"status":"ok"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	cfg.APIUrl = &config.URL{URL: u}

	notifier, err := New(&cfg, templates, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx := notify.WithGroupKey(context.Background(), "test-group")
	_, err = notifier.Notify(ctx, []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
				},
			},
		},
	}...)

	require.NoError(t, err)
	require.Equal(t, "Alert: TestAlert", receivedMsg.Subject)
}
