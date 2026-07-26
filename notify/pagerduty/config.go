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

package pagerduty

import (
	"errors"
	"time"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

var (
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
)

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
