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

package webhook_template

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestWebhookRetry(t *testing.T) {
	u, err := url.Parse("http://example.com")
	if err != nil {
		require.NoError(t, err)
	}
	notifier, err := New(
		&config.WebhookTemplateConfig{
			URL:        &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	if err != nil {
		require.NoError(t, err)
	}

	t.Run(
		"test retry status code", func(t *testing.T) {
			for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
				actual, _ := notifier.retrier.Check(statusCode, nil)
				require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
			}
		},
	)

	t.Run(
		"test retry error details", func(t *testing.T) {
			for _, tc := range []struct {
				status int
				body   io.Reader

				exp string
			}{
				{
					status: http.StatusBadRequest,
					body: bytes.NewBuffer(
						[]byte(
							`{"status":"invalid event"}`,
						),
					),

					exp: fmt.Sprintf(`unexpected status code %d: {"status":"invalid event"}`, http.StatusBadRequest),
				},
				{
					status: http.StatusBadRequest,

					exp: fmt.Sprintf(`unexpected status code %d`, http.StatusBadRequest),
				},
			} {
				t.Run(
					"", func(t *testing.T) {
						_, err = notifier.retrier.Check(tc.status, tc.body)
						require.Equal(t, tc.exp, err.Error())
					},
				)
			}
		},
	)
}

func TestWebhookRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.WebhookTemplateConfig{
			URL:        &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestWebhookReadingURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "webhook_url")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.WebhookTemplateConfig{
			URLFile:    f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}
