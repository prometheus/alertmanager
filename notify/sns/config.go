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
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/sigv4"
)

// sessionTagPattern matches AWS STS session tag key/value constraints:
// letters, numbers, whitespace, and _.:/=+-@
// See: https://docs.aws.amazon.com/STS/latest/APIReference/API_Tag.html
var sessionTagPattern = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+@-]+$`)

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
	if err := c.Sigv4.Validate(); err != nil {
		return err
	}
	// AWS STS has a maximum of 50 session tags.
	// See: https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	if len(c.Sigv4.Tags) > 50 {
		return errors.New("sigv4.tags must not contain more than 50 tags (AWS STS limit)")
	}
	// AWS STS session tag keys are case-insensitive: "team" and "Team" collide.
	// Reject case-insensitive duplicates so the configured attribution is preserved.
	// See: https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
	seenKeys := make(map[string]string, len(c.Sigv4.Tags))
	for k := range c.Sigv4.Tags {
		lower := strings.ToLower(k)
		if prev, ok := seenKeys[lower]; ok {
			return fmt.Errorf("sigv4.tags keys %q and %q collide (AWS STS tag keys are case-insensitive)", prev, k)
		}
		seenKeys[lower] = k
	}
	for k, v := range c.Sigv4.Tags {
		// AWS reserves the "aws:" prefix for both tag keys and values.
		if hasAWSPrefix(k) {
			return fmt.Errorf("sigv4.tags key %q must not use the reserved 'aws:' prefix", k)
		}
		if hasAWSPrefix(v) {
			return fmt.Errorf("sigv4.tags value for key %q must not use the reserved 'aws:' prefix", k)
		}
		// AWS tag keys must be non-empty and ≤ 128 characters (rune-based).
		if utf8.RuneCountInString(k) > 128 {
			return fmt.Errorf("sigv4.tags key %q exceeds maximum length of 128", k)
		}
		// AWS tag values must be ≤ 256 characters (rune-based).
		if utf8.RuneCountInString(v) > 256 {
			return fmt.Errorf("sigv4.tags value for key %q exceeds maximum length of 256", k)
		}
		// Validate allowed characters for both keys and values.
		// See: https://docs.aws.amazon.com/STS/latest/APIReference/API_Tag.html
		if !sessionTagPattern.MatchString(k) {
			return fmt.Errorf("sigv4.tags key %q contains invalid characters (allowed: letters, numbers, whitespace, _.:/=+-@)", k)
		}
		if v != "" && !sessionTagPattern.MatchString(v) {
			return fmt.Errorf("sigv4.tags value for key %q contains invalid characters (allowed: letters, numbers, whitespace, _.:/=+-@)", k)
		}
	}
	return nil
}

// hasAWSPrefix reports whether s begins with the reserved "aws:" prefix.
func hasAWSPrefix(s string) bool {
	return len(s) >= 4 && s[:4] == "aws:"
}
