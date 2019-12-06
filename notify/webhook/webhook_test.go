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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
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
			URL:        &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	if err != nil {
		require.NoError(t, err)
	}

	retryCodes := append(test.DefaultRetryCodes(), http.StatusRequestTimeout)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestTimeout(t *testing.T) {
	const (
		configTimeout   = model.Duration(1500 * time.Millisecond)
		expectedTimeout = "1.500000"
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := r.Header.Get("X-Alertmanager-Notify-Timeout-Seconds")
			if timeout != expectedTimeout {
				t.Errorf("Expected scrape timeout header %q, got %q", expectedTimeout, timeout)
			}
			w.WriteHeader(http.StatusRequestTimeout)
		}),
	)
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		require.NoError(t, err)
	}
	notifier, err := New(
		&config.WebhookConfig{
			URL:        &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			Timeout:    configTimeout,
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	if err != nil {
		require.NoError(t, err)
	}

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"job":       "j1",
				"instance":  "i1",
				"alertname": "an1",
			},
		},
	}

	retry, err := notifier.Notify(context.Background(), alert)
	require.Error(t, err)
	require.Equal(t, true, retry)
}
