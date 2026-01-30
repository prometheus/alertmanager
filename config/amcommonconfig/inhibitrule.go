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

package amcommonconfig

import (
	"fmt"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/matcher/compat"
)

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
