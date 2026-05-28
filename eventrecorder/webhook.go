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

package eventrecorder

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	commoncfg "github.com/prometheus/common/config"
)

const (
	defaultWebhookTimeout      = 10 * time.Second
	defaultWebhookWorkers      = 4
	defaultWebhookMaxRetries   = 3
	defaultWebhookRetryBackoff = 500 * time.Millisecond
	defaultWebhookMaxBackoff   = 30 * time.Second
	webhookQueueSize           = 1024
)

// WebhookOutput POSTs each event as a JSON body to a configured URL.
// Events are processed by a bounded worker pool so that a slow or
// temporarily unavailable webhook does not block the event recorder queue.
// Events are dropped (with a log message) when the internal queue is
// full.
type WebhookOutput struct {
	client       *http.Client
	url          string
	name         string
	maxRetries   int
	retryBackoff time.Duration
	maxBackoff   time.Duration
	logger       *slog.Logger
	drops        prometheus.Counter
	work         chan []byte
	done         chan struct{}
	cancel       chan struct{} // closed after drain to abort remaining retries
	wg           sync.WaitGroup
}

// NewWebhookOutput creates a new webhook-based event recorder output.
func NewWebhookOutput(cfg Output, dropsCounter *prometheus.CounterVec, logger *slog.Logger) (*WebhookOutput, error) {
	httpCfg := commoncfg.DefaultHTTPClientConfig
	if cfg.HTTPConfig != nil {
		httpCfg = *cfg.HTTPConfig
	}

	client, err := commoncfg.NewClientFromConfig(httpCfg, "eventrecorder")
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client for event recorder webhook: %w", err)
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

	urlStr := cfg.URL.String()
	wo := &WebhookOutput{
		client:       client,
		url:          urlStr,
		name:         fmt.Sprintf("webhook:%s", sanitizeURL(urlStr)),
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		maxBackoff:   defaultWebhookMaxBackoff,
		logger:       logger,
		drops:        dropsCounter.WithLabelValues(fmt.Sprintf("webhook:%s", sanitizeURL(urlStr))),
		work:         make(chan []byte, webhookQueueSize),
		done:         make(chan struct{}),
		cancel:       make(chan struct{}),
	}

	for range workers {
		wo.wg.Add(1)
		go wo.worker()
	}

	return wo, nil
}

// sanitizeURL strips userinfo and query parameters from a URL string,
// returning only scheme://host/path.  This prevents credentials from
// leaking into metrics labels and log messages.
func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// Name returns a stable identifier for this output.  The URL is
// sanitized to avoid leaking credentials.
func (wo *WebhookOutput) Name() string {
	return wo.name
}

// SendEvent queues the pre-serialized JSON bytes for delivery by a
// worker.  If the internal queue is full, the event is dropped and
// counted via the webhook_drops metric.
func (wo *WebhookOutput) SendEvent(data []byte) error {
	select {
	case wo.work <- data:
	default:
		wo.drops.Inc()
		wo.logger.Warn("Event recorder webhook queue full, dropping event", "output", wo.name)
	}
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
		wo.logger.Warn("Event recorder webhook POST failed", "output", wo.name, "attempt", attempt+1, "err", err)
		if attempt < wo.maxRetries-1 {
			backoff := min(wo.retryBackoff<<attempt, wo.maxBackoff)
			select {
			case <-time.After(backoff):
			case <-wo.cancel:
				wo.logger.Warn("Event recorder webhook shutdown during retry backoff, dropping event", "output", wo.name)
				return
			}
		}
	}
	wo.logger.Error("Event recorder webhook POST failed after retries, dropping event", "output", wo.name, "retries", wo.maxRetries)
}

func (wo *WebhookOutput) post(data []byte) error {
	resp, err := wo.client.Post(wo.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("event recorder webhook POST failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("event recorder webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Close signals all workers to stop, drains remaining events, and
// waits.  If the drain takes longer than 30 seconds, remaining
// retries are canceled.
func (wo *WebhookOutput) Close() error {
	close(wo.done)
	ch := make(chan struct{})
	go func() {
		wo.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		close(wo.cancel)
		wo.wg.Wait()
	}
	return nil
}
