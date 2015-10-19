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
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
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
}

// Config is the top-level configuration for Alertmanager's config files.
type Config struct {
	Global              *GlobalConfig         `yaml:"global,omitempty"`
	Routes              []*Route              `yaml:"routes,omitempty"`
	InhibitRules        []*InhibitRule        `yaml:"inhibit_rules,omitempty"`
	NotificationConfigs []*NotificationConfig `yaml:"notification_configs,omitempty"`
	Templates           []string              `yaml:"templates"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`

	// original is the input from which the config was parsed.
	original string
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

func (c Config) String() string {
	if c.original != "" {
		return c.original
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
		*c.Global = DefaultGlobalConfig
	}

	names := map[string]struct{}{}

	for _, nc := range c.NotificationConfigs {
		if _, ok := names[nc.Name]; ok {
			return fmt.Errorf("notification config name %q is not unique", nc.Name)
		}
		for _, ec := range nc.EmailConfigs {
			if ec.Smarthost == "" {
				if c.Global.Smarthost == "" {
					return fmt.Errorf("no global mail smarthost set")
				}
				ec.Smarthost = c.Global.Smarthost
			}
		}
		for _, sc := range nc.SlackConfigs {
			if sc.URL == "" {
				if c.Global.SlackURL == "" {
					return fmt.Errorf("no global Slack URL set")
				}
				sc.URL = c.Global.SlackURL
			}
		}
		for _, pdc := range nc.PagerdutyConfigs {
			if pdc.URL == "" {
				if c.Global.PagerdutyURL == "" {
					return fmt.Errorf("no global PagerDuty URL set")
				}
				pdc.URL = c.Global.PagerdutyURL
			}
		}
		names[nc.Name] = struct{}{}
	}
	return checkOverflow(c.XXX, "config")
}

var DefaultGlobalConfig = GlobalConfig{
	GroupWait:      model.Duration(30 * time.Second),
	GroupInterval:  model.Duration(5 * time.Minute),
	RepeatInterval: model.Duration(1 * time.Hour),
	SendResolved:   true,

	PagerdutyURL: "https://events.pagerduty.com/generic/2010-04-15/create_event.json",
}

type GlobalConfig struct {
	GroupWait      model.Duration `yaml:"group_wait"`
	GroupInterval  model.Duration `yaml:"group_interval"`
	RepeatInterval model.Duration `yaml:"repeat_interval"`
	SendResolved   bool           `yaml:"send_resolved"`

	Smarthost    string `yaml:"smarthost"`
	SlackURL     string `yaml:"slack_url"`
	PagerdutyURL string `yaml:"pagerduty_url"`
}

func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultGlobalConfig
	type plain GlobalConfig
	return unmarshal((*plain)(c))
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	SendTo  string            `yaml:"send_to,omitempty"`
	GroupBy []model.LabelName `yaml:"group_by,omitempty"`

	Match    map[string]string `yaml:"match,omitempty"`
	MatchRE  map[string]Regexp `yaml:"match_re,omitempty"`
	Continue bool              `yaml:"continue,omitempty"`
	Routes   []*Route          `yaml:"routes,omitempty"`

	GroupWait      *model.Duration `yaml:"group_wait,omitempty"`
	GroupInterval  *model.Duration `yaml:"group_interval,omitempty"`
	RepeatInterval *model.Duration `yaml:"repeat_interval"`
	SendResolved   *bool           `yaml:"send_resolved"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Route) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Route
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	for k := range r.Match {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for k := range r.MatchRE {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	groupBy := map[model.LabelName]struct{}{}

	for _, ln := range r.GroupBy {
		if _, ok := groupBy[ln]; ok {
			return fmt.Errorf("duplicated label %q in group_by", ln)
		}
		groupBy[ln] = struct{}{}
	}

	return checkOverflow(r.XXX, "route")
}

type InhibitRule struct {
	SourceMatch   map[string]string `yaml:"source_match"`
	SourceMatchRE map[string]Regexp `yaml:"source_match_re"`
	TargetMatch   map[string]string `yaml:"target_match"`
	TargetMatchRE map[string]Regexp `yaml:"target_match_re"`
	Equal         model.LabelNames  `yaml:"equal"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *InhibitRule) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain InhibitRule
	if err := unmarshal((*plain)(r)); err != nil {
		return err
	}

	for k := range r.SourceMatch {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for k := range r.SourceMatchRE {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for k := range r.TargetMatch {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	for k := range r.TargetMatchRE {
		if !model.LabelNameRE.MatchString(k) {
			return fmt.Errorf("invalid label name %q", k)
		}
	}

	return checkOverflow(r.XXX, "inhibit rule")
}

// Notification configuration definition.
type NotificationConfig struct {
	// Name of this NotificationConfig. Referenced from AggregationRule.
	Name string `yaml:"name"`

	PagerdutyConfigs []*PagerdutyConfig `yaml:"pagerduty_configs"`
	EmailConfigs     []*EmailConfig     `yaml:"email_configs"`
	PushoverConfigs  []*PushoverConfig  `yaml:"pushover_configs"`
	HipchatConfigs   []*HipchatConfig   `yaml:"hipchat_configs"`
	SlackConfigs     []*SlackConfig     `yaml:"slack_configs"`
	FlowdockConfigs  []*FlowdockConfig  `yaml:"flowdock_configs"`
	WebhookConfigs   []*WebhookConfig   `yaml:"webhook_configs"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NotificationConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain NotificationConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing name in notification config")
	}
	return checkOverflow(c.XXX, "notification config")
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	regexp.Regexp
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile(s)
	if err != nil {
		return err
	}
	re.Regexp = *regex
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re *Regexp) MarshalYAML() (interface{}, error) {
	if re != nil {
		return re.String(), nil
	}
	return nil, nil
}
