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

package wechat

import (
	"errors"
	"fmt"
	"regexp"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultWechatConfig defines default values for wechat configurations.
var DefaultWechatConfig = WechatConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: false,
	},
	Message: `{{ template "wechat.default.message" . }}`,
	ToUser:  `{{ template "wechat.default.to_user" . }}`,
	ToParty: `{{ template "wechat.default.to_party" . }}`,
	ToTag:   `{{ template "wechat.default.to_tag" . }}`,
	AgentID: `{{ template "wechat.default.agent_id" . }}`,
}

// WechatConfig configures notifications via Wechat.
type WechatConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APISecret     commoncfg.Secret `yaml:"api_secret,omitempty" json:"api_secret,omitempty"`
	APISecretFile string           `yaml:"api_secret_file,omitempty" json:"api_secret_file,omitempty"`
	CorpID        string           `yaml:"corp_id,omitempty" json:"corp_id,omitempty"`
	Message       string           `yaml:"message,omitempty" json:"message,omitempty"`
	APIURL        *amcommoncfg.URL `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	ToUser        string           `yaml:"to_user,omitempty" json:"to_user,omitempty"`
	ToParty       string           `yaml:"to_party,omitempty" json:"to_party,omitempty"`
	ToTag         string           `yaml:"to_tag,omitempty" json:"to_tag,omitempty"`
	AgentID       string           `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	MessageType   string           `yaml:"message_type,omitempty" json:"message_type,omitempty"`
}

const wechatValidTypesRe = `^(text|markdown)$`

var wechatTypeMatcher = regexp.MustCompile(wechatValidTypesRe)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WechatConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultWechatConfig
	type plain WechatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.MessageType == "" {
		c.MessageType = "text"
	}

	return c.Validate()
}

func (c *WechatConfig) Validate() error {
	if !wechatTypeMatcher.MatchString(c.MessageType) {
		return fmt.Errorf("weChat message type %q does not match valid options %s", c.MessageType, wechatValidTypesRe)
	}

	if c.APISecret != "" && len(c.APISecretFile) > 0 {
		return errors.New("at most one of api_secret & api_secret_file must be configured")
	}

	return nil
}
