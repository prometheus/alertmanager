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

package slack

import (
	"context"
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

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestSlackRetry(t *testing.T) {
	notifier, err := New(
		&config.SlackConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
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

func TestSlackRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.SlackConfig{
			APIURL:     &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestGettingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "slack_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String())
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestTrimmingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "slack_test_newline")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestNotifier_Notify_WithReason(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedReason notify.Reason
		expectedErr    string
		expectedRetry  bool
		noError        bool
	}{
		{
			name:           "with a 4xx status code",
			statusCode:     http.StatusUnauthorized,
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 401",
		},
		{
			name:           "with a 5xx status code",
			statusCode:     http.StatusInternalServerError,
			expectedReason: notify.ServerErrorReason,
			expectedRetry:  true,
			expectedErr:    "unexpected status code 500",
		},
		{
			name:           "with a 3xx status code",
			statusCode:     http.StatusTemporaryRedirect,
			expectedReason: notify.DefaultReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 307",
		},
		{
			name:           "with a 1xx status code",
			statusCode:     http.StatusSwitchingProtocols,
			expectedReason: notify.DefaultReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 101",
		},
		{
			name:           "2xx response with invalid JSON",
			statusCode:     http.StatusOK,
			responseBody:   `{"not valid json"}`,
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  true,
			expectedErr:    "could not unmarshal",
		},
		{
			name:           "2xx response with a JSON error",
			statusCode:     http.StatusOK,
			responseBody:   `{"ok":false,"error":"error_message"}`,
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  false,
			expectedErr:    "error response from Slack: error_message",
		},
		{
			name:           "2xx response with a plaintext error",
			statusCode:     http.StatusOK,
			responseBody:   "no_channel",
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  false,
			expectedErr:    "error response from Slack: no_channel",
		},
		{
			name:         "successful JSON response",
			statusCode:   http.StatusOK,
			responseBody: `{"ok":true}`,
			noError:      true,
		},
		{
			name:         "successful plaintext response",
			statusCode:   http.StatusOK,
			responseBody: "ok",
			noError:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiurl, _ := url.Parse("https://slack.com/post.Message")
			notifier, err := New(
				&config.SlackConfig{
					NotifierConfig: config.NotifierConfig{},
					HTTPConfig:     &commoncfg.HTTPClientConfig{},
					APIURL:         &config.SecretURL{URL: apiurl},
					Channel:        "channelname",
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)

			notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
				resp := httptest.NewRecorder()
				if strings.HasPrefix(tt.responseBody, "{") {
					resp.Header().Add("Content-Type", "application/json; charset=utf-8")
				}
				resp.WriteHeader(tt.statusCode)
				resp.WriteString(tt.responseBody)
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
			retry, err := notifier.Notify(ctx, alert1)
			require.Equal(t, tt.expectedRetry, retry)
			if tt.noError {
				require.NoError(t, err)
			} else {
				var reasonError *notify.ErrorWithReason
				require.ErrorAs(t, err, &reasonError)
				require.Equal(t, tt.expectedReason, reasonError.Reason)
				require.Contains(t, err.Error(), tt.expectedErr)
				require.Contains(t, err.Error(), "channelname")
			}
		})
	}
}
