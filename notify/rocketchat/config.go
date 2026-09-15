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

package rocketchat

import (
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultRocketchatConfig defines default values for Rocketchat configurations.
var DefaultRocketchatConfig = RocketchatConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: false,
	},
	Color:     `{{ if eq .Status "firing" }}red{{ else }}green{{ end }}`,
	Emoji:     `{{ template "rocketchat.default.emoji" . }}`,
	IconURL:   `{{ template "rocketchat.default.iconurl" . }}`,
	Text:      `{{ template "rocketchat.default.text" . }}`,
	Title:     `{{ template "rocketchat.default.title" . }}`,
	TitleLink: `{{ template "rocketchat.default.titlelink" . }}`,
}

type RocketchatAttachmentField struct {
	Short *bool  `json:"short"`
	Title string `json:"title,omitempty"`
	Value string `json:"value,omitempty"`
}

const (
	ProcessingTypeSendMessage        = "sendMessage"
	ProcessingTypeRespondWithMessage = "respondWithMessage"
)

type RocketchatAttachmentAction struct {
	Type               string `json:"type,omitempty"`
	Text               string `json:"text,omitempty"`
	URL                string `json:"url,omitempty"`
	ImageURL           string `json:"image_url,omitempty"`
	IsWebView          bool   `json:"is_webview"`
	WebviewHeightRatio string `json:"webview_height_ratio,omitempty"`
	Msg                string `json:"msg,omitempty"`
	MsgInChatWindow    bool   `json:"msg_in_chat_window"`
	MsgProcessingType  string `json:"msg_processing_type,omitempty"`
}

// RocketchatConfig configures notifications via Rocketchat.
type RocketchatConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIURL      *amcommoncfg.URL  `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	TokenID     *commoncfg.Secret `yaml:"token_id,omitempty" json:"token_id,omitempty"`
	TokenIDFile string            `yaml:"token_id_file,omitempty" json:"token_id_file,omitempty"`
	Token       *commoncfg.Secret `yaml:"token,omitempty" json:"token,omitempty"`
	TokenFile   string            `yaml:"token_file,omitempty" json:"token_file,omitempty"`

	// RocketChat channel override, (like #other-channel or @username).
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	Color       string                        `yaml:"color,omitempty" json:"color,omitempty"`
	Title       string                        `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string                        `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Text        string                        `yaml:"text,omitempty" json:"text,omitempty"`
	Fields      []*RocketchatAttachmentField  `yaml:"fields,omitempty" json:"fields,omitempty"`
	ShortFields bool                          `yaml:"short_fields" json:"short_fields,omitempty"`
	Emoji       string                        `yaml:"emoji,omitempty" json:"emoji,omitempty"`
	IconURL     string                        `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	ImageURL    string                        `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	ThumbURL    string                        `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	LinkNames   bool                          `yaml:"link_names" json:"link_names,omitempty"`
	Actions     []*RocketchatAttachmentAction `yaml:"actions,omitempty" json:"actions,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RocketchatConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultRocketchatConfig
	type plain RocketchatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

// Validate checks the RocketchatConfig for correctness.
func (c *RocketchatConfig) Validate() error {
	if c.Token != nil && len(c.TokenFile) > 0 {
		return errors.New("at most one of token & token_file must be configured")
	}
	if c.TokenID != nil && len(c.TokenIDFile) > 0 {
		return errors.New("at most one of token_id & token_id_file must be configured")
	}
	return nil
}
