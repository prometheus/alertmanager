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

package webhook

import (
	"errors"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config/amcommonconfig"
)

// WebhookConfig configures notifications via a generic webhook.
type WebhookConfig struct {
	amcommonconfig.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// URL to send POST request to.
	URL     amcommonconfig.SecretTemplateURL `yaml:"url,omitempty" json:"url,omitempty"`
	URLFile string                           `yaml:"url_file" json:"url_file"`

	// MaxAlerts is the maximum number of alerts to be sent per webhook message.
	// Alerts exceeding this threshold will be truncated. Setting this to 0
	// allows an unlimited number of alerts.
	MaxAlerts uint64 `yaml:"max_alerts" json:"max_alerts"`

	// Timeout is the maximum time allowed to invoke the webhook. Setting this to 0
	// does not impose a timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebhookConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultWebhookConfig
	type plain WebhookConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == "" && c.URLFile == "" {
		return errors.New("one of url or url_file must be configured")
	}
	if c.URL != "" && c.URLFile != "" {
		return errors.New("at most one of url & url_file must be configured")
	}
	return nil
}

// DefaultWebhookConfig defines default values for Webhook configurations.
var DefaultWebhookConfig = WebhookConfig{
	NotifierConfig: amcommonconfig.NotifierConfig{
		VSendResolved: true,
	},
}
