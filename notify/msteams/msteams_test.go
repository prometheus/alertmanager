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

package msteams

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
var testWebhookURL, _ = url.Parse("https://example.webhook.office.com/webhookb2/xxxxxx/IncomingWebhook/xxx/xxx")

func TestMSTeamsRetry(t *testing.T) {
	notifier, err := New(
		&config.MSTeamsConfig{
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

func TestMSTeamsTemplating(t *testing.T) {
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
		cfg   *config.MSTeamsConfig

		retry  bool
		errMsg string
	}{
		{
			title: "full-blown message",
			cfg: &config.MSTeamsConfig{
				Title:   `{{ template "msteams.default.title" . }}`,
				Summary: `{{ template "msteams.default.summary" . }}`,
				Text:    `{{ template "msteams.default.text" . }}`,
			},
			retry: false,
		},
		{
			title: "title with templating errors",
			cfg: &config.MSTeamsConfig{
				Title: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "summary with templating errors",
			cfg: &config.MSTeamsConfig{
				Title:   `{{ template "msteams.default.title" . }}`,
				Summary: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
		{
			title: "message with templating errors",
			cfg: &config.MSTeamsConfig{
				Title:   `{{ template "msteams.default.title" . }}`,
				Summary: `{{ template "msteams.default.summary" . }}`,
				Text:    "{{ ",
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

func TestNotifier_Notify_WithReason(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		responseContent string
		expectedReason  notify.Reason
		noError         bool
	}{
		{
			name:            "with a 2xx status code and response 1",
			statusCode:      http.StatusOK,
			responseContent: "1",
			noError:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := New(
				&config.MSTeamsConfig{
					WebhookURL: &amcommoncfg.SecretURL{URL: testWebhookURL},
					HTTPConfig: &commoncfg.HTTPClientConfig{},
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)

			notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
				resp := httptest.NewRecorder()
				resp.WriteString(tt.responseContent)
				resp.WriteHeader(tt.statusCode)
				return resp.Result(), nil
			}
			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			alert1 := &types.Alert{
				Alert: model.Alert{
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}
			_, err = notifier.Notify(ctx, alert1)
			if tt.noError {
				require.NoError(t, err)
			} else {
				var reasonError *notify.ErrorWithReason
				require.ErrorAs(t, err, &reasonError)
				require.Equal(t, tt.expectedReason, reasonError.Reason)
			}
		})
	}
}

func TestMSTeamsRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.MSTeamsConfig{
			WebhookURL: &amcommoncfg.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestMSTeamsReadingURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "webhook_url")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.MSTeamsConfig{
			WebhookURLFile: f.Name(),
			HTTPConfig:     &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}
