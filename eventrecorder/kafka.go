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
	"net"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	commoncfg "github.com/prometheus/common/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kslog"
	"google.golang.org/protobuf/proto"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// Kafka format identifiers.
const (
	KafkaFormatJSON     = "json"
	KafkaFormatProtobuf = "protobuf"
)

// Kafka acks levels.
const (
	KafkaAcksNone   = "none"
	KafkaAcksLeader = "leader"
	KafkaAcksAll    = "all"
)

// Kafka compression codecs.
const (
	KafkaCompressionNone   = "none"
	KafkaCompressionGzip   = "gzip"
	KafkaCompressionSnappy = "snappy"
	KafkaCompressionLZ4    = "lz4"
	KafkaCompressionZstd   = "zstd"
)

const (
	defaultKafkaBufferSize  = 1024
	defaultKafkaClientID    = "alertmanager"
	defaultKafkaPingTimeout = 5 * time.Second
	defaultKafkaLinger      = 5 * time.Millisecond
	defaultKafkaFlushBudget = 30 * time.Second
)

// validateKafka validates and normalizes Kafka-specific Output fields.
// Called from Output.UnmarshalYAML when Type == OutputKafka.
func (o *Output) validateKafka() error {
	if len(o.Brokers) == 0 {
		return errors.New("event_recorder kafka output requires at least one broker")
	}
	if slices.Contains(o.Brokers, "") {
		return errors.New("event_recorder kafka output broker entries must be non-empty")
	}
	if o.Topic == "" {
		return errors.New("event_recorder kafka output requires a topic")
	}
	if o.Format == "" {
		o.Format = KafkaFormatJSON
	}
	switch o.Format {
	case KafkaFormatJSON, KafkaFormatProtobuf:
	default:
		return fmt.Errorf("event_recorder kafka output: unknown format %q, must be %q or %q",
			o.Format, KafkaFormatJSON, KafkaFormatProtobuf)
	}
	switch o.Acks {
	case "", KafkaAcksLeader, KafkaAcksNone, KafkaAcksAll:
	default:
		return fmt.Errorf("event_recorder kafka output: unknown acks %q, must be %q, %q, or %q",
			o.Acks, KafkaAcksNone, KafkaAcksLeader, KafkaAcksAll)
	}
	switch o.Compression {
	case "", KafkaCompressionNone, KafkaCompressionGzip,
		KafkaCompressionSnappy, KafkaCompressionLZ4, KafkaCompressionZstd:
	default:
		return fmt.Errorf("event_recorder kafka output: unknown compression %q", o.Compression)
	}
	return nil
}

// kafkaOutputsEqual compares the kafka-specific fields of two Outputs.
// The caller has already verified that both outputs are of type
// OutputKafka.  Broker lists are compared order-independently because
// reordering brokers in YAML is semantically a no-op.
func kafkaOutputsEqual(a, b Output) bool {
	if !sortedStringSliceEqual(a.Brokers, b.Brokers) {
		return false
	}
	if a.Topic != b.Topic {
		return false
	}
	if a.ClientID != b.ClientID {
		return false
	}
	if a.Format != b.Format {
		return false
	}
	if a.Acks != b.Acks {
		return false
	}
	if a.Compression != b.Compression {
		return false
	}
	if a.BufferSize != b.BufferSize {
		return false
	}
	return reflect.DeepEqual(a.TLSConfig, b.TLSConfig)
}

// sortedStringSliceEqual reports whether a and b contain the same strings,
// independent of order.  Broker lists are considered equivalent when their
// contents match regardless of how the operator wrote them in YAML.
func sortedStringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
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
	format      string // KafkaFormatJSON or KafkaFormatProtobuf
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
	cfg Output,
	instance string,
	dropsCounter *prometheus.CounterVec,
	produceErrors *prometheus.CounterVec,
	logger *slog.Logger,
) (*KafkaOutput, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka output requires at least one broker")
	}
	if slices.Contains(cfg.Brokers, "") {
		return nil, errors.New("kafka output broker entries must be non-empty")
	}
	if cfg.Topic == "" {
		return nil, errors.New("kafka output requires a topic")
	}

	format := cfg.Format
	if format == "" {
		format = KafkaFormatJSON
	}
	if format != KafkaFormatJSON && format != KafkaFormatProtobuf {
		return nil, fmt.Errorf("kafka output: unsupported format %q", format)
	}

	clientID := cfg.ClientID
	if clientID == "" {
		clientID = defaultKafkaClientID
	}

	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultKafkaBufferSize
	}

	name := kafkaOutputName(cfg.Brokers, cfg.Topic)

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.DefaultProduceTopic(cfg.Topic),
		kgo.ClientID(clientID),
		kgo.ProducerLinger(defaultKafkaLinger),
		kgo.WithLogger(kslog.New(logger)),
	}

	acksOpt, err := kafkaAcksOpt(cfg.Acks)
	if err != nil {
		return nil, err
	}
	opts = append(opts, acksOpt)

	// franz-go enables idempotent writes by default, which mandates
	// acks=all.  Our default is acks=leader for low latency, so disable
	// idempotency unless the operator explicitly opted into acks=all.
	if cfg.Acks != KafkaAcksAll {
		opts = append(opts, kgo.DisableIdempotentWrite())
	}

	if cfg.Compression != "" {
		codec, err := kafkaCompressionCodec(cfg.Compression)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.ProducerBatchCompression(codec))
	}

	if cfg.TLSConfig != nil {
		tlsCfg, err := commoncfg.NewTLSConfig(cfg.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("kafka output: building TLS config: %w", err)
		}
		opts = append(opts, kgo.DialTLSConfig(tlsCfg))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka output: creating client: %w", err)
	}

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
		flushBudget: defaultKafkaFlushBudget,
	}

	// Best-effort connectivity check runs in the background so that
	// alertmanager startup (and event_recorder hot reload) is never
	// blocked by an unreachable broker.  Failure is logged at warn
	// level; franz-go keeps retrying connections internally, and any
	// records produced while the brokers are unreachable are buffered
	// (or dropped, counted via dropsCounter).
	go func() {
		pingCtx, cancel := context.WithTimeout(context.Background(), defaultKafkaPingTimeout)
		defer cancel()
		if pingErr := client.Ping(pingCtx); pingErr != nil {
			logger.Warn("Kafka event recorder output could not reach brokers at startup; will retry in background",
				"output", name, "err", pingErr)
		}
	}()

	ko.wg.Add(1)
	go ko.dispatch()

	return ko, nil
}

