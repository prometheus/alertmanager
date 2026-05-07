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

package incidentio

import (
	"errors"
	"time"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	commoncfg "github.com/prometheus/common/config"
)

// defaultIncidentioConfig defines default values for Incident.io configurations.
var defaultIncidentioConfig = IncidentioConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
}

// IncidentioConfig configures notifications via incident.io.
type IncidentioConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// URL to send POST request to.
	URL     *amcommoncfg.URL `yaml:"url" json:"url"`
	URLFile string           `yaml:"url_file" json:"url_file"`

	// AlertSourceToken is the key used to authenticate with the alert source in incident.io.
	AlertSourceToken     commoncfg.Secret `yaml:"alert_source_token,omitempty" json:"alert_source_token,omitempty"`
	AlertSourceTokenFile string           `yaml:"alert_source_token_file,omitempty" json:"alert_source_token_file,omitempty"`

	// MaxAlerts is the maximum number of alerts to be sent per incident.io message.
	// Alerts exceeding this threshold will be truncated. Setting this to 0
	// allows an unlimited number of alerts. Note that if the payload exceeds
	// incident.io's size limits, you will receive a 429 response and alerts
	// will not be ingested.
	MaxAlerts uint64 `yaml:"max_alerts" json:"max_alerts"`

	// Timeout is the maximum time allowed to invoke incident.io. Setting this to 0
	// does not impose a timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *IncidentioConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = defaultIncidentioConfig
	type plain IncidentioConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil && c.URLFile == "" {
		return errors.New("one of url or url_file must be configured")
	}
	if c.URL != nil && c.URLFile != "" {
		return errors.New("at most one of url & url_file must be configured")
	}
	if c.AlertSourceToken != "" && c.AlertSourceTokenFile != "" {
		return errors.New("at most one of alert_source_token & alert_source_token_file must be configured")
	}
	if c.HTTPConfig != nil && c.HTTPConfig.Authorization != nil && (c.AlertSourceToken != "" || c.AlertSourceTokenFile != "") {
		return errors.New("cannot specify alert_source_token or alert_source_token_file when using http_config.authorization")
	}

	if (c.HTTPConfig != nil && c.HTTPConfig.Authorization == nil) && c.AlertSourceToken == "" && c.AlertSourceTokenFile == "" {
		return errors.New("at least one of alert_source_token, alert_source_token_file or http_config.authorization must be configured")
	}
	return nil
}
