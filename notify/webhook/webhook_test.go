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
	"fmt"
	"net/url"
	"testing"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
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
	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
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
