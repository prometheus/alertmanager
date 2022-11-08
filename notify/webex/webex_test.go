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

package webex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestWebexRetry(t *testing.T) {
	testWebhookURL, err := url.Parse("https://api.ciscospark.com/v1/message")
	require.NoError(t, err)

	notifier, err := New(
		&config.WebexConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			WebhookURL: &config.SecretURL{URL: testWebhookURL},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestWebexTemplating(t *testing.T) {
	tc := []struct {
		name string

		cfg     *config.WebexConfig
		Message string
		expJSON string

		retry  bool
		errMsg string
	}{
		{
			name: "with a valid message, it is formatted as expected",
			cfg: &config.WebexConfig{
				Message: `{{ template "webex.default.message" . }}`,
			},
			expJSON: `{"markdown":"\n\nAlerts Firing:\nLabels:\n - lbl1 = val1\n - lbl3 = val3\nAnnotations:\nSource: \nLabels:\n - lbl1 = val1\n - lbl2 = val2\nAnnotations:\nSource: \n\n\n\n"}`,
			retry:   false,
		},
		{
			name: "with templating errors, it fails.",
			cfg: &config.WebexConfig{
				Message: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			var out []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				out, err = io.ReadAll(r.Body)
				require.NoError(t, err)
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			tt.cfg.WebhookURL = &config.SecretURL{URL: u}
			tt.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			notifierWebex, err := New(tt.cfg, test.CreateTmpl(t), log.NewNopLogger())
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "1")

			ok, err := notifierWebex.Notify(ctx, []*types.Alert{
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
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
							"lbl2": "val2",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)

			if tt.errMsg == "" {
				require.NoError(t, err)
				fmt.Println(string(out))
				require.JSONEq(t, tt.expJSON, string(out))
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			}

			require.Equal(t, tt.retry, ok)
		})
	}
}
