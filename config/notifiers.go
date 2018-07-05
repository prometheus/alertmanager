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
	"net/url"
	"strings"
	"time"

	commoncfg "github.com/prometheus/common/config"
)

var (
	// DefaultWebhookConfig defines default values for Webhook configurations.
	DefaultWebhookConfig = WebhookConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: true,
		},
	}

	// DefaultEmailConfig defines default values for Email configurations.
	DefaultEmailConfig = EmailConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: false,
		},
		HTML: `{{ template "email.default.html" . }}`,
		Text: ``,
	}

	// DefaultEmailSubject defines the default Subject header of an Email.
	DefaultEmailSubject = `{{ template "email.default.subject" . }}`

	// DefaultPagerdutyDetails defines the default values for PagerDuty details.
	DefaultPagerdutyDetails = map[string]string{
		"firing":       `{{ template "pagerduty.default.instances" .Alerts.Firing }}`,
		"resolved":     `{{ template "pagerduty.default.instances" .Alerts.Resolved }}`,
		"num_firing":   `{{ .Alerts.Firing | len }}`,
		"num_resolved": `{{ .Alerts.Resolved | len }}`,
	}

	// DefaultPagerdutyConfig defines default values for PagerDuty configurations.
	DefaultPagerdutyConfig = PagerdutyConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: true,
		},
		Description: `{{ template "pagerduty.default.description" .}}`,
		Client:      `{{ template "pagerduty.default.client" . }}`,
		ClientURL:   `{{ template "pagerduty.default.clientURL" . }}`,
	}

	// DefaultSlackConfig defines default values for Slack configurations.
	DefaultSlackConfig = SlackConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: false,
		},
		Color:     `{{ if eq .Status "firing" }}danger{{ else }}good{{ end }}`,
		Username:  `{{ template "slack.default.username" . }}`,
		Title:     `{{ template "slack.default.title" . }}`,
		TitleLink: `{{ template "slack.default.titlelink" . }}`,
		IconEmoji: `{{ template "slack.default.iconemoji" . }}`,
		IconURL:   `{{ template "slack.default.iconurl" . }}`,
		Pretext:   `{{ template "slack.default.pretext" . }}`,
		Text:      `{{ template "slack.default.text" . }}`,
		Fallback:  `{{ template "slack.default.fallback" . }}`,
		Footer:    `{{ template "slack.default.footer" . }}`,
	}

	// DefaultHipchatConfig defines default values for Hipchat configurations.
	DefaultHipchatConfig = HipchatConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: false,
		},
		Color:         `{{ if eq .Status "firing" }}red{{ else }}green{{ end }}`,
		From:          `{{ template "hipchat.default.from" . }}`,
		Notify:        false,
		Message:       `{{ template "hipchat.default.message" . }}`,
		MessageFormat: `text`,
	}

	// DefaultOpsGenieConfig defines default values for OpsGenie configurations.
	DefaultOpsGenieConfig = OpsGenieConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: true,
		},
		Message:     `{{ template "opsgenie.default.message" . }}`,
		Description: `{{ template "opsgenie.default.description" . }}`,
		Source:      `{{ template "opsgenie.default.source" . }}`,
		// TODO: Add a details field with all the alerts.
	}

	// DefaultWechatConfig defines default values for wechat configurations.
	DefaultWechatConfig = WechatConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: false,
		},
		Message:   `{{ template "wechat.default.message" . }}`,
		APISecret: `{{ template "wechat.default.api_secret" . }}`,
		ToUser:    `{{ template "wechat.default.to_user" . }}`,
		ToParty:   `{{ template "wechat.default.to_party" . }}`,
		ToTag:     `{{ template "wechat.default.to_tag" . }}`,
		AgentID:   `{{ template "wechat.default.agent_id" . }}`,
	}

	// DefaultVictorOpsConfig defines default values for VictorOps configurations.
	DefaultVictorOpsConfig = VictorOpsConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: true,
		},
		MessageType:       `CRITICAL`,
		StateMessage:      `{{ template "victorops.default.state_message" . }}`,
		EntityDisplayName: `{{ template "victorops.default.entity_display_name" . }}`,
		MonitoringTool:    `{{ template "victorops.default.monitoring_tool" . }}`,
	}

	// DefaultPushoverConfig defines default values for Pushover configurations.
	DefaultPushoverConfig = PushoverConfig{
		NotifierConfig: NotifierConfig{
			VSendResolved: true,
		},
		Title:    `{{ template "pushover.default.title" . }}`,
		Message:  `{{ template "pushover.default.message" . }}`,
		URL:      `{{ template "pushover.default.url" . }}`,
		Priority: `{{ if eq .Status "firing" }}2{{ else }}0{{ end }}`, // emergency (firing) or normal
		Retry:    duration(1 * time.Minute),
		Expire:   duration(1 * time.Hour),
	}
)

