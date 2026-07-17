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

package sns

import (
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/sigv4"
)

// DefaultSNSConfig defines default values for SNS configurations.
var DefaultSNSConfig = SNSConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Subject: `{{ template "sns.default.subject" . }}`,
	Message: `{{ template "sns.default.message" . }}`,
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
	return c.Validate()
}

// Validate checks the SNSConfig for correctness.
func (c *SNSConfig) Validate() error {
	if (c.TargetARN == "") != (c.TopicARN == "") != (c.PhoneNumber == "") {
		return errors.New("must provide either a Target ARN, Topic ARN, or Phone Number for SNS config")
	}
	return nil
}
