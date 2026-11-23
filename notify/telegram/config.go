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

package telegram

import (
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

var DefaultTelegramConfig = TelegramConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	DisableNotifications: false,
	Message:              `{{ template "telegram.default.message" . }}`,
	ParseMode:            "HTML",
}

// TelegramConfig configures notifications via Telegram.
type TelegramConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIUrl               *amcommoncfg.URL `yaml:"api_url" json:"api_url,omitempty"`
	BotToken             commoncfg.Secret `yaml:"bot_token,omitempty" json:"token,omitempty"`
	BotTokenFile         string           `yaml:"bot_token_file,omitempty" json:"token_file,omitempty"`
	ChatID               int64            `yaml:"chat_id,omitempty" json:"chat,omitempty"`
	ChatIDFile           string           `yaml:"chat_id_file,omitempty" json:"chat_file,omitempty"`
	MessageThreadID      int              `yaml:"message_thread_id,omitempty" json:"message_thread_id,omitempty"`
	Message              string           `yaml:"message,omitempty" json:"message,omitempty"`
	DisableNotifications bool             `yaml:"disable_notifications,omitempty" json:"disable_notifications,omitempty"`
	ParseMode            string           `yaml:"parse_mode,omitempty" json:"parse_mode,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *TelegramConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultTelegramConfig
	type plain TelegramConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.BotToken != "" && c.BotTokenFile != "" {
		return errors.New("at most one of bot_token & bot_token_file must be configured")
	}
	if c.ChatID == 0 && c.ChatIDFile == "" {
		return errors.New("missing chat_id or chat_id_file on telegram_config")
	}
	if c.ChatID != 0 && c.ChatIDFile != "" {
		return errors.New("at most one of chat_id & chat_id_file must be configured")
	}
	if c.ParseMode != "" &&
		c.ParseMode != "Markdown" &&
		c.ParseMode != "MarkdownV2" &&
		c.ParseMode != "HTML" {
		return errors.New("unknown parse_mode on telegram_config, must be Markdown, MarkdownV2, HTML or empty string")
	}
	return nil
}
