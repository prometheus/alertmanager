// Copyright 2023 The Prometheus Authors
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

package matchers

import (
	"testing"

	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected labels.Matchers
		error    string
	}{{
		name:     "no parens",
		input:    "",
		expected: nil,
	}, {
		name:     "open and closing parens",
		input:    "{}",
		expected: nil,
	}, {
		name:     "equals",
		input:    "{foo=\"bar\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals unicode emoji",
		input:    "{foo=\"🙂\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals without quotes",
		input:    "{foo=bar}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without parens",
		input:    "foo=\"bar\"",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without parens or quotes",
		input:    "foo=bar",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals with trailing comma",
		input:    "{foo=\"bar\",}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without parens but trailing comma",
		input:    "foo=\"bar\",",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals with newline",
		input:    "{foo=\"bar\\n\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\n")},
	}, {
		name:     "equals with tab",
		input:    "{foo=\"bar\\t\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\t")},
	}, {
		name:     "equals with escaped quotes",
		input:    "{foo=\"\\\"bar\\\"\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "\"bar\"")},
	}, {
		name:     "equals with escaped backslash",
		input:    "{foo=\"bar\\\\\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\\")},
	}, {
		name:     "not equals",
		input:    "{foo!=\"bar\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar")},
	}, {
		name:     "match regex",
		input:    "{foo=~\"[a-z]+\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+")},
	}, {
		name:     "doesn't match regex",
		input:    "{foo!~\"[a-z]+\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+")},
	}, {
		name:  "complex",
		input: "{foo=\"bar\",bar!=\"baz\"}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without quotes",
		input: "{foo=bar,bar!=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without parens",
		input: "foo=\"bar\",bar!=\"baz\"",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without parens or quotes",
		input: "foo=bar,bar!=baz",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "open paren",
		input: "{",
		error: "0:1: end of input: expected close paren",
	}, {
		name:  "close paren",
		input: "}",
		error: "0:1: }: expected opening paren",
	}, {
		name:  "no open paren",
		input: "foo=\"bar\"}",
		error: "0:10: }: expected opening paren",
	}, {
		name:  "no close paren",
		input: "{foo=\"bar\"",
		error: "0:10: end of input: expected close paren",
	}, {
		name:  "invalid operator",
		input: "{foo=:\"bar\"}",
		error: "5:6: :: invalid input: expected label value",
	}, {
		name:  "another invalid operator",
		input: "{foo%=\"bar\"}",
		error: "4:5: %: invalid input: expected an operator such as '=', '!=', '=~' or '!~'",
	}, {
		name:  "invalid escape sequence",
		input: "{foo=\"bar\\w\"}",
		error: "5:12: \"bar\\w\": invalid input",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchers, err := Parse(test.input)
			if test.error != "" {
				require.EqualError(t, err, test.error)
			} else {
				require.Nil(t, err)
				assert.EqualValues(t, test.expected, matchers)
			}
		})
	}
}

func mustNewMatcher(t *testing.T, op labels.MatchType, name, value string) *labels.Matcher {
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}
