// Copyright The Prometheus Authors
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

package eventlog

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
)

const defaultWebhookTimeout = 10 * time.Second

// WebhookOutput POSTs each event as a JSON body to a configured URL.
type WebhookOutput struct {
	client *http.Client
	url    string
}

// NewWebhookOutput creates a new webhook-based event log output.
func NewWebhookOutput(cfg config.EventLogOutput, logger *slog.Logger) (*WebhookOutput, error) {
	httpCfg := commoncfg.DefaultHTTPClientConfig
	if cfg.HTTPConfig != nil {
		httpCfg = *cfg.HTTPConfig
	}

	client, err := commoncfg.NewClientFromConfig(httpCfg, "eventlog")
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client for event log webhook: %w", err)
	}

	timeout := defaultWebhookTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout)
	}
	client.Timeout = timeout

	return &WebhookOutput{
		client: client,
		url:    cfg.URL.String(),
	}, nil
}

// WriteEvent POSTs the pre-serialized JSON bytes to the webhook URL.
func (wo *WebhookOutput) WriteEvent(data []byte) error {
	resp, err := wo.client.Post(wo.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("event log webhook POST failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("event log webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (wo *WebhookOutput) Close() error {
	return nil
}
