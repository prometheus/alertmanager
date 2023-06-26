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

package parse

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/matchers"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected matchers.Matchers
		error    string
	}{{
		name:     "no braces",
		input:    "",
		expected: nil,
	}, {
		name:     "open and closing braces",
		input:    "{}",
		expected: nil,
	}, {
		name:     "equals",
		input:    "{foo=\"bar\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals unicode emoji",
		input:    "{foo=\"🙂\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals without quotes",
		input:    "{foo=bar}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without braces",
		input:    "foo=\"bar\"",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without braces or quotes",
		input:    "foo=bar",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals with trailing comma",
		input:    "{foo=\"bar\",}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without braces but trailing comma",
		input:    "foo=\"bar\",",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals with newline",
		input:    "{foo=\"bar\\n\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar\n")},
	}, {
		name:     "equals with tab",
		input:    "{foo=\"bar\\t\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar\t")},
	}, {
		name:     "equals with escaped quotes",
		input:    "{foo=\"\\\"bar\\\"\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "\"bar\"")},
	}, {
		name:     "equals with escaped backslash",
		input:    "{foo=\"bar\\\\\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchEqual, "foo", "bar\\")},
	}, {
		name:     "not equals",
		input:    "{foo!=\"bar\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchNotEqual, "foo", "bar")},
	}, {
		name:     "match regex",
		input:    "{foo=~\"[a-z]+\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchRegexp, "foo", "[a-z]+")},
	}, {
		name:     "doesn't match regex",
		input:    "{foo!~\"[a-z]+\"}",
		expected: matchers.Matchers{mustNewMatcher(t, matchers.MatchNotRegexp, "foo", "[a-z]+")},
	}, {
		name:  "complex",
		input: "{foo=\"bar\",bar!=\"baz\"}",
		expected: matchers.Matchers{
			mustNewMatcher(t, matchers.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, matchers.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without quotes",
		input: "{foo=bar,bar!=baz}",
		expected: matchers.Matchers{
			mustNewMatcher(t, matchers.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, matchers.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without braces",
		input: "foo=\"bar\",bar!=\"baz\"",
		expected: matchers.Matchers{
			mustNewMatcher(t, matchers.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, matchers.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without braces or quotes",
		input: "foo=bar,bar!=baz",
		expected: matchers.Matchers{
			mustNewMatcher(t, matchers.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, matchers.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "open brace",
		input: "{",
		error: "0:1: end of input: expected close brace",
	}, {
		name:  "close brace",
		input: "}",
		error: "0:1: }: expected opening brace",
	}, {
		name:  "no open brace",
		input: "foo=\"bar\"}",
		error: "0:10: }: expected opening brace",
	}, {
		name:  "no close brace",
		input: "{foo=\"bar\"",
		error: "0:10: end of input: expected close brace",
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
				require.EqualValues(t, test.expected, matchers)
			}
		})
	}
}

func mustNewMatcher(t *testing.T, op matchers.MatchType, name, value string) *matchers.Matcher {
	m, err := matchers.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}
