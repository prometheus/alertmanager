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

// Package eventrecorder provides a structured event recorder for
// significant Alertmanager events.  Events are serialized as JSON and
// fanned out to one or more configured destinations (JSONL file,
// webhook, etc.).
//
// RecordEvent never blocks the caller: events are serialized and
// placed on a bounded in-memory queue.  A background goroutine
// drains the queue and sends to destinations.  If the queue is full,
// events are dropped and a metric is incremented.
package eventrecorder

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
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

// Config configures the event recorder feature.
type Config struct {
	Outputs []Output `yaml:"outputs,omitempty" json:"outputs,omitempty"`
}

// Output configures a single event recorder output destination.
type Output struct {
	Type       string                      `yaml:"type" json:"type"`
	Path       string                      `yaml:"path,omitempty" json:"path,omitempty"`
	URL        *amcommoncfg.SecretURL      `yaml:"url,omitempty" json:"url,omitempty"`
	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	// Timeout for webhook HTTP requests (default 10s).
	Timeout model.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// Workers is the number of concurrent webhook delivery goroutines
	// (default 4).  Only applicable to webhook outputs.
	Workers int `yaml:"workers,omitempty" json:"workers,omitempty"`
	// MaxRetries is the maximum number of delivery attempts per event
	// (default 3).  Only applicable to webhook outputs.
	MaxRetries int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	// RetryBackoff is the base backoff duration between retry attempts
	// (default 500ms).  Successive attempts use exponential backoff
	// (base * 2^attempt).  Only applicable to webhook outputs.
	RetryBackoff model.Duration `yaml:"retry_backoff,omitempty" json:"retry_backoff,omitempty"`

	// --- Kafka-specific fields (Type == OutputKafka) ---

	// Brokers is the list of Kafka seed brokers in host:port form.
	Brokers []string `yaml:"brokers,omitempty" json:"brokers,omitempty"`
	// Topic is the Kafka topic to produce events to.
	Topic string `yaml:"topic,omitempty" json:"topic,omitempty"`
	// ClientID is reported to the Kafka brokers (default "alertmanager").
	ClientID string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	// Format selects the on-the-wire encoding of each event value:
	// "json" (default, JSON via protojson) or "protobuf" (binary proto).
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	// Acks controls the producer acknowledgement level:
	// "none", "leader" (default, franz-go default), or "all".
	Acks string `yaml:"acks,omitempty" json:"acks,omitempty"`
	// Compression selects the producer compression codec:
	// "" (default, no compression), "none", "gzip", "snappy", "lz4", or "zstd".
	Compression string `yaml:"compression,omitempty" json:"compression,omitempty"`
	// BufferSize is the capacity of the local channel between the event
	// recorder dispatcher and the franz-go producer (default 1024).
	BufferSize int `yaml:"buffer_size,omitempty" json:"buffer_size,omitempty"`
	// TLSConfig configures TLS for the Kafka broker connection.  If unset,
	// PLAINTEXT is used.
	TLSConfig *commoncfg.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Output.
func (o *Output) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Output
	if err := unmarshal((*plain)(o)); err != nil {
		return err
	}
	switch o.Type {
	case OutputFile:
		if o.Path == "" {
			return errors.New("event_recorder file output requires a path")
		}
	case OutputWebhook:
		if o.URL == nil {
			return errors.New("event_recorder webhook output requires a url")
		}
	case OutputKafka:
		return o.validateKafka()
	default:
		return fmt.Errorf("unknown event_recorder output type %q, must be %q, %q, or %q",
			o.Type, OutputFile, OutputWebhook, OutputKafka)
	}
	return nil
}

// validateKafka validates and normalizes Kafka-specific Output fields.
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

// configEqual compares two Config values by their
// semantically significant fields.
func configEqual(a, b Config) bool {
	if len(a.Outputs) != len(b.Outputs) {
		return false
	}
	for i := range a.Outputs {
		oa, ob := a.Outputs[i], b.Outputs[i]
		if oa.Type != ob.Type {
			return false
		}
		if oa.Path != ob.Path {
			return false
		}
		if oa.Timeout != ob.Timeout {
			return false
		}
		aURL, bURL := "", ""
		if oa.URL != nil {
			aURL = oa.URL.String()
		}
		if ob.URL != nil {
			bURL = ob.URL.String()
		}
		if aURL != bURL {
			return false
		}
		if oa.Workers != ob.Workers {
			return false
		}
		if oa.MaxRetries != ob.MaxRetries {
			return false
		}
		if oa.RetryBackoff != ob.RetryBackoff {
			return false
		}
		if !reflect.DeepEqual(oa.HTTPConfig, ob.HTTPConfig) {
			return false
		}
		// Kafka fields.
		if !sortedStringSliceEqual(oa.Brokers, ob.Brokers) {
			return false
		}
		if oa.Topic != ob.Topic {
			return false
		}
		if oa.ClientID != ob.ClientID {
			return false
		}
		if oa.Format != ob.Format {
			return false
		}
		if oa.Acks != ob.Acks {
			return false
		}
		if oa.Compression != ob.Compression {
			return false
		}
		if oa.BufferSize != ob.BufferSize {
			return false
		}
		if !reflect.DeepEqual(oa.TLSConfig, ob.TLSConfig) {
			return false
		}
	}
	return true
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