// kafkaOutputName builds a stable, Prometheus-label-safe identifier for
// the output.  Brokers are sorted so config reordering does not produce
// distinct metric label values.
func kafkaOutputName(brokers []string, topic string) string {
	sorted := append([]string(nil), brokers...)
	sort.Strings(sorted)
	return fmt.Sprintf("kafka:%s/%s", strings.Join(sorted, ","), topic)
}

// kafkaAcksOpt translates the user-facing acks string into a franz-go
// option.  Empty defaults to LeaderAck.
func kafkaAcksOpt(s string) (kgo.Opt, error) {
	switch s {
	case "", KafkaAcksLeader:
		return kgo.RequiredAcks(kgo.LeaderAck()), nil
	case KafkaAcksNone:
		return kgo.RequiredAcks(kgo.NoAck()), nil
	case KafkaAcksAll:
		return kgo.RequiredAcks(kgo.AllISRAcks()), nil
	default:
		return nil, fmt.Errorf("kafka output: unknown acks %q", s)
	}
}

// kafkaCompressionCodec translates the user-facing compression string
// into a franz-go codec.
func kafkaCompressionCodec(s string) (kgo.CompressionCodec, error) {
	switch s {
	case KafkaCompressionNone:
		return kgo.NoCompression(), nil
	case KafkaCompressionGzip:
		return kgo.GzipCompression(), nil
	case KafkaCompressionSnappy:
		return kgo.SnappyCompression(), nil
	case KafkaCompressionLZ4:
		return kgo.Lz4Compression(), nil
	case KafkaCompressionZstd:
		return kgo.ZstdCompression(), nil
	default:
		return kgo.NoCompression(), fmt.Errorf("kafka output: unknown compression %q", s)
	}
}

// Name returns the stable identifier for this output.
func (ko *KafkaOutput) Name() string { return ko.name }

// WantsProto reports whether this output prefers SendProto over SendEvent.
func (ko *KafkaOutput) WantsProto() bool { return ko.format == KafkaFormatProtobuf }

// SendEvent queues pre-serialized JSON bytes for delivery.  Used when
// the output is in JSON format (the default).
func (ko *KafkaOutput) SendEvent(data []byte) error {
	// Copy the slice: the caller (marshalAndSend) reuses the JSON buffer
	// across all destinations within a single event, and the dispatcher
	// runs asynchronously.
	cp := append([]byte(nil), data...)
	return ko.enqueue(cp)
}

// SendProto serializes the event as protobuf and queues it for delivery.
// Used when the output is in protobuf format.
func (ko *KafkaOutput) SendProto(event *eventrecorderpb.Event) (int, error) {
	data, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("kafka output: marshalling protobuf event: %w", err)
	}
	if err := ko.enqueue(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// enqueue places the value on the local buffer.  Returns an error if
// the output is already closed (so a SendEvent/SendProto racing with
// Close cannot land a record on a channel that no dispatcher will
// drain).  If the buffer is full the event is dropped; the upstream
// metric records the drop directly so callers do not need to inspect
// this error to count drops.
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
		ko.produceErrs.WithLabelValues(ko.name, classifyKafkaError(err)).Inc()
		ko.logger.Warn("Kafka event recorder produce failed", "output", ko.name, "err", err)
	})
}

// classifyKafkaError buckets a franz-go error into a coarse category
// suitable for use as a Prometheus label value.  This keeps the
// label cardinality bounded.
func classifyKafkaError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout"
		}
		return "network"
	}
	// franz-go surfaces broker-side errors typed as kerr.Error; we don't
	// need to import kerr to detect them — error strings produced by the
	// library all contain "broker" or a Kafka error code reference.
	msg := err.Error()
	if strings.Contains(msg, "broker") || strings.Contains(msg, "kafka:") {
		return "broker"
	}
	return "unknown"
}

// Close stops the dispatcher, drains any remaining buffered records
// into franz-go's producer, flushes the producer (up to flushBudget),
// and then closes the underlying client.  Pending records that do not
// flush before the budget expires are dropped on client close.
//
// Close is safe to call multiple times; subsequent calls are no-ops.
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
