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
	"fmt"
	"time"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

const (
	// PublishingStrategyNew sends each notification as a separate message (default).
	PublishingStrategyNew PublishingStrategy = "new"
	// PublishingStrategyUpdate updates the original message in-place.
	PublishingStrategyUpdate PublishingStrategy = "update"
	// PublishingStrategyThread sends subsequent notifications as threaded replies.
	PublishingStrategyThread PublishingStrategy = "thread"
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

// PublishingStrategy controls how subsequent notifications for the same
// alert group are delivered to Slack.
type PublishingStrategy string

// SlackThreadedOptions configures thread-specific behavior when message_strategy is "thread".
type SlackThreadedOptions struct {
	// ResolveEmoji is the emoji name (without colons) to react with on the
	// original thread message when all alerts in the group are resolved.
	// Requires the bot token to have reactions:write scope.
	ResolveEmoji string `yaml:"resolve_emoji,omitempty" json:"resolve_emoji,omitempty"`

	// UseSummaryHeader controls whether the thread parent is a lightweight
	// auto-updated summary (true, default) or the first actual alert message
	// (false). When true, all alert content is posted as replies and the parent
	// is continuously updated with the transition title and color.
	UseSummaryHeader *bool `yaml:"use_summary_header,omitempty" json:"use_summary_header,omitempty"`

	// SummaryHeader holds options for summary-header mode only (see UseSummaryHeader).
	SummaryHeader *SlackThreadSummaryHeaderOptions `yaml:"summary_header,omitempty" json:"summary_header,omitempty"`
}

// SlackThreadSummaryHeaderOptions configures fields that only apply when
// message_strategy is "thread" and use_summary_header is true (the lightweight
// parent summary mode).
type SlackThreadSummaryHeaderOptions struct {
	// ResolveColor overrides the parent summary attachment color when the alert
	// group resolves. Supports Go templates.
	ResolveColor string `yaml:"resolve_color,omitempty" json:"resolve_color,omitempty"`
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

	// MessageStrategy controls how subsequent notifications for the same alert
	// group are delivered: "new" (default), "update" (edit in-place), or "thread"
	// (threaded replies). "update" and "thread" require a bot token.
	MessageStrategy PublishingStrategy `yaml:"message_strategy,omitempty" json:"message_strategy,omitempty"`

	// UpdateMessage enables updating existing Slack messages instead of creating new ones.
	// Deprecated: use message_strategy: update instead. If true, message_strategy must
	// be unset or "update"; when message_strategy is unset it is treated as "update".
	UpdateMessage bool `yaml:"update_message,omitempty" json:"update_message,omitempty"`

	// ThreadedOptions configures thread-specific behavior.
	// Only valid when message_strategy is "thread".
	ThreadedOptions *SlackThreadedOptions `yaml:"threaded_options,omitempty" json:"threaded_options,omitempty"`
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

	// Deprecated: remove this block when update_message is deleted.
	if c.UpdateMessage {
		if c.MessageStrategy != "" && c.MessageStrategy != PublishingStrategyUpdate {
			return fmt.Errorf("update_message: true is incompatible with message_strategy %q; omit message_strategy or set message_strategy: \"update\"", c.MessageStrategy)
		}
		if c.MessageStrategy == "" {
			c.MessageStrategy = PublishingStrategyUpdate
		}
	}
	if c.MessageStrategy == "" {
		c.MessageStrategy = PublishingStrategyNew
	}

	if c.MessageStrategy != PublishingStrategyNew &&
		c.MessageStrategy != PublishingStrategyUpdate &&
		c.MessageStrategy != PublishingStrategyThread {
		return fmt.Errorf("unknown message_strategy %q; must be \"new\", \"update\", or \"thread\"", c.MessageStrategy)
	}

	if c.ThreadedOptions != nil {
		if c.MessageStrategy != PublishingStrategyThread {
			return errors.New("threaded_options requires message_strategy to be \"thread\"")
		}
		if c.ThreadedOptions.UseSummaryHeader != nil && !*c.ThreadedOptions.UseSummaryHeader && c.ThreadedOptions.SummaryHeader != nil {
			return errors.New("threaded_options.summary_header requires use_summary_header to be enabled")
		}
	}

	return nil
}

// ValidateMessageStrategy checks that the resolved api_url (after global defaults
// have been applied) satisfies the requirements for update/thread strategies.
func (c *Config) ValidateMessageStrategy() error {
	switch c.MessageStrategy {
	case PublishingStrategyUpdate, PublishingStrategyThread:
		if c.APIURL == nil && c.APIURLFile == "" {
			return fmt.Errorf("message_strategy %q requires api_url or api_url_file", c.MessageStrategy)
		}
		if c.APIURL != nil && c.APIURL.String() != "https://slack.com/api/chat.postMessage" {
			return fmt.Errorf("message_strategy %q requires a bot token; api_url must be https://slack.com/api/chat.postMessage", c.MessageStrategy)
		}
	}
	return nil
}

// UseSummaryHeaderInThread returns true when the thread parent should be a lightweight
// auto-updated summary. Returns true by default (nil or explicit true).
func (c *Config) UseSummaryHeaderInThread() bool {
	if c.ThreadedOptions == nil || c.ThreadedOptions.UseSummaryHeader == nil {
		return true
	}
	return *c.ThreadedOptions.UseSummaryHeader
}

// HasStrategyThatUpdatesParent reports whether message_strategy uses nflog to tie
// multiple notifications to one Slack message or thread.
func (c *Config) HasStrategyThatUpdatesParent() bool {
	return c.HasUpdateStrategy() || c.HasThreadStrategy()
}

// HasUpdateStrategy is true when message_strategy is "update" (chat.update in place).
func (c *Config) HasUpdateStrategy() bool {
	return c.MessageStrategy == PublishingStrategyUpdate
}

// HasThreadStrategy is true when message_strategy is "thread" (threaded replies).
func (c *Config) HasThreadStrategy() bool {
	return c.MessageStrategy == PublishingStrategyThread
}
