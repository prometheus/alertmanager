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

package jsmops

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultJSMOpsConfig defines default values for JSM Ops configurations.
var DefaultJSMOpsConfig = JSMOpsConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Message:     `{{ template "jsmops.default.message" . }}`,
	Description: `{{ template "jsmops.default.description" . }}`,
	Source:      `{{ template "jsmops.default.source" . }}`,
}

// jsmopsValidTypesRe defines the accepted responder types for JSM Ops.
// Per swagger.v3.json the API enum is: team, user, escalation, schedule.
// The synthetic "teams" value is a config convenience that splits a
// comma-separated name into multiple "team" responders at send time.
const jsmopsValidTypesRe = `^(team|teams|user|escalation|schedule)$`

var jsmopsTypeMatcher = regexp.MustCompile(jsmopsValidTypesRe)

// JSMOpsConfig configures notifications via Jira Service Management Operations.
type JSMOpsConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// CloudID is the required Atlassian cloud ID used as a path variable in the
	// JSM Ops API URL (https://api.atlassian.com/jsm/ops/api/{cloudId}/...).
	CloudID      string                  `yaml:"cloud_id,omitempty" json:"cloud_id,omitempty"`
	APIURL       *amcommoncfg.URL        `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Message      string                  `yaml:"message,omitempty" json:"message,omitempty"`
	Description  string                  `yaml:"description,omitempty" json:"description,omitempty"`
	Source       string                  `yaml:"source,omitempty" json:"source,omitempty"`
	Details      map[string]string       `yaml:"details,omitempty" json:"details,omitempty"`
	Entity       string                  `yaml:"entity,omitempty" json:"entity,omitempty"`
	Responders   []JSMOpsConfigResponder `yaml:"responders,omitempty" json:"responders,omitempty"`
	Actions      string                  `yaml:"actions,omitempty" json:"actions,omitempty"`
	Tags         string                  `yaml:"tags,omitempty" json:"tags,omitempty"`
	Note         string                  `yaml:"note,omitempty" json:"note,omitempty"`
	Priority     string                  `yaml:"priority,omitempty" json:"priority,omitempty"`
	UpdateAlerts bool                    `yaml:"update_alerts,omitempty" json:"update_alerts,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *JSMOpsConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultJSMOpsConfig
	type plain JSMOpsConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.CloudID == "" {
		return errors.New("missing cloud_id in jsmops_config")
	}

	for i, r := range c.Responders {
		if r.ID == "" && r.Username == "" && r.Name == "" {
			return fmt.Errorf("jsmOpsConfig responder %v has to have at least one of id, username or name specified", r)
		}

		isTemplated, err := amcommoncfg.ContainsTemplating(r.Type)
		if err != nil {
			return fmt.Errorf("jsmOpsConfig responder %v type contains invalid template syntax: %w", r, err)
		}
		if !isTemplated {
			c.Responders[i].Type = strings.ToLower(r.Type)
			if !jsmopsTypeMatcher.MatchString(c.Responders[i].Type) {
				return fmt.Errorf("jsmOpsConfig responder %v type does not match valid options %s", r, jsmopsValidTypesRe)
			}
		}
	}

	return nil
}

// JSMOpsConfigResponder describes a single JSM Ops responder.
type JSMOpsConfigResponder struct {
	// One of those 3 should be filled.
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// Type is the responder type (team, user, escalation, schedule, etc.).
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
}
