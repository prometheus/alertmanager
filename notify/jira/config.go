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

package jira

import (
	"errors"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

var DefaultJiraConfig = JiraConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	APIType: "auto",
	Summary: JiraFieldConfig{
		Template: `{{ template "jira.default.summary" . }}`,
	},
	Description: JiraFieldConfig{
		Template: `{{ template "jira.default.description" . }}`,
	},
	Priority: `{{ template "jira.default.priority" . }}`,
}

type JiraFieldConfig struct {
	// Template is the template string used to render the field.
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
	// EnableUpdate indicates whether this field should be omitted when updating an existing issue.
	EnableUpdate *bool `yaml:"enable_update,omitempty" json:"enable_update,omitempty"`
}

type JiraConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`
	HTTPConfig                 *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIURL  *amcommoncfg.URL `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	APIType string           `yaml:"api_type,omitempty" json:"api_type,omitempty"`

	Project     string          `yaml:"project,omitempty" json:"project,omitempty"`
	Summary     JiraFieldConfig `yaml:"summary,omitempty" json:"summary,omitempty"`
	Description JiraFieldConfig `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      []string        `yaml:"labels,omitempty" json:"labels,omitempty"`
	Priority    string          `yaml:"priority,omitempty" json:"priority,omitempty"`
	IssueType   string          `yaml:"issue_type,omitempty" json:"issue_type,omitempty"`

	ReopenTransition  string         `yaml:"reopen_transition,omitempty" json:"reopen_transition,omitempty"`
	ResolveTransition string         `yaml:"resolve_transition,omitempty" json:"resolve_transition,omitempty"`
	WontFixResolution string         `yaml:"wont_fix_resolution,omitempty" json:"wont_fix_resolution,omitempty"`
	ReopenDuration    model.Duration `yaml:"reopen_duration,omitempty" json:"reopen_duration,omitempty"`

	Fields map[string]any `yaml:"fields,omitempty" json:"custom_fields,omitempty"`
}

func (f *JiraFieldConfig) EnableUpdateValue() bool {
	if f.EnableUpdate == nil {
		return true
	}
	return *f.EnableUpdate
}

// Supports both the legacy string and the new object form.
func (f *JiraFieldConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Try simple string first (backward compatibility).
	var s string
	if err := unmarshal(&s); err == nil {
		f.Template = s
		// DisableUpdate stays false by default.
		return nil
	}

	// Fallback to full object form.
	type plain JiraFieldConfig
	return unmarshal((*plain)(f))
}

func (c *JiraConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultJiraConfig
	type plain JiraConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.Project == "" {
		return errors.New("missing project in jira_config")
	}
	if c.IssueType == "" {
		return errors.New("missing issue_type in jira_config")
	}
	if c.APIType != "auto" &&
		c.APIType != "cloud" &&
		c.APIType != "datacenter" {
		return errors.New("unknown api_type on jira_config, must be auto, cloud or datacenter")
	}
	return nil
}
