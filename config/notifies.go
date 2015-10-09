// Copyright 2015 Prometheus Team
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

package config

import (
	"fmt"
)

var (
	DefaultHipchatConfig = HipchatConfig{
		ColorFiring:   "purple",
		ColorResolved: "green",
		MessageFormat: HipchatFormatHTML,
	}

	DefaultSlackConfig = SlackConfig{
		ColorFiring:   "warning",
		ColorResolved: "good",

		Templates: SlackTemplates{
			Title:     "slack_default_title",
			TitleLink: "slack_default_title_link",
			Pretext:   "slack_default_pretext",
			Text:      "slack_default_text",
			Fallback:  "slack_default_fallback",
		},
	}

	DefaultEmailConfig = EmailConfig{
		Templates: EmailTemplates{
			HTML:  "email_default_html",
			Plain: "email_default_plain",
		},
	}
)

// Configuration for notification via PagerDuty.
type PagerdutyConfig struct {
	// PagerDuty service key, see:
	// http://developer.pagerduty.com/documentation/integration/events
	ServiceKey string `yaml:"service_key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.ServiceKey == "" {
		return fmt.Errorf("missing service key in PagerDuty config")
	}
	return checkOverflow(c.XXX, "pagerduty config")
}

// Configuration for notification via mail.
type EmailConfig struct {
	// Email address to notify.
	Email     string `yaml:"email"`
	Smarthost string `yaml:"smarthost,omitempty"`
	Sender    string `yaml:"sender"`

	Templates EmailTemplates `yaml:"templates"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

type EmailTemplates struct {
	HTML  string `yaml:"html"`
	Plain string `yaml:"plain"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain EmailConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Email == "" {
		return fmt.Errorf("missing email address in email config")
	}
	if c.Sender == "" {
		return fmt.Errorf("missing SMTP sender in email config")
	}
	if c.Smarthost == "" {
		return fmt.Errorf("missing smart host in email config")
	}
	return checkOverflow(c.XXX, "email config")
}

// Configuration for notification via pushover.net.
type PushoverConfig struct {
	// Pushover token.
	Token string `yaml:"token"`

	// Pushover user_key.
	UserKey string `yaml:"user_key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PushoverConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PushoverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Token == "" {
		return fmt.Errorf("missing token in Pushover config")
	}
	if c.UserKey == "" {
		return fmt.Errorf("missing user key in Pushover config")
	}
	return checkOverflow(c.XXX, "pushover config")
}

type HipchatFormat string

const (
	HipchatFormatHTML HipchatFormat = "html"
	HipchatFormatText HipchatFormat = "text"
)

// Configuration for notification via HipChat.
// https://www.hipchat.com/docs/apiv2/method/send_room_notification
type HipchatConfig struct {
	// HipChat auth token, (https://www.hipchat.com/docs/api/auth).
	AuthToken string `yaml:"auth_token"`

	// HipChat room id, (https://www.hipchat.com/rooms/ids).
	RoomID int `yaml:"room_id"`

	// The message colors.
	ColorFiring   string `yaml:"color_firing"`
	ColorResolved string `yaml:"color_resolved"`

	// Should this message notify or not.
	Notify bool `yaml:"notify"`

	// Prefix to be put in front of the message (useful for @mentions, etc.).
	Prefix string `yaml:"prefix"`

	// Format the message as "html" or "text".
	MessageFormat HipchatFormat `yaml:"message_format"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *HipchatConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultHipchatConfig
	type plain HipchatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.AuthToken == "" {
		return fmt.Errorf("missing auth token in HipChat config")
	}
	if c.MessageFormat != HipchatFormatHTML && c.MessageFormat != HipchatFormatText {
		return fmt.Errorf("invalid message format %q", c.MessageFormat)
	}
	return checkOverflow(c.XXX, "hipchat config")
}

// Configuration for notification via Slack.
type SlackConfig struct {
	URL string `yaml:"url"`

	// Slack channel override, (like #other-channel or @username).
	Channel string `yaml:"channel"`

	// The message colors.
	ColorFiring   string `yaml:"color_firing"`
	ColorResolved string `yaml:"color_resolved"`

	Templates SlackTemplates `yaml:"templates"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

type SlackTemplates struct {
	Title     string `yaml:"title"`
	TitleLink string `yaml:"title_link"`
	Pretext   string `yaml:"pretext"`
	Text      string `yaml:"text"`
	Fallback  string `yaml:"fallback"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == "" {
		return fmt.Errorf("missing URL in Slack config")
	}
	if c.Channel == "" {
		return fmt.Errorf("missing channel in Slack config")
	}
	return checkOverflow(c.XXX, "slack config")
}

// Configuration for notification via Flowdock.
type FlowdockConfig struct {
	// Flowdock flow API token.
	APIToken string `yaml:"api_token"`

	// Flowdock from_address.
	FromAddress string `yaml:"from_address"`

	// Flowdock flow tags.
	Tags []string `yaml:"tags"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *FlowdockConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain FlowdockConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.APIToken == "" {
		return fmt.Errorf("missing API token in Flowdock config")
	}
	if c.FromAddress == "" {
		return fmt.Errorf("missing from address in Flowdock config")
	}
	return checkOverflow(c.XXX, "flowdock config")
}

// Configuration for notification via generic webhook.
type WebhookConfig struct {
	// URL to send POST request to.
	URL string `yaml:"url"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebhookConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain WebhookConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == "" {
		return fmt.Errorf("missing URL in webhook config")
	}
	return checkOverflow(c.XXX, "slack config")
}
