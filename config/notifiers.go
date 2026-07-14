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

package config

import (
	"errors"
	"fmt"
	"net/textproto"
	"regexp"
	"slices"
	"time"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/sigv4"
)

var (

	// DefaultWebexConfig defines default values for Webex configurations.
	DefaultWebexConfig = WebexConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: true,
		},
		Message: `{{ template "webex.default.message" . }}`,
	}

	// DefaultEmailConfig defines default values for Email configurations.
	DefaultEmailConfig = EmailConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: false,
		},
		HTML: `{{ template "email.default.html" . }}`,
		Text: ``,
	}

	// DefaultEmailSubject defines the default Subject header of an Email.
	DefaultEmailSubject = `{{ template "email.default.subject" . }}`

	// DefaultPagerdutyDetails defines the default values for PagerDuty details.
	DefaultPagerdutyDetails = map[string]any{
		"firing":       `{{ .Alerts.Firing | toJson }}`,
		"resolved":     `{{ .Alerts.Resolved | toJson }}`,
		"num_firing":   `{{ .Alerts.Firing | len }}`,
		"num_resolved": `{{ .Alerts.Resolved | len }}`,
	}

	// DefaultPagerdutyConfig defines default values for PagerDuty configurations.
	DefaultPagerdutyConfig = PagerdutyConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: true,
		},
		Description: `{{ template "pagerduty.default.description" .}}`,
		Client:      `{{ template "pagerduty.default.client" . }}`,
		ClientURL:   `{{ template "pagerduty.default.clientURL" . }}`,
	}

	// DefaultSlackConfig defines default values for Slack configurations.
	DefaultSlackConfig = SlackConfig{
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
	// DefaultRocketchatConfig defines default values for Rocketchat configurations.
	DefaultRocketchatConfig = RocketchatConfig{
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

	// DefaultWechatConfig defines default values for wechat configurations.
	DefaultWechatConfig = WechatConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: false,
		},
		Message: `{{ template "wechat.default.message" . }}`,
		ToUser:  `{{ template "wechat.default.to_user" . }}`,
		ToParty: `{{ template "wechat.default.to_party" . }}`,
		ToTag:   `{{ template "wechat.default.to_tag" . }}`,
		AgentID: `{{ template "wechat.default.agent_id" . }}`,
	}

	// DefaultVictorOpsConfig defines default values for VictorOps configurations.
	DefaultVictorOpsConfig = VictorOpsConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: true,
		},
		MessageType:       `CRITICAL`,
		StateMessage:      `{{ template "victorops.default.state_message" . }}`,
		EntityDisplayName: `{{ template "victorops.default.entity_display_name" . }}`,
		MonitoringTool:    `{{ template "victorops.default.monitoring_tool" . }}`,
	}

	// DefaultPushoverConfig defines default values for Pushover configurations.
	DefaultPushoverConfig = PushoverConfig{
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

	// DefaultSNSConfig defines default values for SNS configurations.
	DefaultSNSConfig = SNSConfig{
		NotifierConfig: amcommoncfg.NotifierConfig{
			VSendResolved: true,
		},
		Subject: `{{ template "sns.default.subject" . }}`,
		Message: `{{ template "sns.default.message" . }}`,
	}
)

