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
	"math"
	"net/http"
	"sync"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
)

const (
	defaultWebhookTimeout      = 10 * time.Second
	defaultWebhookWorkers      = 4
	defaultWebhookMaxRetries   = 3
	defaultWebhookRetryBackoff = 500 * time.Millisecond
)

// WebhookOutput POSTs each event as a JSON body to a configured URL.
// Events are processed by a bounded worker pool so that a slow or
// temporarily unavailable webhook does not block the event log queue.
type WebhookOutput struct {
	client       *http.Client
	url          string
	maxRetries   int
	retryBackoff time.Duration
	logger       *slog.Logger
	work         chan []byte
	done         chan struct{}
	wg           sync.WaitGroup
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

	workers := defaultWebhookWorkers
	if cfg.Workers > 0 {
		workers = cfg.Workers
	}

	maxRetries := defaultWebhookMaxRetries
	if cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}

	retryBackoff := defaultWebhookRetryBackoff
	if cfg.RetryBackoff > 0 {
		retryBackoff = time.Duration(cfg.RetryBackoff)
	}

	wo := &WebhookOutput{
		client:       client,
		url:          cfg.URL.String(),
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		logger:       logger,
		work:         make(chan []byte, workers),
		done:         make(chan struct{}),
	}

	for range workers {
		wo.wg.Add(1)
		go wo.worker()
	}

	return wo, nil
}

// Name returns a stable identifier for this output.
func (wo *WebhookOutput) Name() string {
	return fmt.Sprintf("webhook:%s", wo.url)
}

// WriteEvent queues the pre-serialized JSON bytes for delivery by a
// worker.  If all workers are busy the call blocks until one is free,
// providing backpressure through the global event queue.
func (wo *WebhookOutput) WriteEvent(data []byte) error {
	wo.work <- data
	return nil
}

func (wo *WebhookOutput) worker() {
	defer wo.wg.Done()
	for {
		select {
		case data := <-wo.work:
			wo.postWithRetry(data)
		case <-wo.done:
			// Drain remaining items.
			for {
				select {
				case data := <-wo.work:
					wo.postWithRetry(data)
				default:
					return
				}
			}
		}
	}
}

func (wo *WebhookOutput) postWithRetry(data []byte) {
	for attempt := range wo.maxRetries {
		err := wo.post(data)
		if err == nil {
			return
		}
		wo.logger.Warn("Event log webhook POST failed", "url", wo.url, "attempt", attempt+1, "err", err)
		if attempt < wo.maxRetries-1 {
			backoff := time.Duration(float64(wo.retryBackoff) * math.Pow(2, float64(attempt)))
			time.Sleep(backoff)
		}
	}
	wo.logger.Error("Event log webhook POST failed after retries, dropping event", "url", wo.url, "retries", wo.maxRetries)
}

func (wo *WebhookOutput) post(data []byte) error {
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

// Close signals all workers to stop, drains remaining events, and waits.
func (wo *WebhookOutput) Close() error {
	close(wo.done)
	wo.wg.Wait()
	return nil
}
