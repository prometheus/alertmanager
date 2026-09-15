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

package pushover

import (
	"errors"
	"time"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultPushoverConfig defines default values for Pushover configurations.
var DefaultPushoverConfig = PushoverConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Title:    `{{ template "pushover.default.title" . }}`,
	Message:  `{{ template "pushover.default.message" . }}`,
	URL:      `{{ template "pushover.default.url" . }}`,
	Priority: `{{ if eq .Status "firing" }}2{{ else }}0{{ end }}`, // emergency (firing) or normal
	Retry:    duration(1 * time.Minute),
	Expire:   duration(1 * time.Hour),
	HTML:     false,
}

type duration time.Duration

func (d *duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err == nil {
		*d = duration(parsed)
	}
	return err
}

func (d duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

type PushoverConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	UserKey     commoncfg.Secret `yaml:"user_key,omitempty" json:"user_key,omitempty"`
	UserKeyFile string           `yaml:"user_key_file,omitempty" json:"user_key_file,omitempty"`
	Token       commoncfg.Secret `yaml:"token,omitempty" json:"token,omitempty"`
	TokenFile   string           `yaml:"token_file,omitempty" json:"token_file,omitempty"`
	Title       string           `yaml:"title,omitempty" json:"title,omitempty"`
	Message     string           `yaml:"message,omitempty" json:"message,omitempty"`
	URL         string           `yaml:"url,omitempty" json:"url,omitempty"`
	URLTitle    string           `yaml:"url_title,omitempty" json:"url_title,omitempty"`
	Device      string           `yaml:"device,omitempty" json:"device,omitempty"`
	Sound       string           `yaml:"sound,omitempty" json:"sound,omitempty"`
	Priority    string           `yaml:"priority,omitempty" json:"priority,omitempty"`
	Retry       duration         `yaml:"retry,omitempty" json:"retry,omitempty"`
	Expire      duration         `yaml:"expire,omitempty" json:"expire,omitempty"`
	TTL         duration         `yaml:"ttl,omitempty" json:"ttl,omitempty"`
	HTML        bool             `yaml:"html,omitempty" json:"html,omitempty"`
	Monospace   bool             `yaml:"monospace,omitempty" json:"monospace,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PushoverConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultPushoverConfig
	type plain PushoverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

// Validate checks the PushoverConfig for correctness.
func (c *PushoverConfig) Validate() error {
	if c.UserKey == "" && c.UserKeyFile == "" {
		return errors.New("one of user_key or user_key_file must be configured")
	}
	if c.UserKey != "" && c.UserKeyFile != "" {
		return errors.New("at most one of user_key & user_key_file must be configured")
	}
	if c.Token == "" && c.TokenFile == "" {
		return errors.New("one of token or token_file must be configured")
	}
	if c.Token != "" && c.TokenFile != "" {
		return errors.New("at most one of token & token_file must be configured")
	}
	if c.HTML && c.Monospace {
		return errors.New("at most one of monospace & html must be configured")
	}
	return nil
}