// WebexConfig configures notifications via Webex.
type WebexConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig                 *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	APIURL                     *amcommoncfg.URL            `yaml:"api_url,omitempty" json:"api_url,omitempty"`

	Message string `yaml:"message,omitempty" json:"message,omitempty"`
	RoomID  string `yaml:"room_id" json:"room_id"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebexConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultWebexConfig
	type plain WebexConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.RoomID == "" {
		return errors.New("missing room_id on webex_config")
	}

	if c.HTTPConfig == nil || c.HTTPConfig.Authorization == nil {
		return errors.New("missing webex_configs.http_config.authorization")
	}

	return nil
}

// EmailConfig configures notifications via mail.
type EmailConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	// Email address to notify.
	To               string               `yaml:"to,omitempty" json:"to,omitempty"`
	From             string               `yaml:"from,omitempty" json:"from,omitempty"`
	Hello            string               `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost        HostPort             `yaml:"smarthost,omitempty" json:"smarthost,omitempty"`
	AuthUsername     string               `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	AuthPassword     commoncfg.Secret     `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	AuthPasswordFile string               `yaml:"auth_password_file,omitempty" json:"auth_password_file,omitempty"`
	AuthSecret       commoncfg.Secret     `yaml:"auth_secret,omitempty" json:"auth_secret,omitempty"`
	AuthSecretFile   string               `yaml:"auth_secret_file,omitempty" json:"auth_secret_file,omitempty"`
	AuthIdentity     string               `yaml:"auth_identity,omitempty" json:"auth_identity,omitempty"`
	Headers          map[string]string    `yaml:"headers,omitempty" json:"headers,omitempty"`
	HTML             string               `yaml:"html,omitempty" json:"html,omitempty"`
	Text             string               `yaml:"text,omitempty" json:"text,omitempty"`
	RequireTLS       *bool                `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	TLSConfig        *commoncfg.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
	// ForceImplicitTLS controls whether to use implicit TLS (direct TLS connection).
	// true: force use of implicit TLS (direct TLS connection)
	// false: force disable implicit TLS (use explicit TLS/STARTTLS if required)
	// nil (default): auto-detect based on port (465=implicit, other=explicit) for backward compatibility
	ForceImplicitTLS *bool           `yaml:"force_implicit_tls,omitempty" json:"force_implicit_tls,omitempty"`
	Threading        ThreadingConfig `yaml:"threading,omitempty" json:"threading,omitempty"`
}

// ThreadingConfig configures mail threading.
type ThreadingConfig struct {
	Enabled      bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	ThreadByDate string `yaml:"thread_by_date,omitempty" json:"thread_by_date,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultEmailConfig
	type plain EmailConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.To == "" {
		return errors.New("missing to address in email config")
	}
	// Header names are case-insensitive, check for collisions.
	normalizedHeaders := map[string]string{}
	for h, v := range c.Headers {
		normalized := textproto.CanonicalMIMEHeaderKey(h)
		if _, ok := normalizedHeaders[normalized]; ok {
			return fmt.Errorf("duplicate header %q in email config", normalized)
		}
		normalizedHeaders[normalized] = v
	}
	c.Headers = normalizedHeaders

	if c.Threading.Enabled {
		if _, ok := normalizedHeaders["References"]; ok {
			return errors.New("conflicting configuration: threading.enabled conflicts with custom References header")
		}
		if _, ok := normalizedHeaders["In-Reply-To"]; ok {
			return errors.New("conflicting configuration: threading.enabled conflicts with custom In-Reply-To header")
		}
		if !slices.Contains([]string{"none", "daily"}, c.Threading.ThreadByDate) {
			return errors.New("threading.thread_by_date must be either 'none' or 'daily'")
		}
	}

	return nil
}

// PagerdutyConfig configures notifications via PagerDuty.
type PagerdutyConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	ServiceKey     commoncfg.Secret `yaml:"service_key,omitempty" json:"service_key,omitempty"`
	ServiceKeyFile string           `yaml:"service_key_file,omitempty" json:"service_key_file,omitempty"`
	RoutingKey     commoncfg.Secret `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`
	RoutingKeyFile string           `yaml:"routing_key_file,omitempty" json:"routing_key_file,omitempty"`
	URL            *amcommoncfg.URL `yaml:"url,omitempty" json:"url,omitempty"`
	Client         string           `yaml:"client,omitempty" json:"client,omitempty"`
	ClientURL      string           `yaml:"client_url,omitempty" json:"client_url,omitempty"`
	Description    string           `yaml:"description,omitempty" json:"description,omitempty"`
	Details        map[string]any   `yaml:"details,omitempty" json:"details,omitempty"`
	Images         []PagerdutyImage `yaml:"images,omitempty" json:"images,omitempty"`
	Links          []PagerdutyLink  `yaml:"links,omitempty" json:"links,omitempty"`
	Source         string           `yaml:"source,omitempty" json:"source,omitempty"`
	Severity       string           `yaml:"severity,omitempty" json:"severity,omitempty"`
	Class          string           `yaml:"class,omitempty" json:"class,omitempty"`
	Component      string           `yaml:"component,omitempty" json:"component,omitempty"`
	Group          string           `yaml:"group,omitempty" json:"group,omitempty"`
	// Timeout is the maximum time allowed to invoke the pagerduty. Setting this to 0
	// does not impose a timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// PagerdutyLink is a link.
type PagerdutyLink struct {
	Href string `yaml:"href,omitempty" json:"href,omitempty"`
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
}

