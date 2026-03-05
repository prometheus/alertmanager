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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/alertmanager/tracing"
)

// containsTemplating checks if the string contains template syntax.
func containsTemplating(s string) (bool, error) {
	if !strings.Contains(s, "{{") {
		return false, nil
	}
	// If it contains template syntax, validate it's actually a valid templ.
	_, err := template.New("").Parse(s)
	if err != nil {
		return true, err
	}
	return true, nil
}

// SecretTemplateURL is a Secret string that represents a URL which may contain
// Go template syntax. Unlike SecretURL, it allows templated values and only
// validates non-templated URLs at unmarshal time.
type SecretTemplateURL commoncfg.Secret

// MarshalYAML implements the yaml.Marshaler interface for SecretTemplateURL.
func (s SecretTemplateURL) MarshalYAML() (any, error) {
	if s != "" {
		if commoncfg.MarshalSecretValue {
			return string(s), nil
		}
		return amcommoncfg.SecretToken, nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SecretTemplateURL.
func (s *SecretTemplateURL) UnmarshalYAML(unmarshal func(any) error) error {
	type plain commoncfg.Secret
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}

	urlStr := string(*s)

	// Skip validation for empty strings or secret token
	if urlStr == "" || urlStr == amcommoncfg.SecretToken {
		return nil
	}

	// Check if the URL contains template syntax
	isTemplated, err := containsTemplating(urlStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	// Only validate as URL if it's not templated
	if !isTemplated {
		if _, err := amcommoncfg.ParseURL(urlStr); err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
	}

	return nil
}

// MarshalJSON implements the json.Marshaler interface for SecretTemplateURL.
func (s SecretTemplateURL) MarshalJSON() ([]byte, error) {
	return commoncfg.Secret(s).MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface for SecretTemplateURL.
func (s *SecretTemplateURL) UnmarshalJSON(data []byte) error {
	if string(data) == amcommoncfg.SecretToken || string(data) == amcommoncfg.SecretTokenJSON {
		*s = ""
		return nil
	}
	// Just unmarshal as a string since Secret doesn't have UnmarshalJSON
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = SecretTemplateURL(str)
	return nil
}

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	// Check if we have a root route. We cannot check for it in the
	// UnmarshalYAML method because it won't be called if the input is empty
	// (e.g. the config file is empty or only contains whitespace).
	if cfg.Route == nil {
		return nil, errors.New("no route provided in config")
	}

	// Check if continue in root route.
	if cfg.Route.Continue {
		return nil, errors.New("cannot have continue in root route")
	}

	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, err
	}

	resolveFilepaths(filepath.Dir(filename), cfg)
	return cfg, nil
}

// resolveFilepaths joins all relative paths in a configuration
// with a given base directory.
func resolveFilepaths(baseDir string, cfg *Config) {
	join := func(fp string) string {
		if len(fp) > 0 && !filepath.IsAbs(fp) {
			fp = filepath.Join(baseDir, fp)
		}
		return fp
	}

	for i, tf := range cfg.Templates {
		cfg.Templates[i] = join(tf)
	}

	cfg.Global.HTTPConfig.SetDirectory(baseDir)
	for _, receiver := range cfg.Receivers {
		for _, cfg := range receiver.OpsGenieConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.PagerdutyConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.PushoverConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.SlackConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.VictorOpsConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.WebhookConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.WechatConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.SNSConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.TelegramConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.DiscordConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.WebexConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.MSTeamsConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.MSTeamsV2Configs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.JiraConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.RocketchatConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
		for _, cfg := range receiver.MattermostConfigs {
			cfg.HTTPConfig.SetDirectory(baseDir)
		}
	}
}

// MuteTimeInterval represents a named set of time intervals for which a route should be muted.
type MuteTimeInterval struct {
	Name          string                      `yaml:"name" json:"name"`
	TimeIntervals []timeinterval.TimeInterval `yaml:"time_intervals" json:"time_intervals"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for MuteTimeInterval.
func (mt *MuteTimeInterval) UnmarshalYAML(unmarshal func(any) error) error {
	type plain MuteTimeInterval
	if err := unmarshal((*plain)(mt)); err != nil {
		return err
	}
	if mt.Name == "" {
		return errors.New("missing name in mute time interval")
	}
	return nil
}

// TimeInterval represents a named set of time intervals for which a route should be muted.
type TimeInterval struct {
	Name          string                      `yaml:"name" json:"name"`
	TimeIntervals []timeinterval.TimeInterval `yaml:"time_intervals" json:"time_intervals"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for MuteTimeInterval.
func (ti *TimeInterval) UnmarshalYAML(unmarshal func(any) error) error {
	type plain TimeInterval
	if err := unmarshal((*plain)(ti)); err != nil {
		return err
	}
	if ti.Name == "" {
		return errors.New("missing name in time interval")
	}
	return nil
}

// Config is the top-level configuration for Alertmanager's config files.
type Config struct {
	Global       *GlobalConfig `yaml:"global,omitempty" json:"global,omitempty"`
	Route        *Route        `yaml:"route,omitempty" json:"route,omitempty"`
	InhibitRules []InhibitRule `yaml:"inhibit_rules,omitempty" json:"inhibit_rules,omitempty"`
	Receivers    []Receiver    `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Templates    []string      `yaml:"templates" json:"templates"`
	// Deprecated. Remove before v1.0 release.
	MuteTimeIntervals []MuteTimeInterval `yaml:"mute_time_intervals,omitempty" json:"mute_time_intervals,omitempty"`
	TimeIntervals     []TimeInterval     `yaml:"time_intervals,omitempty" json:"time_intervals,omitempty"`

	TracingConfig tracing.TracingConfig `yaml:"tracing,omitempty" json:"tracing,omitempty"`

	// original is the input from which the config was parsed.
	original string
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Config.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	// We want to set c to the defaults and then overwrite it with the input.
	// To make unmarshal fill the plain data struct rather than calling UnmarshalYAML
	// again, we have to hide it using a type indirection.
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// If a global block was open but empty the default global config is overwritten.
	// We have to restore it here.
	if c.Global == nil {
		c.Global = &GlobalConfig{}
		*c.Global = DefaultGlobalConfig()
	}

	if c.Global.SlackAppToken != "" && len(c.Global.SlackAppTokenFile) > 0 {
		return errors.New("at most one of slack_app_token & slack_app_token_file must be configured")
	}

	if c.Global.SlackAPIURL != nil && len(c.Global.SlackAPIURLFile) > 0 {
		return errors.New("at most one of slack_api_url & slack_api_url_file must be configured")
	}

	if (c.Global.SlackAppToken != "" || len(c.Global.SlackAppTokenFile) > 0) && (c.Global.SlackAPIURL != nil || len(c.Global.SlackAPIURLFile) > 0) {
		// Support transition from workaround suggested in https://github.com/prometheus/alertmanager/issues/2513,
		// where users might set `slack_api_url` at the top level and then have `http_config` with individual
		// bearer tokens in the receivers.
		if c.Global.SlackAPIURL.String() != c.Global.SlackAppURL.String() {
			return errors.New("at most one of slack_app_token/slack_app_token_file & slack_api_url/slack_api_url_file must be configured")
		}
	}

	if c.Global.OpsGenieAPIKey != "" && len(c.Global.OpsGenieAPIKeyFile) > 0 {
		return errors.New("at most one of opsgenie_api_key & opsgenie_api_key_file must be configured")
	}

	if c.Global.VictorOpsAPIKey != "" && len(c.Global.VictorOpsAPIKeyFile) > 0 {
		return errors.New("at most one of victorops_api_key & victorops_api_key_file must be configured")
	}

	if c.Global.TelegramBotToken != "" && len(c.Global.TelegramBotTokenFile) > 0 {
		return errors.New("at most one of telegram_bot_token & telegram_bot_token_file must be configured")
	}

	if len(c.Global.SMTPAuthPassword) > 0 && len(c.Global.SMTPAuthPasswordFile) > 0 {
		return errors.New("at most one of smtp_auth_password & smtp_auth_password_file must be configured")
	}

	if c.Global.RocketchatToken != nil && len(c.Global.RocketchatTokenFile) > 0 {
		return errors.New("at most one of rocketchat_token & rocketchat_token_file must be configured")
	}

	if c.Global.RocketchatTokenID != nil && len(c.Global.RocketchatTokenIDFile) > 0 {
		return errors.New("at most one of rocketchat_token_id & rocketchat_token_id_file must be configured")
	}

	if len(c.Global.SMTPAuthSecret) > 0 && len(c.Global.SMTPAuthSecretFile) > 0 {
		return fmt.Errorf("at most one of smtp_auth_secret & smtp_auth_secret_file must be configured")
	}

	if c.Global.WeChatAPISecret != "" && len(c.Global.WeChatAPISecretFile) > 0 {
		return errors.New("at most one of wechat_api_secret & wechat_api_secret_file must be configured")
	}

	if c.Global.MattermostWebhookURL != nil && len(c.Global.MattermostWebhookURLFile) > 0 {
		return errors.New("at most one of mattermost_webhook_url & mattermost_webhook_url_file must be configured")
	}

	names := map[string]struct{}{}

	for _, rcv := range c.Receivers {
		if _, ok := names[rcv.Name]; ok {
			return fmt.Errorf("notification config name %q is not unique", rcv.Name)
		}
		for _, wh := range rcv.WebhookConfigs {
			if wh == nil {
				return errors.New("missing webhook config")
			}
			wh.HTTPConfig = cmp.Or(wh.HTTPConfig, c.Global.HTTPConfig)
		}
		for _, ec := range rcv.EmailConfigs {
			if ec == nil {
				return errors.New("missing email config")
			}
			ec.TLSConfig = cmp.Or(ec.TLSConfig, c.Global.SMTPTLSConfig)
			ec.Smarthost = cmp.Or(ec.Smarthost, c.Global.SMTPSmarthost)
			if ec.Smarthost.String() == "" {
				return errors.New("no global SMTP smarthost set")
			}
			ec.From = cmp.Or(ec.From, c.Global.SMTPFrom)
			if ec.From == "" {
				return errors.New("no global SMTP from set")
			}
			ec.Hello = cmp.Or(ec.Hello, c.Global.SMTPHello)
			ec.AuthUsername = cmp.Or(ec.AuthUsername, c.Global.SMTPAuthUsername)
			if ec.AuthPassword == "" && ec.AuthPasswordFile == "" {
				ec.AuthPassword = c.Global.SMTPAuthPassword
				ec.AuthPasswordFile = c.Global.SMTPAuthPasswordFile
			}
			ec.AuthSecret = cmp.Or(ec.AuthSecret, c.Global.SMTPAuthSecret)
			ec.AuthSecretFile = cmp.Or(ec.AuthSecretFile, c.Global.SMTPAuthSecretFile)
			ec.AuthIdentity = cmp.Or(ec.AuthIdentity, c.Global.SMTPAuthIdentity)
			if ec.RequireTLS == nil {
				ec.RequireTLS = new(bool)
				*ec.RequireTLS = c.Global.SMTPRequireTLS
			}
			if ec.ForceImplicitTLS == nil {
				ec.ForceImplicitTLS = c.Global.SMTPForceImplicitTLS
			}
		}
		for _, sc := range rcv.SlackConfigs {
			if sc == nil {
				sc = &SlackConfig{}
			}
			sc.AppURL = cmp.Or(sc.AppURL, c.Global.SlackAppURL)
			if sc.AppURL == nil {
				return errors.New("no global Slack App URL set")
			}
			// we only want to set the app token from global if there's no local authorization or webhook url
			if sc.AppToken == "" && len(sc.AppTokenFile) == 0 && (sc.HTTPConfig == nil || sc.HTTPConfig.Authorization == nil) && sc.APIURL == nil {
				sc.AppToken = c.Global.SlackAppToken
				sc.AppTokenFile = c.Global.SlackAppTokenFile
			}
			if sc.APIURL == nil && len(sc.APIURLFile) == 0 {
				sc.APIURL = c.Global.SlackAPIURL
				sc.APIURLFile = c.Global.SlackAPIURLFile
			}
			if sc.APIURL == nil && len(sc.APIURLFile) == 0 && sc.AppToken == "" && len(sc.AppTokenFile) == 0 {
				return errors.New("no Slack API URL nor App token set either inline or in a file")
			}
			if sc.HTTPConfig == nil {
				// we don't want to change the global http config when setting the receiver's http config, do we do a copy
				httpconfig := *c.Global.HTTPConfig
				sc.HTTPConfig = &httpconfig
			}
			if sc.AppToken != "" || len(sc.AppTokenFile) != 0 {
				if sc.HTTPConfig.Authorization != nil {
					return errors.New("http authorization can't be set when using Slack App tokens")
				}
				sc.HTTPConfig.Authorization = &commoncfg.Authorization{
					Type:            "Bearer",
					Credentials:     commoncfg.Secret(sc.AppToken),
					CredentialsFile: sc.AppTokenFile,
				}
				sc.APIURL = (*amcommoncfg.SecretURL)(sc.AppURL)
			}
		}
		for _, poc := range rcv.PushoverConfigs {
			if poc == nil {
				return errors.New("missing pushover config")
			}
			poc.HTTPConfig = cmp.Or(poc.HTTPConfig, c.Global.HTTPConfig)
		}
		for _, pdc := range rcv.PagerdutyConfigs {
			if pdc == nil {
				return errors.New("missing pagerduty config")
			}
			pdc.HTTPConfig = cmp.Or(pdc.HTTPConfig, c.Global.HTTPConfig)
			pdc.URL = cmp.Or(pdc.URL, c.Global.PagerdutyURL)
			if pdc.URL == nil {
				return errors.New("no global PagerDuty URL set")
			}
		}
		for _, iio := range rcv.IncidentioConfigs {
			if iio == nil {
				return errors.New("missing incidentio config")
			}
			iio.HTTPConfig = cmp.Or(iio.HTTPConfig, c.Global.HTTPConfig)
		}
		for _, ogc := range rcv.OpsGenieConfigs {
			if ogc == nil {
				ogc = &OpsGenieConfig{}
			}
			ogc.HTTPConfig = cmp.Or(ogc.HTTPConfig, c.Global.HTTPConfig)
			ogc.APIURL = cmp.Or(ogc.APIURL, c.Global.OpsGenieAPIURL)
			if ogc.APIURL == nil {
				return errors.New("no global OpsGenie URL set")
			}
			if !strings.HasSuffix(ogc.APIURL.Path, "/") {
				ogc.APIURL.Path += "/"
			}
			ogc.APIKey = cmp.Or(ogc.APIKey, c.Global.OpsGenieAPIKey)
			ogc.APIKeyFile = cmp.Or(ogc.APIKeyFile, c.Global.OpsGenieAPIKeyFile)
			if ogc.APIKey == "" && len(ogc.APIKeyFile) == 0 {
				return errors.New("no global OpsGenie API Key set either inline or in a file")
			}
		}
		for _, wcc := range rcv.WechatConfigs {
			if wcc == nil {
				wcc = &WechatConfig{}
			}
			wcc.HTTPConfig = cmp.Or(wcc.HTTPConfig, c.Global.HTTPConfig)
			wcc.APIURL = cmp.Or(wcc.APIURL, c.Global.WeChatAPIURL)
			if wcc.APIURL == nil {
				return errors.New("no global Wechat URL set")
			}

			if wcc.APISecret == "" && len(wcc.APISecretFile) == 0 {
				if c.Global.WeChatAPISecret == "" && len(c.Global.WeChatAPISecretFile) == 0 {
					return errors.New("no global Wechat Api Secret set either inline or in a file")
				}
				wcc.APISecret = c.Global.WeChatAPISecret
				wcc.APISecretFile = c.Global.WeChatAPISecretFile
			}

			wcc.CorpID = cmp.Or(wcc.CorpID, c.Global.WeChatAPICorpID)
			if wcc.CorpID == "" {
				return errors.New("no global Wechat CorpID set")
			}

			if !strings.HasSuffix(wcc.APIURL.Path, "/") {
				wcc.APIURL.Path += "/"
			}
		}
		for _, voc := range rcv.VictorOpsConfigs {
			if voc == nil {
				return errors.New("missing victorops config")
			}
			voc.HTTPConfig = cmp.Or(voc.HTTPConfig, c.Global.HTTPConfig)
			voc.APIURL = cmp.Or(voc.APIURL, c.Global.VictorOpsAPIURL)
			if voc.APIURL == nil {
				return errors.New("no global VictorOps URL set")
			}
			if !strings.HasSuffix(voc.APIURL.Path, "/") {
				voc.APIURL.Path += "/"
			}
			voc.APIKey = cmp.Or(voc.APIKey, c.Global.VictorOpsAPIKey)
			voc.APIKeyFile = cmp.Or(voc.APIKeyFile, c.Global.VictorOpsAPIKeyFile)
			if voc.APIKey == "" && len(voc.APIKeyFile) == 0 {
				return errors.New("no global VictorOps API Key set")
			}
		}
		for _, sns := range rcv.SNSConfigs {
			if sns == nil {
				return errors.New("missing sns config")
			}
			sns.HTTPConfig = cmp.Or(sns.HTTPConfig, c.Global.HTTPConfig)
		}

		for _, telegram := range rcv.TelegramConfigs {
			if telegram == nil {
				return errors.New("missing telegram config")
			}
			telegram.HTTPConfig = cmp.Or(telegram.HTTPConfig, c.Global.HTTPConfig)
			telegram.APIUrl = cmp.Or(telegram.APIUrl, c.Global.TelegramAPIUrl)
			if telegram.BotToken == "" && len(telegram.BotTokenFile) == 0 {
				if c.Global.TelegramBotToken == "" && len(c.Global.TelegramBotTokenFile) == 0 {
					return errors.New("missing bot_token or bot_token_file on telegram_config")
				}
				telegram.BotToken = c.Global.TelegramBotToken
				telegram.BotTokenFile = c.Global.TelegramBotTokenFile
			}
		}
		for _, discord := range rcv.DiscordConfigs {
			if discord == nil {
				return errors.New("missing discord config")
			}
			discord.HTTPConfig = cmp.Or(discord.HTTPConfig, c.Global.HTTPConfig)
			if discord.WebhookURL == nil && len(discord.WebhookURLFile) == 0 {
				return errors.New("no discord webhook URL or URLFile provided")
			}
		}
		for _, webex := range rcv.WebexConfigs {
			if webex == nil {
				return errors.New("missing webex config")
			}
			webex.HTTPConfig = cmp.Or(webex.HTTPConfig, c.Global.HTTPConfig)
			webex.APIURL = cmp.Or(webex.APIURL, c.Global.WebexAPIURL)
			if webex.APIURL == nil {
				return errors.New("no global Webex URL set")
			}
		}
		for _, msteams := range rcv.MSTeamsConfigs {
			if msteams == nil {
				return errors.New("missing msteams config")
			}
			msteams.HTTPConfig = cmp.Or(msteams.HTTPConfig, c.Global.HTTPConfig)
			if msteams.WebhookURL == nil && len(msteams.WebhookURLFile) == 0 {
				return errors.New("no msteams webhook URL or URLFile provided")
			}
		}
		for _, msteamsv2 := range rcv.MSTeamsV2Configs {
			if msteamsv2 == nil {
				return errors.New("missing msteamsv2 config")
			}
			msteamsv2.HTTPConfig = cmp.Or(msteamsv2.HTTPConfig, c.Global.HTTPConfig)
			if msteamsv2.WebhookURL == nil && len(msteamsv2.WebhookURLFile) == 0 {
				return errors.New("no msteamsv2 webhook URL or URLFile provided")
			}
		}
		for _, jira := range rcv.JiraConfigs {
			if jira == nil {
				return errors.New("missing jira config")
			}
			jira.HTTPConfig = cmp.Or(jira.HTTPConfig, c.Global.HTTPConfig)
			jira.APIURL = cmp.Or(jira.APIURL, c.Global.JiraAPIURL)
			if jira.APIURL == nil {
				return errors.New("no global Jira Cloud URL set")
			}
		}
		for _, rocketchat := range rcv.RocketchatConfigs {
			if rocketchat == nil {
				rocketchat = &RocketchatConfig{}
			}
			rocketchat.HTTPConfig = cmp.Or(rocketchat.HTTPConfig, c.Global.HTTPConfig)
			rocketchat.APIURL = cmp.Or(rocketchat.APIURL, c.Global.RocketchatAPIURL)

			rocketchat.TokenID = cmp.Or(rocketchat.TokenID, c.Global.RocketchatTokenID)
			rocketchat.TokenIDFile = cmp.Or(rocketchat.TokenIDFile, c.Global.RocketchatTokenIDFile)
			if rocketchat.TokenID == nil && len(rocketchat.TokenIDFile) == 0 {
				return errors.New("no global Rocketchat TokenID set either inline or in a file")
			}

			rocketchat.Token = cmp.Or(rocketchat.Token, c.Global.RocketchatToken)
			rocketchat.TokenFile = cmp.Or(rocketchat.TokenFile, c.Global.RocketchatTokenFile)
			if rocketchat.Token == nil && len(rocketchat.TokenFile) == 0 {
				return errors.New("no global Rocketchat Token set either inline or in a file")
			}
		}
		for _, mattermost := range rcv.MattermostConfigs {
			if mattermost == nil {
				return errors.New("missing mattermost config")
			}
			mattermost.HTTPConfig = cmp.Or(mattermost.HTTPConfig, c.Global.HTTPConfig)
			if mattermost.WebhookURL == nil && len(mattermost.WebhookURLFile) == 0 {
				if c.Global.MattermostWebhookURL == nil && len(c.Global.MattermostWebhookURLFile) == 0 {
					return errors.New("missing webhook_url or webhook_url_file on mattermost_config")
				}
				mattermost.WebhookURL = c.Global.MattermostWebhookURL
				mattermost.WebhookURLFile = c.Global.MattermostWebhookURLFile
			}
		}

		names[rcv.Name] = struct{}{}
	}

	// The root route must not have any matchers as it is the fallback node
	// for all alerts.
	if c.Route == nil {
		return errors.New("no routes provided")
	}
	if len(c.Route.Receiver) == 0 {
		return errors.New("root route must specify a default receiver")
	}
	if len(c.Route.Match) > 0 || len(c.Route.MatchRE) > 0 || len(c.Route.Matchers) > 0 {
		return errors.New("root route must not have any matchers")
	}
	if len(c.Route.MuteTimeIntervals) > 0 {
		return errors.New("root route must not have any mute time intervals")
	}

	if len(c.Route.ActiveTimeIntervals) > 0 {
		return errors.New("root route must not have any active time intervals")
	}

	// Validate that all receivers used in the routing tree are defined.
	if err := checkReceiver(c.Route, names); err != nil {
		return err
	}

	tiNames := make(map[string]struct{})

	// read mute time intervals until deprecated
	for _, mt := range c.MuteTimeIntervals {
		if _, ok := tiNames[mt.Name]; ok {
			return fmt.Errorf("mute time interval %q is not unique", mt.Name)
		}
		tiNames[mt.Name] = struct{}{}
	}

	for _, mt := range c.TimeIntervals {
		if _, ok := tiNames[mt.Name]; ok {
			return fmt.Errorf("time interval %q is not unique", mt.Name)
		}
		tiNames[mt.Name] = struct{}{}
	}

	return checkTimeInterval(c.Route, tiNames)
}

// checkReceiver returns an error if a node in the routing tree
// references a receiver not in the given map.
func checkReceiver(r *Route, receivers map[string]struct{}) error {
	for _, sr := range r.Routes {
		if err := checkReceiver(sr, receivers); err != nil {
			return err
		}
	}
	if r.Receiver == "" {
		return nil
	}
	if _, ok := receivers[r.Receiver]; !ok {
		return fmt.Errorf("undefined receiver %q used in route", r.Receiver)
	}
	return nil
}

func checkTimeInterval(r *Route, timeIntervals map[string]struct{}) error {
	for _, sr := range r.Routes {
		if err := checkTimeInterval(sr, timeIntervals); err != nil {
			return err
		}
	}

	for _, ti := range r.ActiveTimeIntervals {
		if _, ok := timeIntervals[ti]; !ok {
			return fmt.Errorf("undefined time interval %q used in route", ti)
		}
	}

	for _, tm := range r.MuteTimeIntervals {
		if _, ok := timeIntervals[tm]; !ok {
			return fmt.Errorf("undefined time interval %q used in route", tm)
		}
	}
	return nil
}

// DefaultGlobalConfig returns GlobalConfig with default values.
func DefaultGlobalConfig() GlobalConfig {
	defaultHTTPConfig := commoncfg.DefaultHTTPClientConfig
	defaultSMTPTLSConfig := commoncfg.TLSConfig{}

	return GlobalConfig{
		ResolveTimeout:   model.Duration(5 * time.Minute),
		HTTPConfig:       &defaultHTTPConfig,
		SMTPHello:        "localhost",
		SMTPRequireTLS:   true,
		SMTPTLSConfig:    &defaultSMTPTLSConfig,
		PagerdutyURL:     amcommoncfg.MustParseURL("https://events.pagerduty.com/v2/enqueue"),
		OpsGenieAPIURL:   amcommoncfg.MustParseURL("https://api.opsgenie.com/"),
		WeChatAPIURL:     amcommoncfg.MustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
		VictorOpsAPIURL:  amcommoncfg.MustParseURL("https://alert.victorops.com/integrations/generic/20131114/alert/"),
		TelegramAPIUrl:   amcommoncfg.MustParseURL("https://api.telegram.org"),
		WebexAPIURL:      amcommoncfg.MustParseURL("https://webexapis.com/v1/messages"),
		RocketchatAPIURL: amcommoncfg.MustParseURL("https://open.rocket.chat/"),
		SlackAppURL:      amcommoncfg.MustParseURL("https://slack.com/api/chat.postMessage"),
	}
}

// HostPort represents a "host:port" network address.
type HostPort struct {
	Host string
	Port string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for HostPort.
func (hp *HostPort) UnmarshalYAML(unmarshal func(any) error) error {
	var (
		s   string
		err error
	)
	if err = unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	hp.Host, hp.Port, err = net.SplitHostPort(s)
	if err != nil {
		return err
	}
	if hp.Port == "" {
		return fmt.Errorf("address %q: port cannot be empty", s)
	}
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for HostPort.
func (hp *HostPort) UnmarshalJSON(data []byte) error {
	var (
		s   string
		err error
	)
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	hp.Host, hp.Port, err = net.SplitHostPort(s)
	if err != nil {
		return err
	}
	if hp.Port == "" {
		return fmt.Errorf("address %q: port cannot be empty", s)
	}
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for HostPort.
func (hp HostPort) MarshalYAML() (any, error) {
	return hp.String(), nil
}

// MarshalJSON implements the json.Marshaler interface for HostPort.
func (hp HostPort) MarshalJSON() ([]byte, error) {
	return json.Marshal(hp.String())
}

func (hp HostPort) String() string {
	if hp.Host == "" && hp.Port == "" {
		return ""
	}
	return net.JoinHostPort(hp.Host, hp.Port)
}

// GlobalConfig defines configuration parameters that are valid globally
// unless overwritten.
type GlobalConfig struct {
	// ResolveTimeout is the time after which an alert is declared resolved
	// if it has not been updated.
	ResolveTimeout model.Duration `yaml:"resolve_timeout" json:"resolve_timeout"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	JiraAPIURL               *amcommoncfg.URL       `yaml:"jira_api_url,omitempty" json:"jira_api_url,omitempty"`
	SMTPFrom                 string                 `yaml:"smtp_from,omitempty" json:"smtp_from,omitempty"`
	SMTPHello                string                 `yaml:"smtp_hello,omitempty" json:"smtp_hello,omitempty"`
	SMTPSmarthost            HostPort               `yaml:"smtp_smarthost,omitempty" json:"smtp_smarthost,omitempty"`
	SMTPAuthUsername         string                 `yaml:"smtp_auth_username,omitempty" json:"smtp_auth_username,omitempty"`
	SMTPAuthPassword         commoncfg.Secret       `yaml:"smtp_auth_password,omitempty" json:"smtp_auth_password,omitempty"`
	SMTPAuthPasswordFile     string                 `yaml:"smtp_auth_password_file,omitempty" json:"smtp_auth_password_file,omitempty"`
	SMTPAuthSecret           commoncfg.Secret       `yaml:"smtp_auth_secret,omitempty" json:"smtp_auth_secret,omitempty"`
	SMTPAuthSecretFile       string                 `yaml:"smtp_auth_secret_file,omitempty" json:"smtp_auth_secret_file,omitempty"`
	SMTPAuthIdentity         string                 `yaml:"smtp_auth_identity,omitempty" json:"smtp_auth_identity,omitempty"`
	SMTPRequireTLS           bool                   `yaml:"smtp_require_tls" json:"smtp_require_tls,omitempty"`
	SMTPTLSConfig            *commoncfg.TLSConfig   `yaml:"smtp_tls_config,omitempty" json:"smtp_tls_config,omitempty"`
	SMTPForceImplicitTLS     *bool                  `yaml:"smtp_force_implicit_tls,omitempty" json:"smtp_force_implicit_tls,omitempty"`
	SlackAPIURL              *amcommoncfg.SecretURL `yaml:"slack_api_url,omitempty" json:"slack_api_url,omitempty"`
	SlackAPIURLFile          string                 `yaml:"slack_api_url_file,omitempty" json:"slack_api_url_file,omitempty"`
	SlackAppToken            commoncfg.Secret       `yaml:"slack_app_token,omitempty" json:"slack_app_token,omitempty"`
	SlackAppTokenFile        string                 `yaml:"slack_app_token_file,omitempty" json:"slack_app_token_file,omitempty"`
	SlackAppURL              *amcommoncfg.URL       `yaml:"slack_app_url,omitempty" json:"slack_app_url,omitempty"`
	PagerdutyURL             *amcommoncfg.URL       `yaml:"pagerduty_url,omitempty" json:"pagerduty_url,omitempty"`
	OpsGenieAPIURL           *amcommoncfg.URL       `yaml:"opsgenie_api_url,omitempty" json:"opsgenie_api_url,omitempty"`
	OpsGenieAPIKey           commoncfg.Secret       `yaml:"opsgenie_api_key,omitempty" json:"opsgenie_api_key,omitempty"`
	OpsGenieAPIKeyFile       string                 `yaml:"opsgenie_api_key_file,omitempty" json:"opsgenie_api_key_file,omitempty"`
	WeChatAPIURL             *amcommoncfg.URL       `yaml:"wechat_api_url,omitempty" json:"wechat_api_url,omitempty"`
	WeChatAPISecret          commoncfg.Secret       `yaml:"wechat_api_secret,omitempty" json:"wechat_api_secret,omitempty"`
	WeChatAPISecretFile      string                 `yaml:"wechat_api_secret_file,omitempty" json:"wechat_api_secret_file,omitempty"`
	WeChatAPICorpID          string                 `yaml:"wechat_api_corp_id,omitempty" json:"wechat_api_corp_id,omitempty"`
	VictorOpsAPIURL          *amcommoncfg.URL       `yaml:"victorops_api_url,omitempty" json:"victorops_api_url,omitempty"`
	VictorOpsAPIKey          commoncfg.Secret       `yaml:"victorops_api_key,omitempty" json:"victorops_api_key,omitempty"`
	VictorOpsAPIKeyFile      string                 `yaml:"victorops_api_key_file,omitempty" json:"victorops_api_key_file,omitempty"`
	TelegramAPIUrl           *amcommoncfg.URL       `yaml:"telegram_api_url,omitempty" json:"telegram_api_url,omitempty"`
	TelegramBotToken         commoncfg.Secret       `yaml:"telegram_bot_token,omitempty" json:"telegram_bot_token,omitempty"`
	TelegramBotTokenFile     string                 `yaml:"telegram_bot_token_file,omitempty" json:"telegram_bot_token_file,omitempty"`
	WebexAPIURL              *amcommoncfg.URL       `yaml:"webex_api_url,omitempty" json:"webex_api_url,omitempty"`
	RocketchatAPIURL         *amcommoncfg.URL       `yaml:"rocketchat_api_url,omitempty" json:"rocketchat_api_url,omitempty"`
	RocketchatToken          *commoncfg.Secret      `yaml:"rocketchat_token,omitempty" json:"rocketchat_token,omitempty"`
	RocketchatTokenFile      string                 `yaml:"rocketchat_token_file,omitempty" json:"rocketchat_token_file,omitempty"`
	RocketchatTokenID        *commoncfg.Secret      `yaml:"rocketchat_token_id,omitempty" json:"rocketchat_token_id,omitempty"`
	RocketchatTokenIDFile    string                 `yaml:"rocketchat_token_id_file,omitempty" json:"rocketchat_token_id_file,omitempty"`
	MattermostWebhookURL     *amcommoncfg.SecretURL `yaml:"mattermost_webhook_url,omitempty" json:"mattermost_webhook_url,omitempty"`
	MattermostWebhookURLFile string                 `yaml:"mattermost_webhook_url_file,omitempty" json:"mattermost_webhook_url_file,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for GlobalConfig.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultGlobalConfig()
	type plain GlobalConfig
	return unmarshal((*plain)(c))
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	Receiver string `yaml:"receiver,omitempty" json:"receiver,omitempty"`

	GroupByStr []string          `yaml:"group_by,omitempty" json:"group_by,omitempty"`
	GroupBy    []model.LabelName `yaml:"-" json:"-"`
	GroupByAll bool              `yaml:"-" json:"-"`
	// Deprecated. Remove before v1.0 release.
	Match map[string]string `yaml:"match,omitempty" json:"match,omitempty"`
	// Deprecated. Remove before v1.0 release.
	MatchRE             MatchRegexps `yaml:"match_re,omitempty" json:"match_re,omitempty"`
	Matchers            Matchers     `yaml:"matchers,omitempty" json:"matchers,omitempty"`
	MuteTimeIntervals   []string     `yaml:"mute_time_intervals,omitempty" json:"mute_time_intervals,omitempty"`
	ActiveTimeIntervals []string     `yaml:"active_time_intervals,omitempty" json:"active_time_intervals,omitempty"`
	Continue            bool         `yaml:"continue" json:"continue,omitempty"`
	Routes              []*Route     `yaml:"routes,omitempty" json:"routes,omitempty"`

	GroupWait      *model.Duration `yaml:"group_wait,omitempty" json:"group_wait,omitempty"`
	GroupInterval  *model.Duration `yaml:"group_interval,omitempty" json:"group_interval,omitempty"`
	RepeatInterval *model.Duration `yaml:"repeat_interval,omitempty" json:"repeat_interval,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Route.
func (r *Route) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Route
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	for k := range r.Match {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for _, l := range r.GroupByStr {
		if l == "..." {
			r.GroupByAll = true
		} else {
			labelName := model.LabelName(l)
			if !compat.IsValidLabelName(labelName) {
				return fmt.Errorf("invalid label name %q in group_by list", l)
			}
			r.GroupBy = append(r.GroupBy, labelName)
		}
	}

	if r.GroupByStr != nil && len(r.GroupByStr) == 0 {
		r.GroupBy = make([]model.LabelName, 0)
	}

	if len(r.GroupBy) > 0 && r.GroupByAll {
		return errors.New("cannot have wildcard group_by (`...`) and other labels at the same time")
	}

	groupBy := map[model.LabelName]struct{}{}

	for _, ln := range r.GroupBy {
		if _, ok := groupBy[ln]; ok {
			return fmt.Errorf("duplicated label %q in group_by", ln)
		}
		groupBy[ln] = struct{}{}
	}

	if r.GroupInterval != nil && time.Duration(*r.GroupInterval) == time.Duration(0) {
		return errors.New("group_interval cannot be zero")
	}
	if r.RepeatInterval != nil && time.Duration(*r.RepeatInterval) == time.Duration(0) {
		return errors.New("repeat_interval cannot be zero")
	}

	return nil
}

// InhibitRule defines an inhibition rule that mutes alerts that match the
// target labels if an alert matching the source labels exists.
// Both alerts have to have a set of labels being equal.
type InhibitRule struct {
	// Name is an optional name for the inhibition rule.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// SourceMatch defines a set of labels that have to equal the given
	// value for source alerts. Deprecated. Remove before v1.0 release.
	SourceMatch map[string]string `yaml:"source_match,omitempty" json:"source_match,omitempty"`
	// SourceMatchRE defines pairs like SourceMatch but does regular expression
	// matching. Deprecated. Remove before v1.0 release.
	SourceMatchRE MatchRegexps `yaml:"source_match_re,omitempty" json:"source_match_re,omitempty"`
	// SourceMatchers defines a set of label matchers that have to be fulfilled for source alerts.
	SourceMatchers Matchers `yaml:"source_matchers,omitempty" json:"source_matchers,omitempty"`
	// TargetMatch defines a set of labels that have to equal the given
	// value for target alerts. Deprecated. Remove before v1.0 release.
	TargetMatch map[string]string `yaml:"target_match,omitempty" json:"target_match,omitempty"`
	// TargetMatchRE defines pairs like TargetMatch but does regular expression
	// matching. Deprecated. Remove before v1.0 release.
	TargetMatchRE MatchRegexps `yaml:"target_match_re,omitempty" json:"target_match_re,omitempty"`
	// TargetMatchers defines a set of label matchers that have to be fulfilled for target alerts.
	TargetMatchers Matchers `yaml:"target_matchers,omitempty" json:"target_matchers,omitempty"`
	// A set of labels that must be equal between the source and target alert
	// for them to be a match.
	Equal []string `yaml:"equal,omitempty" json:"equal,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for InhibitRule.
func (r *InhibitRule) UnmarshalYAML(unmarshal func(any) error) error {
	type plain InhibitRule
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	for k := range r.SourceMatch {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for k := range r.TargetMatch {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for _, l := range r.Equal {
		labelName := model.LabelName(l)
		if !compat.IsValidLabelName(labelName) {
			return fmt.Errorf("invalid label name %q in equal list", l)
		}
	}

	return nil
}

// Receiver configuration provides configuration on how to contact a receiver.
type Receiver struct {
	// A unique identifier for this receiver.
	Name string `yaml:"name" json:"name"`

	DiscordConfigs    []*DiscordConfig    `yaml:"discord_configs,omitempty" json:"discord_configs,omitempty"`
	EmailConfigs      []*EmailConfig      `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`
	IncidentioConfigs []*IncidentioConfig `yaml:"incidentio_configs,omitempty" json:"incidentio_configs,omitempty"`
	PagerdutyConfigs  []*PagerdutyConfig  `yaml:"pagerduty_configs,omitempty" json:"pagerduty_configs,omitempty"`
	SlackConfigs      []*SlackConfig      `yaml:"slack_configs,omitempty" json:"slack_configs,omitempty"`
	WebhookConfigs    []*WebhookConfig    `yaml:"webhook_configs,omitempty" json:"webhook_configs,omitempty"`
	OpsGenieConfigs   []*OpsGenieConfig   `yaml:"opsgenie_configs,omitempty" json:"opsgenie_configs,omitempty"`
	WechatConfigs     []*WechatConfig     `yaml:"wechat_configs,omitempty" json:"wechat_configs,omitempty"`
	PushoverConfigs   []*PushoverConfig   `yaml:"pushover_configs,omitempty" json:"pushover_configs,omitempty"`
	VictorOpsConfigs  []*VictorOpsConfig  `yaml:"victorops_configs,omitempty" json:"victorops_configs,omitempty"`
	SNSConfigs        []*SNSConfig        `yaml:"sns_configs,omitempty" json:"sns_configs,omitempty"`
	TelegramConfigs   []*TelegramConfig   `yaml:"telegram_configs,omitempty" json:"telegram_configs,omitempty"`
	WebexConfigs      []*WebexConfig      `yaml:"webex_configs,omitempty" json:"webex_configs,omitempty"`
	MSTeamsConfigs    []*MSTeamsConfig    `yaml:"msteams_configs,omitempty" json:"msteams_configs,omitempty"`
	MSTeamsV2Configs  []*MSTeamsV2Config  `yaml:"msteamsv2_configs,omitempty" json:"msteamsv2_configs,omitempty"`
	JiraConfigs       []*JiraConfig       `yaml:"jira_configs,omitempty" json:"jira_configs,omitempty"`
	RocketchatConfigs []*RocketchatConfig `yaml:"rocketchat_configs,omitempty" json:"rocketchat_configs,omitempty"`
	MattermostConfigs []*MattermostConfig `yaml:"mattermost_configs,omitempty" json:"mattermost_configs,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Receiver.
func (c *Receiver) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Receiver
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return errors.New("missing name in receiver")
	}
	return nil
}

// MatchRegexps represents a map of Regexp.
type MatchRegexps map[string]Regexp

// UnmarshalYAML implements the yaml.Unmarshaler interface for MatchRegexps.
func (m *MatchRegexps) UnmarshalYAML(unmarshal func(any) error) error {
	type plain MatchRegexps
	if err := unmarshal((*plain)(m)); err != nil {
		return err
	}
	for k, v := range *m {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
		if v.Regexp == nil {
			return fmt.Errorf("invalid regexp value for %q", k)
		}
	}
	return nil
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
	original string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Regexp.
func (re *Regexp) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	re.original = s
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for Regexp.
func (re Regexp) MarshalYAML() (any, error) {
	if re.original != "" {
		return re.original, nil
	}
	return nil, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for Regexp.
func (re *Regexp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	re.original = s
	return nil
}

// MarshalJSON implements the json.Marshaler interface for Regexp.
func (re Regexp) MarshalJSON() ([]byte, error) {
	if re.original != "" {
		return json.Marshal(re.original)
	}
	return []byte("null"), nil
}

// Matchers is label.Matchers with an added UnmarshalYAML method to implement the yaml.Unmarshaler interface
// and MarshalYAML to implement the yaml.Marshaler interface.
type Matchers labels.Matchers

// UnmarshalYAML implements the yaml.Unmarshaler interface for Matchers.
func (m *Matchers) UnmarshalYAML(unmarshal func(any) error) error {
	var lines []string
	if err := unmarshal(&lines); err != nil {
		return err
	}
	for _, line := range lines {
		pm, err := compat.Matchers(line, "config")
		if err != nil {
			return err
		}
		*m = append(*m, pm...)
	}
	sort.Sort(labels.Matchers(*m))
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for Matchers.
func (m Matchers) MarshalYAML() (any, error) {
	result := make([]string, len(m))
	for i, matcher := range m {
		result[i] = matcher.String()
	}
	return result, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for Matchers.
func (m *Matchers) UnmarshalJSON(data []byte) error {
	var lines []string
	if err := json.Unmarshal(data, &lines); err != nil {
		return err
	}
	for _, line := range lines {
		pm, err := compat.Matchers(line, "config")
		if err != nil {
			return err
		}
		*m = append(*m, pm...)
	}
	sort.Sort(labels.Matchers(*m))
	return nil
}

// MarshalJSON implements the json.Marshaler interface for Matchers.
func (m Matchers) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("[]"), nil
	}
	result := make([]string, len(m))
	for i, matcher := range m {
		result[i] = matcher.String()
	}
	return json.Marshal(result)
}