// NotifierConfig contains base options common across all notifier configurations.
type NotifierConfig struct {
	VSendResolved bool `yaml:"send_resolved" json:"send_resolved"`
}

func (nc *NotifierConfig) SendResolved() bool {
	return nc.VSendResolved
}

// EmailConfig configures notifications via mail.
type EmailConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	// Email address to notify.
	To           string            `yaml:"to,omitempty" json:"to,omitempty"`
	From         string            `yaml:"from,omitempty" json:"from,omitempty"`
	Hello        string            `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost    string            `yaml:"smarthost,omitempty" json:"smarthost,omitempty"`
	AuthUsername string            `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	AuthPassword Secret            `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	AuthSecret   Secret            `yaml:"auth_secret,omitempty" json:"auth_secret,omitempty"`
	AuthIdentity string            `yaml:"auth_identity,omitempty" json:"auth_identity,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	HTML         string            `yaml:"html,omitempty" json:"html,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	RequireTLS   *bool             `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
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
	normalizedHeaders := map[string]string{}
	for h, v := range c.Headers {
		normalized := strings.Title(h)
		if _, ok := normalizedHeaders[normalized]; ok {
			return fmt.Errorf("duplicate header %q in email config", normalized)
		}
		normalizedHeaders[normalized] = v
	}
	c.Headers = normalizedHeaders

	return nil
}

// PagerdutyConfig configures notifications via PagerDuty.
type PagerdutyConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	ServiceKey  Secret            `yaml:"service_key,omitempty" json"service_key,omitempty"`
	RoutingKey  Secret            `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`
	URL         string            `yaml:"url,omitempty" json:"url,omitempty"`
	Client      string            `yaml:"client,omitempty" json:"client,omitempty"`
	ClientURL   string            `yaml:"client_url,omitempty" json:"client_url,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Details     map[string]string `yaml:"details,omitempty" json:"details,omitempty"`
	Severity    string            `yaml:"severity,omitempty" json:"severity,omitempty"`
	Class       string            `yaml:"class,omitempty" json:"class,omitempty"`
	Component   string            `yaml:"component,omitempty" json:"component,omitempty"`
	Group       string            `yaml:"group,omitempty" json:"group,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultPagerdutyConfig
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoutingKey == "" && c.ServiceKey == "" {
		return fmt.Errorf("missing service or routing key in PagerDuty config")
	}
	if c.Details == nil {
		c.Details = make(map[string]string)
	}
	for k, v := range DefaultPagerdutyDetails {
		if _, ok := c.Details[k]; !ok {
			c.Details[k] = v
		}
	}
	return nil
}

// SlackAction configures a single Slack action that is sent with each notification.
// Each action must contain a type, text, and url.
// See https://api.slack.com/docs/message-attachments#action_fields for more information.
type SlackAction struct {
	Type  string `yaml:"type,omitempty"  json:"type,omitempty"`
	Text  string `yaml:"text,omitempty"  json:"text,omitempty"`
	URL   string `yaml:"url,omitempty"   json:"url,omitempty"`
	Style string `yaml:"style,omitempty" json:"style,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SlackAction.
func (c *SlackAction) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain SlackAction
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Type == "" {
		return fmt.Errorf("missing type in Slack action configuration")
	}
	if c.Text == "" {
		return fmt.Errorf("missing value in Slack text configuration")
	}
	if c.URL == "" {
		return fmt.Errorf("missing value in Slack url configuration")
	}
	return nil
}

// SlackField configures a single Slack field that is sent with each notification.
// Each field must contain a title, value, and optionally, a boolean value to indicate if the field
// is short enough to be displayed next to other fields designated as short.
// See https://api.slack.com/docs/message-attachments#fields for more information.
type SlackField struct {
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	Short *bool  `yaml:"short,omitempty" json:"short,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SlackField.
func (c *SlackField) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain SlackField
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Title == "" {
		return fmt.Errorf("missing title in Slack field configuration")
	}
	if c.Value == "" {
		return fmt.Errorf("missing value in Slack field configuration")
	}
	return nil
}

// SlackConfig configures notifications via Slack.
type SlackConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIURL Secret `yaml:"api_url,omitempty" json:"api_url,omitempty"`

	// Slack channel override, (like #other-channel or @username).
	Channel  string `yaml:"channel,omitempty" json:"channel,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Color    string `yaml:"color,omitempty" json:"color,omitempty"`

	Title       string         `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string         `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Pretext     string         `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text        string         `yaml:"text,omitempty" json:"text,omitempty"`
	Fields      []*SlackField  `yaml:"fields,omitempty" json:"fields,omitempty"`
	ShortFields bool           `yaml:"short_fields,omitempty" json:"short_fields,omitempty"`
	Footer      string         `yaml:"footer,omitempty" json:"footer,omitempty"`
	Fallback    string         `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	IconEmoji   string         `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	IconURL     string         `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	LinkNames   bool           `yaml:"link_names,omitempty" json:"link_names,omitempty"`
	Actions     []*SlackAction `yaml:"actions,omitempty" json:"actions,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
	return unmarshal((*plain)(c))
}

// HipchatConfig configures notifications via Hipchat.
type HipchatConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIURL        string `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	AuthToken     Secret `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	RoomID        string `yaml:"room_id,omitempty" json:"room_id,omitempty"`
	From          string `yaml:"from,omitempty" json:"from,omitempty"`
	Notify        bool   `yaml:"notify,omitempty" json:"notify,omitempty"`
	Message       string `yaml:"message,omitempty" json:"message,omitempty"`
	MessageFormat string `yaml:"message_format,omitempty" json:"message_format,omitempty"`
	Color         string `yaml:"color,omitempty" json:"color,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *HipchatConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultHipchatConfig
	type plain HipchatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoomID == "" {
		return fmt.Errorf("missing room id in Hipchat config")
	}
	return nil
}