// PagerdutyImage is an image.
type PagerdutyImage struct {
	Src  string `yaml:"src,omitempty" json:"src,omitempty"`
	Alt  string `yaml:"alt,omitempty" json:"alt,omitempty"`
	Href string `yaml:"href,omitempty" json:"href,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultPagerdutyConfig
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoutingKey == "" && c.ServiceKey == "" && c.RoutingKeyFile == "" && c.ServiceKeyFile == "" {
		return errors.New("missing service or routing key in PagerDuty config")
	}
	if len(c.RoutingKey) > 0 && len(c.RoutingKeyFile) > 0 {
		return errors.New("at most one of routing_key & routing_key_file must be configured")
	}
	if len(c.ServiceKey) > 0 && len(c.ServiceKeyFile) > 0 {
		return errors.New("at most one of service_key & service_key_file must be configured")
	}
	if c.Details == nil {
		c.Details = make(map[string]any)
	}
	if c.Source == "" {
		c.Source = c.Client
	}
	for k, v := range DefaultPagerdutyDetails {
		if _, ok := c.Details[k]; !ok {
			c.Details[k] = v
		}
	}
	return nil
}

// SlackAction configures a single Slack action that is sent with each notification.
// See https://api.slack.com/docs/message-attachments#action_fields and https://api.slack.com/docs/message-buttons
// for more information.
type SlackAction struct {
	Type         string                  `yaml:"type,omitempty"  json:"type,omitempty"`
	Text         string                  `yaml:"text,omitempty"  json:"text,omitempty"`
	URL          string                  `yaml:"url,omitempty"   json:"url,omitempty"`
	Style        string                  `yaml:"style,omitempty" json:"style,omitempty"`
	Name         string                  `yaml:"name,omitempty"  json:"name,omitempty"`
	Value        string                  `yaml:"value,omitempty"  json:"value,omitempty"`
	ConfirmField *SlackConfirmationField `yaml:"confirm,omitempty"  json:"confirm,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SlackAction.
func (c *SlackAction) UnmarshalYAML(unmarshal func(any) error) error {
	type plain SlackAction
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Type == "" {
		return errors.New("missing type in Slack action configuration")
	}
	if c.Text == "" {
		return errors.New("missing text in Slack action configuration")
	}
	if c.URL != "" {
		// Clear all message action fields.
		c.Name = ""
		c.Value = ""
		c.ConfirmField = nil
	} else if c.Name != "" {
		c.URL = ""
	} else {
		return errors.New("missing name or url in Slack action configuration")
	}
	return nil
}

// SlackConfirmationField protect users from destructive actions or particularly distinguished decisions
// by asking them to confirm their button click one more time.
// See https://api.slack.com/docs/interactive-message-field-guide#confirmation_fields for more information.
type SlackConfirmationField struct {
	Text        string `yaml:"text,omitempty"  json:"text,omitempty"`
	Title       string `yaml:"title,omitempty"  json:"title,omitempty"`
	OkText      string `yaml:"ok_text,omitempty"  json:"ok_text,omitempty"`
	DismissText string `yaml:"dismiss_text,omitempty"  json:"dismiss_text,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SlackConfirmationField.
func (c *SlackConfirmationField) UnmarshalYAML(unmarshal func(any) error) error {
	type plain SlackConfirmationField
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Text == "" {
		return errors.New("missing text in Slack confirmation configuration")
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
func (c *SlackField) UnmarshalYAML(unmarshal func(any) error) error {
	type plain SlackField
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Title == "" {
		return errors.New("missing title in Slack field configuration")
	}
	if c.Value == "" {
		return errors.New("missing value in Slack field configuration")
	}
	return nil
}

// SlackConfig configures notifications via Slack.
type SlackConfig struct {
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

	Title       string         `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string         `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Pretext     string         `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text        string         `yaml:"text,omitempty" json:"text,omitempty"`
	MessageText string         `yaml:"message_text,omitempty" json:"message_text,omitempty"`
	Fields      []*SlackField  `yaml:"fields,omitempty" json:"fields,omitempty"`
	ShortFields bool           `yaml:"short_fields" json:"short_fields,omitempty"`
	Footer      string         `yaml:"footer,omitempty" json:"footer,omitempty"`
	Fallback    string         `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	CallbackID  string         `yaml:"callback_id,omitempty" json:"callback_id,omitempty"`
	IconEmoji   string         `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	IconURL     string         `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	ImageURL    string         `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	ThumbURL    string         `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	LinkNames   bool           `yaml:"link_names" json:"link_names,omitempty"`
	MrkdwnIn    []string       `yaml:"mrkdwn_in,omitempty" json:"mrkdwn_in,omitempty"`
	Actions     []*SlackAction `yaml:"actions,omitempty" json:"actions,omitempty"`

	// UpdateMessage enables updating existing Slack messages instead of creating new ones.
	// Requires bot token with chat:write scope. Webhook URLs do not support updates.

	UpdateMessage bool `yaml:"update_message" json:"update_message,omitempty"`
	// Timeout is the maximum time allowed to invoke the slack. Setting this to 0
	// does not impose a timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
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

	if c.UpdateMessage && c.APIURL.String() != "https://slack.com/api/chat.postMessage" {
		return errors.New("update_message can only be used with bot tokens. api_url must be set to https://slack.com/api/chat.postMessage")
	}

	return nil
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

	if !wechatTypeMatcher.MatchString(c.MessageType) {
		return fmt.Errorf("weChat message type %q does not match valid options %s", c.MessageType, wechatValidTypesRe)
	}

	if c.APISecret != "" && len(c.APISecretFile) > 0 {
		return errors.New("at most one of api_secret & api_secret_file must be configured")
	}

	return nil
}

// VictorOpsConfig configures notifications via VictorOps.
type VictorOpsConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIKey            commoncfg.Secret  `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIKeyFile        string            `yaml:"api_key_file,omitempty" json:"api_key_file,omitempty"`
	APIURL            *amcommoncfg.URL  `yaml:"api_url" json:"api_url"`
	RoutingKey        string            `yaml:"routing_key" json:"routing_key"`
	MessageType       string            `yaml:"message_type" json:"message_type"`
	StateMessage      string            `yaml:"state_message" json:"state_message"`
	EntityDisplayName string            `yaml:"entity_display_name" json:"entity_display_name"`
	MonitoringTool    string            `yaml:"monitoring_tool" json:"monitoring_tool"`
	CustomFields      map[string]string `yaml:"custom_fields,omitempty" json:"custom_fields,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *VictorOpsConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultVictorOpsConfig
	type plain VictorOpsConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoutingKey == "" {
		return errors.New("missing Routing key in VictorOps config")
	}
	if c.APIKey != "" && len(c.APIKeyFile) > 0 {
		return errors.New("at most one of api_key & api_key_file must be configured")
	}

	reservedFields := []string{"routing_key", "message_type", "state_message", "entity_display_name", "monitoring_tool", "entity_id", "entity_state"}

	for _, v := range reservedFields {
		if _, ok := c.CustomFields[v]; ok {
			return fmt.Errorf("victorOps config contains custom field %s which cannot be used as it conflicts with the fixed/static fields", v)
		}
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

type SNSConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIUrl      string            `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Sigv4       sigv4.SigV4Config `yaml:"sigv4" json:"sigv4"`
	TopicARN    string            `yaml:"topic_arn,omitempty" json:"topic_arn,omitempty"`
	PhoneNumber string            `yaml:"phone_number,omitempty" json:"phone_number,omitempty"`
	TargetARN   string            `yaml:"target_arn,omitempty" json:"target_arn,omitempty"`
	Subject     string            `yaml:"subject,omitempty" json:"subject,omitempty"`
	Message     string            `yaml:"message,omitempty" json:"message,omitempty"`
	Attributes  map[string]string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
	// UseAWSHTTPClient forces the AWS SDK's BuildableClient instead of
	// alertmanager's tracing-wrapped HTTP client. Auto-enabled when AWS_CA_BUNDLE
	// is set; set explicitly when configuring ca_bundle via shared AWS config.
	UseAWSHTTPClient bool `yaml:"use_aws_http_client,omitempty" json:"use_aws_http_client,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SNSConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSNSConfig
	type plain SNSConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if (c.TargetARN == "") != (c.TopicARN == "") != (c.PhoneNumber == "") {
		return errors.New("must provide either a Target ARN, Topic ARN, or Phone Number for SNS config")
	}
	return nil
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
	if c.Token != nil && len(c.TokenFile) > 0 {
		return errors.New("at most one of token & token_file must be configured")
	}
	if c.TokenID != nil && len(c.TokenIDFile) > 0 {
		return errors.New("at most one of token_id & token_id_file must be configured")
	}
	return nil
}
