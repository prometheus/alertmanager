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

package gotify

import (
	"errors"
	"time"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	commoncfg "github.com/prometheus/common/config"
)

var defaultGotifyConfig = GotifyConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Title:       `{{ template "gotify.default.title" . }}`,
	Message:     `{{ template "gotify.default.message" . }}`,
	Priority:    `{{ if eq .Status "firing" }}5{{ else }}0{{ end }}`,
	ContentType: "text/plain",
}

type GotifyConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig                 *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	URL                        *amcommoncfg.URL            `yaml:"url,omitempty" json:"url,omitempty"`
	URLFile                    string                      `yaml:"url_file,omitempty" json:"url_file,omitempty"`

	Token     commoncfg.Secret `yaml:"token,omitempty" json:"token,omitempty"`
	TokenFile string           `yaml:"token_file,omitempty" json:"token_file,omitempty"`

	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	Message     string `yaml:"message,omitempty" json:"message,omitempty"`
	Priority    string `yaml:"priority,omitempty" json:"priority,omitempty"`
	ContentType string `yaml:"content_type,omitempty" json:"content_type,omitempty"`

	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

func (c *GotifyConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = defaultGotifyConfig
	type plain GotifyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.URL == nil && c.URLFile == "" {
		return errors.New("one of url or url_file must be configured")
	}
	if c.URL != nil && c.URLFile != "" {
		return errors.New("at most one of url & url_file must be configured")
	}
	if c.Token == "" && c.TokenFile == "" {
		return errors.New("one of token or token_file must be configured")
	}
	if c.Token != "" && c.TokenFile != "" {
		return errors.New("at most one of token & token_file must be configured")
	}
	if c.ContentType == "" {
		c.ContentType = "text/plain"
	}

	return nil
}
