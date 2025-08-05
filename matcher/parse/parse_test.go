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

	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestMatchers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected labels.Matchers
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
		input:    "{foo=bar}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals with trailing comma",
		input:    "{foo=bar,}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "not equals",
		input:    "{foo!=bar}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar")},
	}, {
		name:     "match regex",
		input:    "{foo=~[a-z]+}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+")},
	}, {
		name:     "doesn't match regex",
		input:    "{foo!~[a-z]+}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+")},
	}, {
		name:     "equals unicode emoji",
		input:    "{foo=🙂}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals unicode sentence",
		input:    "{foo=🙂bar}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂bar")},
	}, {
		name:     "equals without braces",
		input:    "foo=bar",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without braces but with trailing comma",
		input:    "foo=bar,",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "not equals without braces",
		input:    "foo!=bar",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar")},
	}, {
		name:     "match regex without braces",
		input:    "foo=~[a-z]+",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+")},
	}, {
		name:     "doesn't match regex without braces",
		input:    "foo!~[a-z]+",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+")},
	}, {
		name:     "equals in quotes",
		input:    "{\"foo\"=\"bar\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals in quotes and with trailing comma",
		input:    "{\"foo\"=\"bar\",}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "not equals in quotes",
		input:    "{\"foo\"!=\"bar\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar")},
	}, {
		name:     "match regex in quotes",
		input:    "{\"foo\"=~\"[a-z]+\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+")},
	}, {
		name:     "match regex digit in quotes",
		input:    "{\"foo\"=~\"\\\\d+\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchRegexp, "foo", "\\d+")},
	}, {
		name:     "doesn't match regex in quotes",
		input:    "{\"foo\"!~\"[a-z]+\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+")},
	}, {
		name:     "equals unicode emoji in quotes",
		input:    "{\"foo\"=\"🙂\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals unicode emoji as bytes in quotes",
		input:    "{\"foo\"=\"\\xf0\\x9f\\x99\\x82\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals unicode emoji as code points in quotes",
		input:    "{\"foo\"=\"\\U0001f642\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂")},
	}, {
		name:     "equals unicode sentence in quotes",
		input:    "{\"foo\"=\"🙂bar\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "🙂bar")},
	}, {
		name:     "equals with newline in quotes",
		input:    "{\"foo\"=\"bar\\n\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\n")},
	}, {
		name:     "equals with tab in quotes",
		input:    "{\"foo\"=\"bar\\t\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\t")},
	}, {
		name:     "equals with escaped quotes in quotes",
		input:    "{\"foo\"=\"\\\"bar\\\"\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "\"bar\"")},
	}, {
		name:     "equals with escaped backslash in quotes",
		input:    "{\"foo\"=\"bar\\\\\"}",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar\\")},
	}, {
		name:     "equals without braces in quotes",
		input:    "\"foo\"=\"bar\"",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:     "equals without braces in quotes with trailing comma",
		input:    "\"foo\"=\"bar\",",
		expected: labels.Matchers{mustNewMatcher(t, labels.MatchEqual, "foo", "bar")},
	}, {
		name:  "complex",
		input: "{foo=bar,bar!=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex in quotes",
		input: "{foo=\"bar\",bar!=\"baz\"}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without braces",
		input: "foo=bar,bar!=baz",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "complex without braces in quotes",
		input: "foo=\"bar\",bar!=\"baz\"",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchNotEqual, "bar", "baz"),
		},
	}, {
		name:  "comma",
		input: ",",
		error: "0:1: unexpected ,: expected label name",
	}, {
		name:  "comma in braces",
		input: "{,}",
		error: "1:2: unexpected ,: expected label name",
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
		input: "foo=bar}",
		error: "0:8: }: expected opening brace",
	}, {
		name:  "no close brace",
		input: "{foo=bar",
		error: "0:8: end of input: expected close brace",
	}, {
		name:  "invalid input after operator and before quotes",
		input: "{foo=:\"bar\"}",
		error: "6:11: unexpected \"bar\": expected a comma or close brace",
	}, {
		name:  "invalid escape sequence",
		input: "{foo=\"bar\\w\"}",
		error: "5:12: \"bar\\w\": invalid input",
	}, {
		name:  "invalid escape sequence regex digits",
		input: "{\"foo\"=~\"\\d+\"}",
		error: "8:13: \"\\d+\": invalid input",
	}, {
		name:  "no unquoted escape sequences",
		input: "{foo=bar\\n}",
		error: "8:9: \\: invalid input: expected a comma or close brace",
	}, {
		name:  "invalid unicode",
		input: "{\"foo\"=\"\\xf0\\x9f\"}",
		error: "7:17: \"\\xf0\\x9f\": invalid input",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchers, err := Matchers(test.input)
			if test.error != "" {
				require.EqualError(t, err, test.error)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, matchers)
			}
		})
	}
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *labels.Matcher
		error    string
	}{{
		name:     "equals",
		input:    "{foo=bar}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "equals with trailing comma",
		input:    "{foo=bar,}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "not equals",
		input:    "{foo!=bar}",
		expected: mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar"),
	}, {
		name:     "match regex",
		input:    "{foo=~[a-z]+}",
		expected: mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+"),
	}, {
		name:     "doesn't match regex",
		input:    "{foo!~[a-z]+}",
		expected: mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+"),
	}, {
		name:     "equals unicode emoji",
		input:    "{foo=🙂}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂"),
	}, {
		name:     "equals unicode emoji as bytes in quotes",
		input:    "{\"foo\"=\"\\xf0\\x9f\\x99\\x82\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂"),
	}, {
		name:     "equals unicode emoji as code points in quotes",
		input:    "{\"foo\"=\"\\U0001f642\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂"),
	}, {
		name:     "equals unicode sentence",
		input:    "{foo=🙂bar}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂bar"),
	}, {
		name:     "equals without braces",
		input:    "foo=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "equals without braces but with trailing comma",
		input:    "foo=bar,",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "not equals without braces",
		input:    "foo!=bar",
		expected: mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar"),
	}, {
		name:     "match regex without braces",
		input:    "foo=~[a-z]+",
		expected: mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+"),
	}, {
		name:     "doesn't match regex without braces",
		input:    "foo!~[a-z]+",
		expected: mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+"),
	}, {
		name:     "equals in quotes",
		input:    "{\"foo\"=\"bar\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "equals in quotes and with trailing comma",
		input:    "{\"foo\"=\"bar\",}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "not equals in quotes",
		input:    "{\"foo\"!=\"bar\"}",
		expected: mustNewMatcher(t, labels.MatchNotEqual, "foo", "bar"),
	}, {
		name:     "match regex in quotes",
		input:    "{\"foo\"=~\"[a-z]+\"}",
		expected: mustNewMatcher(t, labels.MatchRegexp, "foo", "[a-z]+"),
	}, {
		name:     "doesn't match regex in quotes",
		input:    "{\"foo\"!~\"[a-z]+\"}",
		expected: mustNewMatcher(t, labels.MatchNotRegexp, "foo", "[a-z]+"),
	}, {
		name:     "equals unicode emoji in quotes",
		input:    "{\"foo\"=\"🙂\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂"),
	}, {
		name:     "equals unicode sentence in quotes",
		input:    "{\"foo\"=\"🙂bar\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "🙂bar"),
	}, {
		name:     "equals with newline in quotes",
		input:    "{\"foo\"=\"bar\\n\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar\n"),
	}, {
		name:     "equals with tab in quotes",
		input:    "{\"foo\"=\"bar\\t\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar\t"),
	}, {
		name:     "equals with escaped quotes in quotes",
		input:    "{\"foo\"=\"\\\"bar\\\"\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "\"bar\""),
	}, {
		name:     "equals with escaped backslash in quotes",
		input:    "{\"foo\"=\"bar\\\\\"}",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar\\"),
	}, {
		name:     "equals without braces in quotes",
		input:    "\"foo\"=\"bar\"",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "equals without braces in quotes with trailing comma",
		input:    "\"foo\"=\"bar\",",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:  "no input",
		error: "no matchers",
	}, {
		name:  "open and closing braces",
		input: "{}",
		error: "no matchers",
	}, {
		name:  "two or more returns error",
		input: "foo=bar,bar=baz",
		error: "expected 1 matcher, found 2",
	}, {
		name:  "invalid unicode",
		input: "foo=\"\\xf0\\x9f\"",
		error: "4:14: \"\\xf0\\x9f\": invalid input",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher, err := Matcher(test.input)
			if test.error != "" {
				require.EqualError(t, err, test.error)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, matcher)
			}
		})
	}
}

func mustNewMatcher(t *testing.T, op labels.MatchType, name, value string) *labels.Matcher {
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}
