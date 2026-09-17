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

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
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
	return c.Validate()
}

func (c *WebexConfig) Validate() error {
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
	// Header names are case insensitive. The normalization loop below
	// detects duplicates and builds a canonical header map in one pass.
	// Both the detection and the normalization stay here rather than in
	// Validate to avoid iterating over the headers a second time.
	normalizedHeaders := map[string]string{}
	for h, v := range c.Headers {
		normalized := textproto.CanonicalMIMEHeaderKey(h)
		if _, ok := normalizedHeaders[normalized]; ok {
			return fmt.Errorf("duplicate header %q in email config", normalized)
		}
		normalizedHeaders[normalized] = v
	}
	c.Headers = normalizedHeaders

	return c.Validate()
}

func (c *EmailConfig) Validate() error {
	if c.To == "" {
		return errors.New("missing to address in email config")
	}

	if c.Threading.Enabled {
		if _, ok := c.Headers["References"]; ok {
			return errors.New("conflicting configuration: threading.enabled conflicts with custom References header")
		}
		if _, ok := c.Headers["In-Reply-To"]; ok {
			return errors.New("conflicting configuration: threading.enabled conflicts with custom In-Reply-To header")
		}
		if !slices.Contains([]string{"none", "daily"}, c.Threading.ThreadByDate) {
			return errors.New("threading.thread_by_date must be either 'none' or 'daily'")
		}
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
	return c.Validate()
}

func (c *VictorOpsConfig) Validate() error {
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
