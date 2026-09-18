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

package webex

import (
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultWebexConfig defines default values for Webex configurations.
var DefaultWebexConfig = WebexConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Message: `{{ template "webex.default.message" . }}`,
}

// WebexConfig configures notifications via Webex.
type WebexConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig                 *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIURL                     *amcommoncfg.URL            `yaml:"api_url,omitempty" json:"api_url,omitempty"`

	Message string `yaml:"message,omitempty" json:"message,omitempty"`
	RoomID  string `yaml:"room_id" json:"room_id"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebexConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultWebexConfig
	type plain WebexConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

func (c *WebexConfig) Validate() error {
	if c.RoomID == "" {
		return errors.New("missing room_id on webex_config")
	}

	if c.HTTPConfig == nil || c.HTTPConfig.Authorization == nil {
		return errors.New("missing webex_configs.http_config.authorization")
	}

	return nil
}
