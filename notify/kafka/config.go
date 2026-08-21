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
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	sharedkafka "github.com/prometheus/alertmanager/kafka"
)

var defaultConfig = Config{
	NotifierConfig: amcommoncfg.NotifierConfig{VSendResolved: true},
}

// Config configures notifications sent to a Kafka topic.
type Config struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	Brokers     []string                `yaml:"brokers" json:"brokers"`
	Topic       string                  `yaml:"topic" json:"topic"`
	ClientID    string                  `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	Acks        sharedkafka.Acks        `yaml:"acks,omitempty" json:"acks,omitempty"`
	Compression sharedkafka.Compression `yaml:"compression,omitempty" json:"compression,omitempty"`
	TLSConfig   *commoncfg.TLSConfig    `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	*c = defaultConfig
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.validate()
}

func (c Config) validate() error {
	if err := c.clientOptions().Validate(); err != nil {
		return err
	}
	if c.Topic == "" {
		return errors.New("kafka: topic is required")
	}
	return nil
}

func (c Config) clientOptions() sharedkafka.ClientOptions {
	return sharedkafka.ClientOptions{
		Brokers:     c.Brokers,
		Topic:       c.Topic,
		ClientID:    c.ClientID,
		Acks:        c.Acks,
		Compression: c.Compression,
		TLSConfig:   c.TLSConfig,
	}
}
