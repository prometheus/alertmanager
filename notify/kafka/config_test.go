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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	sharedkafka "github.com/prometheus/alertmanager/kafka"
)

func TestConfigUnmarshalYAML(t *testing.T) {
	var c Config
	require.NoError(t, yaml.Unmarshal([]byte(`
brokers: [kafka-1:9092, kafka-2:9092]
topic: alerts
client_id: alertmanager-test
acks: all
compression: zstd
`), &c))
	require.True(t, c.SendResolved())
	require.Equal(t, []string{"kafka-1:9092", "kafka-2:9092"}, c.Brokers)
	require.Equal(t, "alerts", c.Topic)
	require.Equal(t, "alertmanager-test", c.ClientID)
	require.Equal(t, sharedkafka.AcksAll, c.Acks)
	require.Equal(t, sharedkafka.CompressionZstd, c.Compression)
}

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		err  string
	}{
		{name: "missing brokers", yaml: "topic: alerts", err: "at least one broker"},
		{name: "empty broker", yaml: "brokers: ['']\ntopic: alerts", err: "broker entries must be non-empty"},
		{name: "missing topic", yaml: "brokers: [kafka:9092]", err: "topic is required"},
		{name: "invalid acks", yaml: "brokers: [kafka:9092]\ntopic: alerts\nacks: majority", err: "unknown acks"},
		{name: "invalid compression", yaml: "brokers: [kafka:9092]\ntopic: alerts\ncompression: deflate", err: "unknown compression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			err := yaml.Unmarshal([]byte(tc.yaml), &c)
			require.ErrorContains(t, err, tc.err)
		})
	}
}
