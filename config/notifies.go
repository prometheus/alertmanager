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
	"strings"
)

var (
	// DefaultHipchatConfig defines default values for Hipchat configurations.
	DefaultHipchatConfig = HipchatConfig{
		Color:         `{{ if eq .Status "firing" }}purple{{ else }}green{{ end }}`,
		MessageFormat: HipchatFormatHTML,
	}

	// DefaultSlackConfig defines default values for Slack configurations.
	DefaultSlackConfig = SlackConfig{
		Color:     `{{ if eq .Status "firing" }}warning{{ else }}good{{ end }}`,
		Title:     `{{template "slack.default.title" . }}`,
		TitleLink: `{{template "slack.default.title_link" . }}`,
		Pretext:   `{{template "slack.default.pretext" . }}`,
		Text:      `{{template "slack.default.text" . }}`,
		Fallback:  `{{template "slack.default.fallback" . }}`,
	}

	// DefaultEmailConfig defines default values for Email configurations.
	DefaultEmailConfig = EmailConfig{
		HTML: `{{template "email.default.html" .}}`,
	}
	DefaultEmailSubject = `{{template "email.default.subject" .}}`

	// DefaultPagerdutyConfig defines default values for PagerDuty configurations.
	DefaultPagerdutyConfig = PagerdutyConfig{
		Description: `{{template "pagerduty.default.description" .}}`,
		// TODO: Add a details field with all the alerts.
	}
)

// PagerdutyConfig configures notifications via PagerDuty.
type PagerdutyConfig struct {
	ServiceKey  string            `yaml:"service_key"`
	URL         string            `yaml:"url"`
	Description string            `yaml:"description"`
	Details     map[string]string `yaml:"details"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultPagerdutyConfig
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.ServiceKey == "" {
		return fmt.Errorf("missing service key in PagerDuty config")
	}
	return checkOverflow(c.XXX, "pagerduty config")
}

// EmailConfig configures notifications via mail.
type EmailConfig struct {
	// Email address to notify.
	To        string            `yaml:"to"`
	From      string            `yaml:"from"`
	Smarthost string            `yaml:"smarthost,omitempty"`
	Headers   map[string]string `yaml:"headers"`
	HTML      string            `yaml:"html"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultEmailConfig
	type plain EmailConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.To == "" {
		return fmt.Errorf("missing to address in email config")
	}

	// Header names are case-insensitive, check for collisions.
	normalisedHeaders := map[string]string{}
	for h, v := range c.Headers {
		normalised := strings.ToTitle(h)
		if _, ok := normalisedHeaders[normalised]; ok {
			return fmt.Errorf("duplicate header %q in email config", normalised)
		}
		normalisedHeaders[normalised] = v
	}
	c.Headers = normalisedHeaders
	if _, ok := c.Headers["Subject"]; !ok {
		c.Headers["Subject"] = DefaultEmailSubject
	}
	if _, ok := c.Headers["To"]; !ok {
		c.Headers["To"] = c.To
	}
	if _, ok := c.Headers["From"]; !ok {
		c.Headers["From"] = c.From
	}

	return checkOverflow(c.XXX, "email config")
}

// PushoverConfig configures notifications via PushOver.
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

// HipchatFormat defines text formats for Hipchat.
type HipchatFormat string

const (
	HipchatFormatHTML HipchatFormat = "html"
	HipchatFormatText HipchatFormat = "text"
)

// HipchatConfig configures notifications via Hipchat.
// https://www.hipchat.com/docs/apiv2/method/send_room_notification
type HipchatConfig struct {
	// HipChat auth token, (https://www.hipchat.com/docs/api/auth).
	AuthToken string `yaml:"auth_token"`

	// HipChat room id, (https://www.hipchat.com/rooms/ids).
	RoomID int `yaml:"room_id"`

	// The message color.
	Color string `yaml:"color"`

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

// SlackConfig configures notifications via Slack.
type SlackConfig struct {
	URL string `yaml:"url"`

	// Slack channel override, (like #other-channel or @username).
	Channel string `yaml:"channel"`

	// The message color.
	Color string `yaml:"color"`

	Title     string `yaml:"title"`
	TitleLink string `yaml:"title_link"`
	Pretext   string `yaml:"pretext"`
	Text      string `yaml:"text"`
	Fallback  string `yaml:"fallback"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Channel == "" {
		return fmt.Errorf("missing channel in Slack config")
	}
	return checkOverflow(c.XXX, "slack config")
}

// FlowdockConfig configures notifications via Flowdock.
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

// WebhookConfig configures notifications via a generic webhook.
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
