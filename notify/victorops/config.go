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

package victorops

import (
	"errors"
	"fmt"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultVictorOpsConfig defines default values for Splunk On-Call configurations.
var DefaultVictorOpsConfig = VictorOpsConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	MessageType:       `CRITICAL`,
	StateMessage:      `{{ template "victorops.default.state_message" . }}`,
	EntityDisplayName: `{{ template "victorops.default.entity_display_name" . }}`,
	MonitoringTool:    `{{ template "victorops.default.monitoring_tool" . }}`,
}

// VictorOpsConfig configures notifications through Splunk On-Call.
// The type name is retained for configuration compatibility.
type VictorOpsConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIKey            commoncfg.Secret  `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIKeyFile        string            `yaml:"api_key_file,omitempty" json:"api_key_file,omitempty"`
	APIURL            *amcommoncfg.URL  `yaml:"api_url" json:"api_url"`
	RoutingKey        string            `yaml:"routing_key" json:"routing_key"`
	MessageType       string            `yaml:"message_type" json:"message_type"`
	StateMessage      string            `yaml:"state_message" json:"state_message"`
	EntityDisplayName string            `yaml:"entity_display_name" json:"entity_display_name"`
	MonitoringTool    string            `yaml:"monitoring_tool" json:"monitoring_tool"`
	CustomFields      map[string]string `yaml:"custom_fields,omitempty" json:"custom_fields,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *VictorOpsConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultVictorOpsConfig
	type plain VictorOpsConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

func (c *VictorOpsConfig) Validate() error {
	if c.RoutingKey == "" {
		return errors.New("missing Routing key in VictorOps config")
	}
	if c.APIKey != "" && len(c.APIKeyFile) > 0 {
		return errors.New("at most one of api_key & api_key_file must be configured")
	}

	reservedFields := []string{"routing_key", "message_type", "state_message", "entity_display_name", "monitoring_tool", "entity_id", "entity_state"}

	for _, v := range reservedFields {
		if _, ok := c.CustomFields[v]; ok {
			return fmt.Errorf("victorOps config contains custom field %s which cannot be used as it conflicts with the fixed/static fields", v)
		}
	}

	return nil
}
