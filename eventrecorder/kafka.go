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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	commoncfg "github.com/prometheus/common/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/kafka"
)

const defaultKafkaBufferSize = 1024

// KafkaOutputConfig configures a Kafka event recorder output.
type KafkaOutputConfig struct {
	// Brokers is the list of Kafka seed brokers in host:port form.
	Brokers []string `yaml:"brokers" json:"brokers"`
	// Topic is the Kafka topic to produce events to.
	Topic string `yaml:"topic" json:"topic"`
	// ClientID is reported to the Kafka brokers (default "alertmanager").
	ClientID string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	// Format selects the on-the-wire encoding of each event value:
	// "json" (default, JSON via protojson) or "protobuf" (binary proto).
	Format kafka.Format `yaml:"format,omitempty" json:"format,omitempty"`
	// Acks controls the producer acknowledgement level:
	// "none", "leader" (default), or "all".
	Acks kafka.Acks `yaml:"acks,omitempty" json:"acks,omitempty"`
	// Compression selects the producer compression codec:
	// "" (default, no compression), "none", "gzip", "snappy", "lz4", or "zstd".
	Compression kafka.Compression `yaml:"compression,omitempty" json:"compression,omitempty"`
	// BufferSize is the capacity of the local channel between the event
	// recorder dispatcher and the franz-go producer (default 1024).
	BufferSize int `yaml:"buffer_size,omitempty" json:"buffer_size,omitempty"`
	// TLSConfig configures TLS for the Kafka broker connection.  If unset,
	// PLAINTEXT is used.
	TLSConfig *commoncfg.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface, validating
// and normalising the Kafka output configuration.  Transport-layer
// validation (brokers, acks, compression, TLS) is delegated to the
// shared kafka package so this code and a future Kafka receiver share a
// single source of truth.
func (c *KafkaOutputConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type plain KafkaOutputConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if err := c.clientOptions().Validate(); err != nil {
		// The shared validator's messages already say "kafka: ..."; we
		// prefix with the event_recorder context for user clarity.
		return fmt.Errorf("event_recorder %w", err)
	}
	if c.Topic == "" {
		return errors.New("event_recorder kafka output requires a topic")
	}
	if c.Format == "" {
		c.Format = kafka.FormatJSON
	}
	if err := kafka.ValidateFormat(c.Format); err != nil {
		return fmt.Errorf("event_recorder %w", err)
	}
	return nil
}

// clientOptions copies the config into a kafka.ClientOptions value
// suitable for passing to kafka.BuildOpts.
func (c KafkaOutputConfig) clientOptions() kafka.ClientOptions {
	return kafka.ClientOptions{
		Brokers:     c.Brokers,
		Topic:       c.Topic,
		ClientID:    c.ClientID,
		Acks:        c.Acks,
		Compression: c.Compression,
		TLSConfig:   c.TLSConfig,
	}
}

// equal reports whether two kafka output configs are semantically
// equal.  Broker lists are compared order-independently because
// reordering brokers in YAML is semantically a no-op.
func (c KafkaOutputConfig) equal(o KafkaOutputConfig) bool {
	if !kafka.BrokerListsEqual(c.Brokers, o.Brokers) {
		return false
	}
	if c.Topic != o.Topic {
		return false
	}
	if c.ClientID != o.ClientID {
		return false
	}
	if c.Format != o.Format {
		return false
	}
	if c.Acks != o.Acks {
		return false
	}
	if c.Compression != o.Compression {
		return false
	}
	if c.BufferSize != o.BufferSize {
		return false
	}
	return reflect.DeepEqual(c.TLSConfig, o.TLSConfig)
}

// KafkaOutput delivers serialized events to a Kafka topic via franz-go.
// Events are buffered in a bounded local channel and produced by a
// single dispatcher goroutine; franz-go handles batching, compression,
// and retries internally.
//
// When the local buffer is full, events are dropped (with a log
// message and a metric increment) so that a slow or unreachable
// broker cannot block the upstream event recorder pipeline.
type KafkaOutput struct {
	mu          sync.RWMutex
	closed      bool
	client      *kgo.Client
	topic       string
	instance    string // used as the message key
	format      kafka.Format
	name        string // "kafka:<sorted-brokers>/<topic>"
	logger      *slog.Logger
	drops       prometheus.Counter
	produceErrs *prometheus.CounterVec
	work        chan []byte
	done        chan struct{}
	wg          sync.WaitGroup
	flushBudget time.Duration
}

// NewKafkaOutput constructs a KafkaOutput from the supplied configuration.
// A failure to reach the brokers at startup is logged at warn level but
// does not fail construction; franz-go retries connections in the
// background and records are buffered until delivery becomes possible.
func NewKafkaOutput(
	cfg KafkaOutputConfig,
	instance string,
	dropsCounter *prometheus.CounterVec,
	produceErrors *prometheus.CounterVec,
	logger *slog.Logger,
) (*KafkaOutput, error) {
	if cfg.Topic == "" {
		return nil, errors.New("kafka output requires a topic")
	}
	format := cfg.Format
	if format == "" {
		format = kafka.FormatJSON
	}
	if err := kafka.ValidateFormat(format); err != nil {
		return nil, err
	}

	// Shared validation + franz-go option construction lives in the
	// kafka package so a future Kafka receiver can reuse it.
	kopts, err := kafka.BuildOpts(cfg.clientOptions(), logger)
	if err != nil {
		return nil, err
	}

	client, err := kgo.NewClient(kopts...)
	if err != nil {
		return nil, fmt.Errorf("kafka output: creating client: %w", err)
	}

	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultKafkaBufferSize
	}

	name := fmt.Sprintf("kafka:%s/%s", kafka.BrokerList(cfg.Brokers), cfg.Topic)

	ko := &KafkaOutput{
		client:      client,
		topic:       cfg.Topic,
		instance:    instance,
		format:      format,
		name:        name,
		logger:      logger,
		drops:       dropsCounter.WithLabelValues(name),
		produceErrs: produceErrors,
		work:        make(chan []byte, bufferSize),
		done:        make(chan struct{}),
		flushBudget: kafka.DefaultFlushBudget,
	}

	// Best-effort connectivity check runs in the background so that
	// alertmanager startup (and event_recorder hot reload) is never
	// blocked by an unreachable broker.
	kafka.PingInBackground(client, logger)

	ko.wg.Add(1)
	go ko.dispatch()

	return ko, nil
}

