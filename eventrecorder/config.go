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
