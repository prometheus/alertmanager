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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// WebhookOutputConfig configures an HTTP webhook event recorder output.
type WebhookOutputConfig struct {
	// URL is the endpoint to POST each event to.
	URL *amcommoncfg.SecretURL `yaml:"url" json:"url"`
	// HTTPConfig configures the HTTP client used for webhook delivery.
	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	// Timeout for webhook HTTP requests (default 10s).
	Timeout model.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// Workers is the number of concurrent delivery goroutines (default 4).
	Workers int `yaml:"workers,omitempty" json:"workers,omitempty"`
	// MaxRetries is the maximum number of delivery attempts per event
	// (default 3).
	MaxRetries int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	// RetryBackoff is the base backoff between retry attempts (default
	// 500ms).  Successive attempts use exponential backoff (base *
	// 2^attempt).
	RetryBackoff model.Duration `yaml:"retry_backoff,omitempty" json:"retry_backoff,omitempty"`
	// Batch enables sending events as JSON arrays instead of individual objects.
	Batch bool `yaml:"batch,omitempty" json:"batch,omitempty"`
	// BatchMaxEvents is the maximum number of events in one request (default 100).
	BatchMaxEvents int `yaml:"batch_max_events,omitempty" json:"batch_max_events,omitempty"`
	// BatchMaxBytes is the soft maximum encoded request size (default 1 MiB).
	BatchMaxBytes int `yaml:"batch_max_bytes,omitempty" json:"batch_max_bytes,omitempty"`
	// BatchFlushInterval is the maximum time an incomplete batch waits (default 100ms).
	BatchFlushInterval model.Duration `yaml:"batch_flush_interval,omitempty" json:"batch_flush_interval,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface, validating
// the webhook output configuration.
//
// Note: SecretURL.UnmarshalYAML delegates to ParseURL, which already
// enforces a non-empty host and an http(s) scheme.  The only way an
// otherwise-valid config reaches this function with a degenerate URL is
// via the "<secret>" placeholder shortcut in SecretURL.UnmarshalYAML,
// which sets URL to an empty url.URL{}.  We catch that case here.
func (c *WebhookOutputConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type plain WebhookOutputConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil || c.URL.URL == nil {
		return errors.New("event_recorder webhook output requires a url")
	}
	if c.URL.Scheme == "" || c.URL.Host == "" {
		return errors.New("event_recorder webhook output requires an absolute http(s) url")
	}
	if c.BatchMaxEvents < 0 || c.BatchMaxBytes < 0 || c.BatchFlushInterval < 0 {
		return errors.New("event_recorder webhook batch settings cannot be negative")
	}
	if !c.Batch && (c.BatchMaxEvents != 0 || c.BatchMaxBytes != 0 || c.BatchFlushInterval != 0) {
		return errors.New("event_recorder webhook batch settings require batch: true")
	}
	return nil
}

// equal reports whether two webhook output configs are semantically
// equal.
func (c WebhookOutputConfig) equal(o WebhookOutputConfig) bool {
	aURL, bURL := "", ""
	if c.URL != nil {
		aURL = c.URL.String()
	}
	if o.URL != nil {
		bURL = o.URL.String()
	}
	if aURL != bURL {
		return false
	}
	if c.Timeout != o.Timeout {
		return false
	}
	if c.Workers != o.Workers {
		return false
	}
	if c.MaxRetries != o.MaxRetries {
		return false
	}
	if c.RetryBackoff != o.RetryBackoff {
		return false
	}
	if c.Batch != o.Batch {
		return false
	}
	if c.Batch && !httpBatchConfigsEqual(
		newHTTPBatchConfig(c.BatchMaxEvents, c.BatchMaxBytes, c.BatchFlushInterval),
		newHTTPBatchConfig(o.BatchMaxEvents, o.BatchMaxBytes, o.BatchFlushInterval),
	) {
		return false
	}
	return reflect.DeepEqual(c.HTTPConfig, o.HTTPConfig)
}

const (
	defaultWebhookTimeout      = 10 * time.Second
	defaultWebhookWorkers      = 4
	defaultWebhookMaxRetries   = 3
	defaultWebhookRetryBackoff = 500 * time.Millisecond
	defaultWebhookMaxBackoff   = 30 * time.Second
	defaultHTTPBatchMaxEvents  = 100
	defaultHTTPBatchMaxBytes   = 1 << 20
	defaultHTTPBatchInterval   = 100 * time.Millisecond
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
	kind         string
	batch        *httpBatchConfig
	maxRetries   int
	retryBackoff time.Duration
	maxBackoff   time.Duration
	logger       *slog.Logger
	drops        prometheus.Counter
	work         chan []byte
	batches      chan []byte
	done         chan struct{}
	cancel       chan struct{} // closed after drain to abort remaining retries
	wg           sync.WaitGroup
}

type httpBatchConfig struct {
	maxEvents     int
	maxBytes      int
	flushInterval time.Duration
}

// NewWebhookOutput creates a new webhook-based event recorder output.
func NewWebhookOutput(cfg WebhookOutputConfig, dropsCounter *prometheus.CounterVec, logger *slog.Logger) (*WebhookOutput, error) {
	var batch *httpBatchConfig
	if cfg.Batch {
		batch = newHTTPBatchConfig(cfg.BatchMaxEvents, cfg.BatchMaxBytes, cfg.BatchFlushInterval)
	}
	return newWebhookOutput(cfg, "webhook", batch, dropsCounter, logger)
}

func newHTTPBatchConfig(maxEvents, maxBytes int, flushInterval model.Duration) *httpBatchConfig {
	batch := &httpBatchConfig{
		maxEvents:     defaultHTTPBatchMaxEvents,
		maxBytes:      defaultHTTPBatchMaxBytes,
		flushInterval: defaultHTTPBatchInterval,
	}
	if maxEvents > 0 {
		batch.maxEvents = maxEvents
	}
	if maxBytes > 0 {
		batch.maxBytes = maxBytes
	}
	if flushInterval > 0 {
		batch.flushInterval = time.Duration(flushInterval)
	}
	return batch
}

func httpBatchConfigsEqual(a, b *httpBatchConfig) bool {
	return *a == *b
}

func newWebhookOutput(cfg WebhookOutputConfig, kind string, batch *httpBatchConfig, dropsCounter *prometheus.CounterVec, logger *slog.Logger) (*WebhookOutput, error) {
	httpCfg := commoncfg.DefaultHTTPClientConfig
	if cfg.HTTPConfig != nil {
		httpCfg = *cfg.HTTPConfig
	}

	client, err := commoncfg.NewClientFromConfig(httpCfg, "eventrecorder")
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client for event recorder %s: %w", kind, err)
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
	name := fmt.Sprintf("%s:%s", kind, sanitizeURL(urlStr))
	wo := &WebhookOutput{
		client:       client,
		url:          urlStr,
		name:         name,
		kind:         kind,
		batch:        batch,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		maxBackoff:   defaultWebhookMaxBackoff,
		logger:       logger,
		drops:        dropsCounter.WithLabelValues(name),
		work:         make(chan []byte, webhookQueueSize),
		done:         make(chan struct{}),
		cancel:       make(chan struct{}),
	}

	if batch != nil {
		wo.wg.Add(1)
		wo.batches = make(chan []byte, workers)
		go wo.batchLoop()
		for range workers {
			wo.wg.Add(1)
			go wo.batchDeliveryWorker()
		}
	} else {
		for range workers {
			wo.wg.Add(1)
			go wo.worker()
		}
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

func sanitizeSecretURL(u *amcommoncfg.SecretURL) string {
	if u == nil || u.URL == nil {
		return "<missing>"
	}
	return sanitizeURL(u.String())
}

// Name returns a stable identifier for this output.  The URL is
// sanitized to avoid leaking credentials.
func (wo *WebhookOutput) Name() string {
	return wo.name
}

// SendEvent serializes the event as JSON and queues it for delivery by
// a worker.  It returns the serialized size (for the bytes-written
// metric).  If the internal queue is full the event is dropped and
// counted via the output-drops metric.
func (wo *WebhookOutput) SendEvent(event *eventrecorderpb.Event) (int, error) {
	data, err := protojson.Marshal(event)
	if err != nil {
		return 0, &serializeError{err: err}
	}
	select {
	case wo.work <- data:
	default:
		wo.drops.Inc()
		wo.logger.Warn("Event recorder HTTP output queue full, dropping event", "output", wo.name)
	}
	return len(data), nil
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

func (wo *WebhookOutput) batchLoop() {
	defer wo.wg.Done()
	defer close(wo.batches)

	batch := make([][]byte, 0, min(wo.batch.maxEvents, webhookQueueSize))
	batchSize := 2 // Opening and closing brackets.
	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			timerC = nil
		}
	}
	flush := func() {
		stopTimer()
		if len(batch) == 0 {
			return
		}
		wo.batches <- jsonArray(batch, batchSize)
		batch = batch[:0]
		batchSize = 2
	}
	add := func(data []byte) {
		additionalSize := len(data)
		if len(batch) > 0 {
			additionalSize++ // Comma separator.
		}
		if len(batch) > 0 && (len(batch) >= wo.batch.maxEvents || batchSize+additionalSize > wo.batch.maxBytes) {
			flush()
			additionalSize = len(data)
		}
		batch = append(batch, data)
		batchSize += additionalSize
		if len(batch) == 1 {
			timer = time.NewTimer(wo.batch.flushInterval)
			timerC = timer.C
		}
		if len(batch) >= wo.batch.maxEvents || batchSize >= wo.batch.maxBytes {
			flush()
		}
	}

	for {
		select {
		case data := <-wo.work:
			add(data)
		case <-timerC:
			flush()
		case <-wo.done:
			for {
				select {
				case data := <-wo.work:
					add(data)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (wo *WebhookOutput) batchDeliveryWorker() {
	defer wo.wg.Done()
	for data := range wo.batches {
		wo.postWithRetry(data)
	}
}

func jsonArray(events [][]byte, size int) []byte {
	data := make([]byte, 0, size)
	data = append(data, '[')
	for i, event := range events {
		if i > 0 {
			data = append(data, ',')
		}
		data = append(data, event...)
	}
	return append(data, ']')
}

func (wo *WebhookOutput) postWithRetry(data []byte) {
	for attempt := range wo.maxRetries {
		err := wo.post(data)
		if err == nil {
			return
		}
		wo.logger.Warn("Event recorder HTTP output POST failed", "output", wo.name, "attempt", attempt+1, "err", err)
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
	wo.logger.Error("Event recorder HTTP output POST failed after retries, dropping event", "output", wo.name, "retries", wo.maxRetries)
}

func (wo *WebhookOutput) post(data []byte) error {
	resp, err := wo.client.Post(wo.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("event recorder %s POST failed: %w", wo.kind, err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("event recorder %s returned HTTP %d", wo.kind, resp.StatusCode)
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
