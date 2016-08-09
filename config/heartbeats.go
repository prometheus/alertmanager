// Copyright 2015 Prometheus Team
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

package config

import (
	"fmt"
	"time"
)

var (
	DefaultHeartbeatOpsGenieConfig = HeartbeatOpsGenieConfig{
		Interval: duration(1 * time.Minute),
	}
)

// HeartbeatOpsGenieConfig configures heartbeats to OpsGenie.
type HeartbeatOpsGenieConfig struct {
	APIKey   Secret   `yaml:"api_key"`
	APIHost  string   `yaml:"api_host"`
	Name     string   `yaml:"name"`
	Interval duration `yaml:"interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *HeartbeatOpsGenieConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultHeartbeatOpsGenieConfig
	type plain HeartbeatOpsGenieConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing hearbeat name in OpsGenie heartbeat config")
	}
	if c.APIKey == "" {
		return fmt.Errorf("missing API key in OpsGenie heartbeat config")
	}
	return checkOverflow(c.XXX, "opsgenie heartbeat config")
}
