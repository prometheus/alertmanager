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

// Package kafka provides the transport-layer building blocks for
// Alertmanager components that talk to Apache Kafka via franz-go.
//
// It deliberately stays narrow: configuration shared by producers and
// consumers, franz-go option construction, an error classifier, and a
// broker-list naming helper.  Higher-level concerns (bounded
// producer buffers, consumer group orchestration, message
// serialisation) live in the calling package.
//
// At the moment the only consumer of this package is the event
// recorder's Kafka output (eventrecorder/kafka.go).  A future Kafka
// receiver (see github.com/prometheus/alertmanager/issues/1996) is the
// other intended user.
package kafka

import (
	"errors"
	"fmt"
	"slices"
	"time"

	commoncfg "github.com/prometheus/common/config"
)

// Wire format identifiers used by producers and consumers.
const (
	FormatJSON     = "json"
	FormatProtobuf = "protobuf"
)

// Producer acknowledgement levels.
const (
	AcksNone   = "none"
	AcksLeader = "leader"
	AcksAll    = "all"
)

// Compression codecs supported on the producer wire.
const (
	CompressionNone   = "none"
	CompressionGzip   = "gzip"
	CompressionSnappy = "snappy"
	CompressionLZ4    = "lz4"
	CompressionZstd   = "zstd"
)

// Default values applied by BuildOpts when ClientOptions leaves a
// field zero.
const (
	DefaultClientID    = "alertmanager"
	DefaultPingTimeout = 5 * time.Second
	DefaultLinger      = 5 * time.Millisecond
	DefaultFlushBudget = 30 * time.Second
)

// ClientOptions is the configuration shared between Kafka producers
// and consumers used in Alertmanager.  It is the input to BuildOpts.
//
// All fields are optional except Brokers; consumers ignore producer-only
// knobs (Acks, Compression) and vice versa.  Topic doubles as the
// franz-go DefaultProduceTopic for producers and may be ignored or
// repurposed (e.g. as a subscription target) by consumers.
type ClientOptions struct {
	// Brokers is the list of Kafka seed brokers in host:port form.
	// At least one entry is required.
	Brokers []string

	// Topic is the default produce topic.  Optional for consumers.
	Topic string

	// ClientID is reported to the brokers.  Defaults to "alertmanager".
	ClientID string

	// Acks is the producer acknowledgement level: "", AcksNone,
	// AcksLeader (default), or AcksAll.  Producer-only.
	Acks string

	// Compression is the producer compression codec: "" (default,
	// no compression), CompressionNone, CompressionGzip,
	// CompressionSnappy, CompressionLZ4, or CompressionZstd.
	// Producer-only.
	Compression string

	// TLSConfig configures TLS for the broker connection.  If nil,
	// PLAINTEXT is used.
	TLSConfig *commoncfg.TLSConfig
}

// Validate checks ClientOptions for obvious problems.  It does not
// contact the brokers and does not mutate the receiver.
func (o ClientOptions) Validate() error {
	if len(o.Brokers) == 0 {
		return errors.New("kafka: at least one broker is required")
	}
	if slices.Contains(o.Brokers, "") {
		return errors.New("kafka: broker entries must be non-empty")
	}
	switch o.Acks {
	case "", AcksNone, AcksLeader, AcksAll:
	default:
		return fmt.Errorf("kafka: unknown acks %q, must be %q, %q, or %q",
			o.Acks, AcksNone, AcksLeader, AcksAll)
	}
	switch o.Compression {
	case "", CompressionNone, CompressionGzip,
		CompressionSnappy, CompressionLZ4, CompressionZstd:
	default:
		return fmt.Errorf("kafka: unknown compression %q", o.Compression)
	}
	return nil
}

// ValidateFormat checks that a wire-format string is recognised.  It
// is provided as a free helper because some callers (e.g. the event
// recorder) carry the format on a per-output struct rather than on the
// shared ClientOptions.
func ValidateFormat(format string) error {
	switch format {
	case FormatJSON, FormatProtobuf:
		return nil
	default:
		return fmt.Errorf("kafka: unknown format %q, must be %q or %q",
			format, FormatJSON, FormatProtobuf)
	}
}
