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

package slack

import (
	"errors"
	"time"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultConfig defines default values for Slack configurations.
var DefaultConfig = Config{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: false,
	},
	Color:      `{{ template "slack.default.color" . }}`,
	Username:   `{{ template "slack.default.username" . }}`,
	Title:      `{{ template "slack.default.title" . }}`,
	TitleLink:  `{{ template "slack.default.titlelink" . }}`,
	IconEmoji:  `{{ template "slack.default.iconemoji" . }}`,
	IconURL:    `{{ template "slack.default.iconurl" . }}`,
	Pretext:    `{{ template "slack.default.pretext" . }}`,
	Text:       `{{ template "slack.default.text" . }}`,
	Fallback:   `{{ template "slack.default.fallback" . }}`,
	CallbackID: `{{ template "slack.default.callbackid" . }}`,
	Footer:     `{{ template "slack.default.footer" . }}`,
}

// Action configures a single Slack action that is sent with each notification.
// See https://api.slack.com/docs/message-attachments#action_fields and https://api.slack.com/docs/message-buttons
// for more information.
type Action struct {
	Type         string             `yaml:"type,omitempty"  json:"type,omitempty"`
	Text         string             `yaml:"text,omitempty"  json:"text,omitempty"`
	URL          string             `yaml:"url,omitempty"   json:"url,omitempty"`
	Style        string             `yaml:"style,omitempty" json:"style,omitempty"`
	Name         string             `yaml:"name,omitempty"  json:"name,omitempty"`
	Value        string             `yaml:"value,omitempty"  json:"value,omitempty"`
	ConfirmField *ConfirmationField `yaml:"confirm,omitempty"  json:"confirm,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Action.
func (a *Action) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Action
	if err := unmarshal((*plain)(a)); err != nil {
		return err
	}
	if a.Type == "" {
		return errors.New("missing type in Slack action configuration")
	}
	if a.Text == "" {
		return errors.New("missing text in Slack action configuration")
	}
	if a.URL != "" {
		// Clear all message action fields.
		a.Name = ""
		a.Value = ""
		a.ConfirmField = nil
	} else if a.Name != "" {
		a.URL = ""
	} else {
		return errors.New("missing name or url in Slack action configuration")
	}
	return nil
}

// ConfirmationField protect users from destructive actions or particularly distinguished decisions
// by asking them to confirm their button click one more time.
// See https://api.slack.com/docs/interactive-message-field-guide#confirmation_fields for more information.
type ConfirmationField struct {
	Text        string `yaml:"text,omitempty"  json:"text,omitempty"`
	Title       string `yaml:"title,omitempty"  json:"title,omitempty"`
	OkText      string `yaml:"ok_text,omitempty"  json:"ok_text,omitempty"`
	DismissText string `yaml:"dismiss_text,omitempty"  json:"dismiss_text,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for ConfirmationField.
func (c *ConfirmationField) UnmarshalYAML(unmarshal func(any) error) error {
	type plain ConfirmationField
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Text == "" {
		return errors.New("missing text in Slack confirmation configuration")
	}
	return nil
}

// Field configures a single Slack field that is sent with each notification.
// Each field must contain a title, value, and optionally, a boolean value to indicate if the field
// is short enough to be displayed next to other fields designated as short.
// See https://api.slack.com/docs/message-attachments#fields for more information.
type Field struct {
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	Short *bool  `yaml:"short,omitempty" json:"short,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Field.
func (f *Field) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Field
	if err := unmarshal((*plain)(f)); err != nil {
		return err
	}
	if f.Title == "" {
		return errors.New("missing title in Slack field configuration")
	}
	if f.Value == "" {
		return errors.New("missing value in Slack field configuration")
	}
	return nil
}

// Config configures notifications via Slack.
type Config struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIURL       *amcommoncfg.SecretURL `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	APIURLFile   string                 `yaml:"api_url_file,omitempty" json:"api_url_file,omitempty"`
	AppToken     commoncfg.Secret       `yaml:"app_token,omitempty" json:"app_token,omitempty"`
	AppTokenFile string                 `yaml:"app_token_file,omitempty" json:"app_token_file,omitempty"`
	AppURL       *amcommoncfg.URL       `yaml:"app_url,omitempty" json:"app_url,omitempty"`

	// Slack channel override, (like #other-channel or @username).
	Channel  string `yaml:"channel,omitempty" json:"channel,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Color    string `yaml:"color,omitempty" json:"color,omitempty"`

	Title       string    `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string    `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Pretext     string    `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text        string    `yaml:"text,omitempty" json:"text,omitempty"`
	MessageText string    `yaml:"message_text,omitempty" json:"message_text,omitempty"`
	Fields      []*Field  `yaml:"fields,omitempty" json:"fields,omitempty"`
	ShortFields bool      `yaml:"short_fields" json:"short_fields,omitempty"`
	Footer      string    `yaml:"footer,omitempty" json:"footer,omitempty"`
	Fallback    string    `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	CallbackID  string    `yaml:"callback_id,omitempty" json:"callback_id,omitempty"`
	IconEmoji   string    `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	IconURL     string    `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	ImageURL    string    `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	ThumbURL    string    `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	LinkNames   bool      `yaml:"link_names" json:"link_names,omitempty"`
	MrkdwnIn    []string  `yaml:"mrkdwn_in,omitempty" json:"mrkdwn_in,omitempty"`
	Actions     []*Action `yaml:"actions,omitempty" json:"actions,omitempty"`

	// UpdateMessage enables updating existing Slack messages instead of creating new ones.
	// Requires bot token with chat:write scope. Webhook URLs do not support updates.

	UpdateMessage bool `yaml:"update_message" json:"update_message,omitempty"`
	// Timeout is the maximum time allowed to invoke the slack. Setting this to 0
	// does not impose a timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultConfig
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.APIURL != nil && len(c.APIURLFile) > 0 {
		return errors.New("at most one of api_url & api_url_file must be configured")
	}
	if c.AppToken != "" && len(c.AppTokenFile) > 0 {
		return errors.New("at most one of app_token & app_token_file must be configured")
	}
	if (c.APIURL != nil || len(c.APIURLFile) > 0) && (c.AppToken != "" || len(c.AppTokenFile) > 0) {
		return errors.New("at most one of api_url/api_url_file & app_token/app_token_file must be configured")
	}

	if c.UpdateMessage {
		if c.APIURL == nil || c.APIURL.String() != "https://slack.com/api/chat.postMessage" {
			return errors.New("update_message can only be used with bot tokens. api_url must be set to https://slack.com/api/chat.postMessage")
		}
	}

	return nil
}
