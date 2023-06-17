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

package webhook

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestWebhookRetry(t *testing.T) {
	u, err := url.Parse("http://example.com")
	if err != nil {
		require.NoError(t, err)
	}
	notifier, err := New(
		&config.WebhookConfig{
			URL:        &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	if err != nil {
		require.NoError(t, err)
	}

	t.Run("test retry status code", func(t *testing.T) {
		for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
			actual, _ := notifier.retrier.Check(statusCode, nil)
			require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
		}
	})

	t.Run("test retry error details", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			body   io.Reader

			exp string
		}{
			{
				status: http.StatusBadRequest,
				body: bytes.NewBuffer([]byte(
					`{"status":"invalid event"}`,
				)),

				exp: fmt.Sprintf(`unexpected status code %d: %s: {"status":"invalid event"}`, http.StatusBadRequest, u.String()),
			},
			{
				status: http.StatusBadRequest,

				exp: fmt.Sprintf(`unexpected status code %d: %s`, http.StatusBadRequest, u.String()),
			},
		} {
			t.Run("", func(t *testing.T) {
				_, err = notifier.retrier.Check(tc.status, tc.body)
				require.Equal(t, tc.exp, err.Error())
			})
		}
	})
}

func TestWebhookTruncateAlerts(t *testing.T) {
	alerts := make([]*types.Alert, 10)

	truncatedAlerts, numTruncated := truncateAlerts(0, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, numTruncated, 0)

	truncatedAlerts, numTruncated = truncateAlerts(4, alerts)
	require.Len(t, truncatedAlerts, 4)
	require.EqualValues(t, numTruncated, 6)

	truncatedAlerts, numTruncated = truncateAlerts(100, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, numTruncated, 0)
}

func TestWebhookRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.WebhookConfig{
			URL:        &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
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
		&config.WebhookConfig{
			URLFile:    f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestWebhookCalculateNumAlerts(t *testing.T) {
	t.Run("OnlyFiringAlerts", func(t *testing.T) {
		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(time.Hour),
				},
			},
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(time.Hour),
				},
			},
		}

		numFiring, numResolved := calculateNumAlerts(alerts)
		require.EqualValues(t, numFiring, 2)
		require.EqualValues(t, numResolved, 0)
	})

	t.Run("OnlyResolvedAlerts", func(t *testing.T) {
		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(-time.Hour),
				},
			},
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(-time.Hour),
				},
			},
		}

		numFiring, numResolved := calculateNumAlerts(alerts)
		require.EqualValues(t, numFiring, 0)
		require.EqualValues(t, numResolved, 2)
	})

	t.Run("MixedAlerts", func(t *testing.T) {
		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(-time.Hour),
				},
			},
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(time.Hour),
				},
			},
		}

		numFiring, numResolved := calculateNumAlerts(alerts)
		require.EqualValues(t, numFiring, 1)
		require.EqualValues(t, numResolved, 1)
	})
}