// WebhookConfig configures notifications via a generic webhook.
type WebhookConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// URL to send POST request to.
	URL string `yaml:"url" json:"url"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebhookConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultWebhookConfig
	type plain WebhookConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == "" {
		return fmt.Errorf("missing URL in webhook config")
	}
	url, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	if url.Scheme != "https" && url.Scheme != "http" {
		return fmt.Errorf("scheme required for webhook url")
	}
	c.URL = url.String()
	return nil
}

// WechatConfig configures notifications via Wechat.
type WechatConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APISecret Secret `yaml:"api_secret,omitempty" json:"api_secret,omitempty"`
	CorpID    string `yaml:"corp_id,omitempty" json:"corp_id,omitempty"`
	Message   string `yaml:"message,omitempty" json:"message,omitempty"`
	APIURL    string `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	ToUser    string `yaml:"to_user,omitempty" json:"to_user,omitempty"`
	ToParty   string `yaml:"to_party,omitempty" json:"to_party,omitempty"`
	ToTag     string `yaml:"to_tag,omitempty" json:"to_tag,omitempty"`
	AgentID   string `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WechatConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultWechatConfig
	type plain WechatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.APISecret == "" {
		return fmt.Errorf("missing Wechat APISecret in Wechat config")
	}
	if c.CorpID == "" {
		return fmt.Errorf("missing Wechat CorpID in Wechat config")
	}
	return nil
}

// OpsGenieConfig configures notifications via OpsGenie.
type OpsGenieConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIKey      Secret            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIURL      string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Message     string            `yaml:"message,omitempty" json:"message,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Source      string            `yaml:"source,omitempty" json:"source,omitempty"`
	Details     map[string]string `yaml:"details,omitempty" json:"details,omitempty"`
	Teams       string            `yaml:"teams,omitempty" json:"teams,omitempty"`
	Tags        string            `yaml:"tags,omitempty" json:"tags,omitempty"`
	Note        string            `yaml:"note,omitempty" json:"note,omitempty"`
	Priority    string            `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpsGenieConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultOpsGenieConfig
	type plain OpsGenieConfig
	return unmarshal((*plain)(c))
}

// VictorOpsConfig configures notifications via VictorOps.
type VictorOpsConfig struct {
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIKey            Secret `yaml:"api_key" json:"api_key"`
	APIURL            string `yaml:"api_url" json:"api_url"`
	RoutingKey        string `yaml:"routing_key" json:"routing_key"`
	MessageType       string `yaml:"message_type" json:"message_type"`
	StateMessage      string `yaml:"state_message" json:"state_message"`
	EntityDisplayName string `yaml:"entity_display_name" json:"entity_display_name"`
	MonitoringTool    string `yaml:"monitoring_tool" json:"monitoring_tool"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *VictorOpsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultVictorOpsConfig
	type plain VictorOpsConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoutingKey == "" {
		return fmt.Errorf("missing Routing key in VictorOps config")
	}
	return nil
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
	NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	UserKey  Secret   `yaml:"user_key,omitempty" json:"user_key,omitempty"`
	Token    Secret   `yaml:"token,omitempty" json:"token,omitempty"`
	Title    string   `yaml:"title,omitempty" json:"title,omitempty"`
	Message  string   `yaml:"message,omitempty" json:"message,omitempty"`
	URL      string   `yaml:"url,omitempty" json:"url,omitempty"`
	Priority string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Retry    duration `yaml:"retry,omitempty" json:"retry,omitempty"`
	Expire   duration `yaml:"expire,omitempty" json:"expire,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PushoverConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultPushoverConfig
	type plain PushoverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.UserKey == "" {
		return fmt.Errorf("missing user key in Pushover config")
	}
	if c.Token == "" {
		return fmt.Errorf("missing token in Pushover config")
	}
	return nil
}
