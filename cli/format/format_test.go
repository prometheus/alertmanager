// Copyright 2018 Prometheus Team
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

package format

import (
	"reflect"
	"testing"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestLabelsMatcher(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := map[string]struct {
		value    string
		input    models.Matcher
		expected *labels.Matcher
	}{
		"IsRegex:true IsEqual:nil": {
			value:    "some value",
			input:    models.Matcher{IsEqual: nil, IsRegex: &trueValue},
			expected: &labels.Matcher{Type: labels.MatchRegexp},
		},
		"IsRegex:true IsEqual:true": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &trueValue, IsRegex: &trueValue},
			expected: &labels.Matcher{Type: labels.MatchRegexp},
		},
		"IsRegex:true IsEqual:false": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &falseValue, IsRegex: &trueValue},
			expected: &labels.Matcher{Type: labels.MatchNotRegexp},
		},

		"IsRegex:false IsEqual:nil": {
			value:    "some value",
			input:    models.Matcher{IsEqual: nil, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchEqual},
		},
		"IsRegex:false IsEqual:true": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &trueValue, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchEqual},
		},
		"IsRegex:false IsEqual:false": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &falseValue, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchNotEqual},
		},

		"IsRegex:nil IsEqual:nil": {
			value:    "some value",
			input:    models.Matcher{IsEqual: nil, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchEqual},
		},
		"IsRegex:nil IsEqual:true": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &trueValue, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchEqual},
		},
		"IsRegex:nil IsEqual:false": {
			value:    "some value",
			input:    models.Matcher{IsEqual: &falseValue, IsRegex: &falseValue},
			expected: &labels.Matcher{Type: labels.MatchNotEqual},
		},
	}

	for name, test := range tests {
		test.input.Name = &name
		test.expected.Name = name
		test.input.Value = &test.value
		test.expected.Value = test.value
		t.Run(name, func(t *testing.T) {
			got := labelsMatcher(test.input)
			if !reflect.DeepEqual(got, test.expected) {
				t.Fail()
			}
		})
	}

}
