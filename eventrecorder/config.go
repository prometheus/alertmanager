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
	"fmt"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// Config configures the event recorder feature.
type Config struct {
	Outputs []Output `yaml:"outputs,omitempty" json:"outputs,omitempty"`
}

// Output configures a single event recorder output destination.
//
// All output types share the same struct because YAML unmarshalling is
// single-pass; only a subset of fields applies to each Type.  The
// per-type field groups are listed below.  Validation and equality
// comparison for the type-specific fields live in the corresponding
// output file (file.go, webhook.go, kafka.go).
type Output struct {
	// Type selects the destination kind.  Must be one of OutputFile,
	// OutputWebhook, or OutputKafka.
	Type string `yaml:"type" json:"type"`

	// --- File output fields (Type == OutputFile) ---

	// Path is the JSONL file to append events to.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`

	// --- Webhook output fields (Type == OutputWebhook) ---

	// URL is the endpoint to POST each event to.
	URL *amcommoncfg.SecretURL `yaml:"url,omitempty" json:"url,omitempty"`
	// HTTPConfig configures the HTTP client used for webhook delivery.
	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	// Timeout for webhook HTTP requests (default 10s).
	Timeout model.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// Workers is the number of concurrent webhook delivery goroutines
	// (default 4).
	Workers int `yaml:"workers,omitempty" json:"workers,omitempty"`
	// MaxRetries is the maximum number of delivery attempts per event
	// (default 3).
	MaxRetries int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	// RetryBackoff is the base backoff duration between retry attempts
	// (default 500ms).  Successive attempts use exponential backoff
	// (base * 2^attempt).
	RetryBackoff model.Duration `yaml:"retry_backoff,omitempty" json:"retry_backoff,omitempty"`

	// --- Kafka output fields (Type == OutputKafka) ---

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
// It dispatches to per-type validators defined in each output's file.
func (o *Output) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Output
	if err := unmarshal((*plain)(o)); err != nil {
		return err
	}
	switch o.Type {
	case OutputFile:
		return o.validateFile()
	case OutputKafka:
		return o.validateKafka()
	case OutputWebhook:
		return o.validateWebhook()
	default:
		return fmt.Errorf("unknown event_recorder output type %q, must be %q, %q, or %q",
			o.Type, OutputFile, OutputWebhook, OutputKafka)
	}
}

// configEqual compares two Config values by their semantically significant
// fields.  For each output, equality is delegated to a per-type helper
// defined alongside that output's implementation.
func configEqual(a, b Config) bool {
	if len(a.Outputs) != len(b.Outputs) {
		return false
	}
	for i := range a.Outputs {
		oa, ob := a.Outputs[i], b.Outputs[i]
		if oa.Type != ob.Type {
			return false
		}
		switch oa.Type {
		case OutputFile:
			if !fileOutputsEqual(oa, ob) {
				return false
			}
		case OutputKafka:
			if !kafkaOutputsEqual(oa, ob) {
				return false
			}
		case OutputWebhook:
			if !webhookOutputsEqual(oa, ob) {
				return false
			}
		default:
			// Unknown types should have been rejected at unmarshal time.
			// Treat as inequality to force a reload.
			return false
		}
	}
	return true
}
