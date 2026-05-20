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

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
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
	default:
		return fmt.Errorf("unknown event_recorder output type %q, must be %q or %q", o.Type, OutputFile, OutputWebhook)
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
	}
	return true
}
