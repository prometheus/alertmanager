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

package msteams

import (
	"errors"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	commoncfg "github.com/prometheus/common/config"
)

var DefaultMSTeamsConfig = MSTeamsConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Title:   `{{ template "msteams.default.title" . }}`,
	Summary: `{{ template "msteams.default.summary" . }}`,
	Text:    `{{ template "msteams.default.text" . }}`,
}

type MSTeamsConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig                 *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	WebhookURL                 *amcommoncfg.SecretURL      `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	WebhookURLFile             string                      `yaml:"webhook_url_file,omitempty" json:"webhook_url_file,omitempty"`

	Title   string `yaml:"title,omitempty" json:"title,omitempty"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	Text    string `yaml:"text,omitempty" json:"text,omitempty"`
}

func (c *MSTeamsConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultMSTeamsConfig
	type plain MSTeamsConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.WebhookURL == nil && c.WebhookURLFile == "" {
		return errors.New("one of webhook_url or webhook_url_file must be configured")
	}

	if c.WebhookURL != nil && len(c.WebhookURLFile) > 0 {
		return errors.New("at most one of webhook_url & webhook_url_file must be configured")
	}

	return nil
}
