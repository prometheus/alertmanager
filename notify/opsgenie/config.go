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

package opsgenie

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// DefaultOpsGenieConfig defines default values for OpsGenie configurations.
var DefaultOpsGenieConfig = OpsGenieConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Message:     `{{ template "opsgenie.default.message" . }}`,
	Description: `{{ template "opsgenie.default.description" . }}`,
	Source:      `{{ template "opsgenie.default.source" . }}`,
	// TODO: Add a details field with all the alerts.
}

var opsgenieTypeMatcher = regexp.MustCompile(opsgenieValidTypesRe)

// OpsGenieConfig configures notifications via OpsGenie.
type OpsGenieConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	APIKey       commoncfg.Secret          `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	APIKeyFile   string                    `yaml:"api_key_file,omitempty" json:"api_key_file,omitempty"`
	APIURL       *amcommoncfg.URL          `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	Message      string                    `yaml:"message,omitempty" json:"message,omitempty"`
	Description  string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Source       string                    `yaml:"source,omitempty" json:"source,omitempty"`
	Details      map[string]string         `yaml:"details,omitempty" json:"details,omitempty"`
	Entity       string                    `yaml:"entity,omitempty" json:"entity,omitempty"`
	Responders   []OpsGenieConfigResponder `yaml:"responders,omitempty" json:"responders,omitempty"`
	Actions      string                    `yaml:"actions,omitempty" json:"actions,omitempty"`
	Tags         string                    `yaml:"tags,omitempty" json:"tags,omitempty"`
	Note         string                    `yaml:"note,omitempty" json:"note,omitempty"`
	Priority     string                    `yaml:"priority,omitempty" json:"priority,omitempty"`
	UpdateAlerts bool                      `yaml:"update_alerts,omitempty" json:"update_alerts,omitempty"`
}

const opsgenieValidTypesRe = `^(team|teams|user|escalation|schedule)$`

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpsGenieConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultOpsGenieConfig
	type plain OpsGenieConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.APIKey != "" && len(c.APIKeyFile) > 0 {
		return errors.New("at most one of api_key & api_key_file must be configured")
	}

	for _, r := range c.Responders {
		if r.ID == "" && r.Username == "" && r.Name == "" {
			return fmt.Errorf("opsGenieConfig responder %v has to have at least one of id, username or name specified", r)
		}

		isTemplated, err := amcommoncfg.ContainsTemplating(r.Type)
		if err != nil {
			return fmt.Errorf("opsGenieConfig responder %v type contains invalid template syntax: %w", r, err)
		}
		if !isTemplated {
			r.Type = strings.ToLower(r.Type)
			if !opsgenieTypeMatcher.MatchString(r.Type) {
				return fmt.Errorf("opsGenieConfig responder %v type does not match valid options %s", r, opsgenieValidTypesRe)
			}
		}
	}

	return nil
}

type OpsGenieConfigResponder struct {
	// One of those 3 should be filled.
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// team, user, escalation, schedule etc.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
}