// Name returns the stable identifier for this output.
func (ko *KafkaOutput) Name() string { return ko.name }

// SendEvent serializes the event in the configured format (JSON or
// protobuf) and queues it for asynchronous delivery.  It returns the
// serialized size (for the bytes-written metric).
func (ko *KafkaOutput) SendEvent(event *eventrecorderpb.Event) (int, error) {
	var (
		data []byte
		err  error
	)
	if ko.format == kafka.FormatProtobuf {
		data, err = proto.Marshal(event)
	} else {
		data, err = protojson.Marshal(event)
	}
	if err != nil {
		return 0, &serializeError{err: err}
	}
	if err := ko.enqueue(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// enqueue places the value on the local buffer.  Returns an error if
// the output is already closed (so a SendEvent racing with Close cannot
// land a record on a channel that no dispatcher will drain).  If the
// buffer is full the event is dropped; the upstream metric records the
// drop directly so callers do not need to inspect this error to count
// drops.
func (ko *KafkaOutput) enqueue(value []byte) error {
	ko.mu.RLock()
	defer ko.mu.RUnlock()
	if ko.closed {
		return errors.New("kafka output: closed")
	}
	select {
	case ko.work <- value:
		return nil
	default:
		ko.drops.Inc()
		ko.logger.Warn("Kafka event recorder buffer full, dropping event", "output", ko.name)
		return nil
	}
}

// dispatch reads queued payloads and hands them to franz-go for
// asynchronous production.  A single goroutine is sufficient because
// kgo.Client.Produce is itself non-blocking (it appends to franz-go's
// internal batched producer).
func (ko *KafkaOutput) dispatch() {
	defer ko.wg.Done()
	for {
		select {
		case value := <-ko.work:
			ko.produce(value)
		case <-ko.done:
			// Drain whatever is left in the local channel into
			// franz-go's producer before returning.  The actual
			// flush to brokers happens in Close.
			ko.drainWork()
			return
		}
	}
}

// drainWork non-blockingly drains every queued value into the
// producer.  Called by dispatch on shutdown.
func (ko *KafkaOutput) drainWork() {
	for {
		select {
		case value := <-ko.work:
			ko.produce(value)
		default:
			return
		}
	}
}

// produce hands a single record to franz-go.  The promise callback
// updates per-output metrics; it must be quick (franz-go calls all
// promises serially from a single goroutine).
func (ko *KafkaOutput) produce(value []byte) {
	rec := &kgo.Record{
		Key:   []byte(ko.instance),
		Value: value,
		Topic: ko.topic,
	}
	ko.client.Produce(context.Background(), rec, func(_ *kgo.Record, err error) {
		if err == nil {
			return
		}
		ko.produceErrs.WithLabelValues(ko.name, string(kafka.ClassifyError(err))).Inc()
		ko.logger.Warn("Kafka event recorder produce failed", "output", ko.name, "err", err)
	})
}

// Close stops the dispatcher, drains any remaining buffered records
// into franz-go's producer, flushes the producer (up to flushBudget),
// and then closes the underlying client.  Pending records that do not
// flush before the budget expires are dropped on client close.
//
// Close is safe to call multiple times; subsequent calls are no-ops.
//
// Note: franz-go's Client.Close has documented blocking behaviour
// around leaving consumer groups, but this client is configured as a
// producer only (no ConsumeTopics / ConsumePartitions / InstanceID),
// so the leave-group path is a no-op and Close will not block on it.
// The only bounded wait here is the explicit Flush above.
func (ko *KafkaOutput) Close() error {
	ko.mu.Lock()
	if ko.closed {
		ko.mu.Unlock()
		return nil
	}
	ko.closed = true
	close(ko.done)
	ko.mu.Unlock()

	ko.wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), ko.flushBudget)
	defer cancel()
	if err := ko.client.Flush(ctx); err != nil {
		ko.logger.Warn("Kafka event recorder flush did not complete within budget; remaining records will be dropped",
			"output", ko.name, "err", err)
	}
	ko.client.Close()
	return nil
}
