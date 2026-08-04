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
)

const maxOutputNameLength = 128

// Config configures the event recorder feature.
//
// Outputs are grouped by type, one list per destination kind, mirroring
// the way receivers group their integrations (e.g. webhook_configs,
// email_configs).  Every recorded event is fanned out to every output
// across all lists.
type Config struct {
	FileOutputs    []FileOutputConfig    `yaml:"file_outputs,omitempty" json:"file_outputs,omitempty"`
	WebhookOutputs []WebhookOutputConfig `yaml:"webhook_outputs,omitempty" json:"webhook_outputs,omitempty"`
	KafkaOutputs   []KafkaOutputConfig   `yaml:"kafka_outputs,omitempty" json:"kafka_outputs,omitempty"`
	StdoutOutputs  []StdoutOutputConfig  `yaml:"stdout_outputs,omitempty" json:"stdout_outputs,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface, validating that
// each output identifier is unique.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.validate()
}

func (c Config) validate() error {
	seen := make(map[string]struct{}, c.totalOutputs())
	add := func(kind, name string) error {
		id, err := outputIdentifier(kind, name)
		if err != nil {
			return err
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("event_recorder output name %q is duplicated for type %s", name, kind)
		}
		seen[id] = struct{}{}
		return nil
	}
	for _, out := range c.FileOutputs {
		if err := add("file", out.Name); err != nil {
			return err
		}
	}
	for _, out := range c.WebhookOutputs {
		if err := add("webhook", out.Name); err != nil {
			return err
		}
	}
	for _, out := range c.KafkaOutputs {
		if err := add("kafka", out.Name); err != nil {
			return err
		}
	}
	for _, out := range c.StdoutOutputs {
		if err := add("stdout", out.Name); err != nil {
			return err
		}
	}
	return nil
}

func outputIdentifier(kind, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("event_recorder %s output requires a name", kind)
	}
	if len(name) > maxOutputNameLength {
		return "", fmt.Errorf("event_recorder %s output name must not exceed %d characters", kind, maxOutputNameLength)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return "", fmt.Errorf("event_recorder %s output name must contain only letters, digits, hyphens, underscores, and periods", kind)
		}
	}
	return kind + ":" + name, nil
}

func safeOutputIdentifier(kind, name string) string {
	id, err := outputIdentifier(kind, name)
	if err != nil {
		return kind + ":<invalid>"
	}
	return id
}

// totalOutputs returns the number of configured outputs across all
// destination kinds.
func (c Config) totalOutputs() int {
	return len(c.FileOutputs) + len(c.WebhookOutputs) + len(c.KafkaOutputs) + len(c.StdoutOutputs)
}

// configEqual compares two Config values by their semantically
// significant fields.  Each per-type list is compared element-wise via
// the type's equal helper (defined alongside that output's
// implementation in file.go, webhook.go, kafka.go).
func configEqual(a, b Config) bool {
	if len(a.FileOutputs) != len(b.FileOutputs) ||
		len(a.WebhookOutputs) != len(b.WebhookOutputs) ||
		len(a.KafkaOutputs) != len(b.KafkaOutputs) ||
		len(a.StdoutOutputs) != len(b.StdoutOutputs) {
		return false
	}
	for i := range a.FileOutputs {
		if !a.FileOutputs[i].equal(b.FileOutputs[i]) {
			return false
		}
	}
	for i := range a.WebhookOutputs {
		if !a.WebhookOutputs[i].equal(b.WebhookOutputs[i]) {
			return false
		}
	}
	for i := range a.KafkaOutputs {
		if !a.KafkaOutputs[i].equal(b.KafkaOutputs[i]) {
			return false
		}
	}
	for i := range a.StdoutOutputs {
		if !a.StdoutOutputs[i].equal(b.StdoutOutputs[i]) {
			return false
		}
	}
	return true
}
