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

package tracing

import (
	"errors"
	"fmt"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

// TODO: probably move these into prometheus/common since they're copied from
// prometheus/prometheus?

type TracingClientType string

const (
	TracingClientHTTP TracingClientType = "http"
	TracingClientGRPC TracingClientType = "grpc"

	GzipCompression = "gzip"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TracingClientType) UnmarshalYAML(unmarshal func(any) error) error {
	*t = TracingClientType("")
	type plain TracingClientType
	if err := unmarshal((*plain)(t)); err != nil {
		return err
	}

	switch *t {
	case TracingClientHTTP, TracingClientGRPC:
		return nil
	default:
		return fmt.Errorf("expected tracing client type to be to be %s or %s, but got %s",
			TracingClientHTTP, TracingClientGRPC, *t,
		)
	}
}

// TracingConfig configures the tracing options.
type TracingConfig struct {
	ClientType       TracingClientType    `yaml:"client_type,omitempty"`
	Endpoint         string               `yaml:"endpoint,omitempty"`
	SamplingFraction float64              `yaml:"sampling_fraction,omitempty"`
	Insecure         bool                 `yaml:"insecure,omitempty"`
	TLSConfig        *commoncfg.TLSConfig `yaml:"tls_config,omitempty"`
	Headers          *commoncfg.Headers   `yaml:"headers,omitempty"`
	Compression      string               `yaml:"compression,omitempty"`
	Timeout          model.Duration       `yaml:"timeout,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (t *TracingConfig) SetDirectory(dir string) {
	t.TLSConfig.SetDirectory(dir)
	t.Headers.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TracingConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*t = TracingConfig{
		ClientType: TracingClientGRPC,
	}
	type plain TracingConfig
	if err := unmarshal((*plain)(t)); err != nil {
		return err
	}

	if t.Endpoint == "" {
		return errors.New("tracing endpoint must be set")
	}

	if t.Compression != "" && t.Compression != GzipCompression {
		return fmt.Errorf("invalid compression type %s provided, valid options: %s",
			t.Compression, GzipCompression)
	}

	return nil
}
