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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestWebhookRetry(t *testing.T) {
	u, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	notifier := &Notifier{conf: &config.WebhookConfig{URL: &config.URL{URL: u}}}
	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}
