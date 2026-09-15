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

package compat

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestFallbackMatcherParser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *labels.Matcher
		err      string
	}{{
		name:     "input is accepted",
		input:    "foo=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:  "input is accepted in neither",
		input: "foo!bar",
		err:   "bad matcher format: foo!bar",
	}, {
		name:     "input is accepted in matchers/parse but not pkg/labels",
		input:    "foo🙂=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
	}, {
		name:     "input is accepted in pkg/labels but not matchers/parse",
		input:    "foo=!bar\\n",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "!bar\n"),
	}, {
		// This input causes disagreement because \xf0\x9f\x99\x82 is the byte sequence for 🙂,
		// which is not understood by pkg/labels but is understood by matchers/parse. In such cases,
		// the fallback parser returns the result from pkg/labels.
		name:     "input causes disagreement",
		input:    "foo=\"\\xf0\\x9f\\x99\\x82\"",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "\\xf0\\x9f\\x99\\x82"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := FallbackMatcherParser(promslog.NewNopLogger())
			matcher, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, matcher)
			}
		})
	}
}

func TestFallbackMatchersParser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected labels.Matchers
		err      string
	}{{
		name:  "input is accepted",
		input: "{foo=bar,bar=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz"),
		},
	}, {
		name:  "input is accepted in neither",
		input: "{foo!bar}",
		err:   "bad matcher format: foo!bar",
	}, {
		name:  "input is accepted in matchers/parse but not pkg/labels",
		input: "{foo🙂=bar,bar=baz🙂}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz🙂"),
		},
	}, {
		name:  "is accepted in pkg/labels but not matchers/parse",
		input: "{foo=!bar,bar=$baz\\n}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "!bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "$baz\n"),
		},
	}, {
		// This input causes disagreement because \xf0\x9f\x99\x82 is the byte sequence for 🙂,
		// which is not understood by pkg/labels but is understood by matchers/parse. In such cases,
		// the fallback parser returns the result from pkg/labels.
		name:  "input causes disagreement",
		input: "{foo=\"\\xf0\\x9f\\x99\\x82\"}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "\\xf0\\x9f\\x99\\x82"),
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := FallbackMatchersParser(promslog.NewNopLogger())
			matchers, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, matchers)
			}
		})
	}
}

func mustNewMatcher(t *testing.T, op labels.MatchType, name, value string) *labels.Matcher {
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}

func TestIsValidClassicLabelName(t *testing.T) {
	tests := []struct {
		name     string
		input    model.LabelName
		expected bool
	}{{
		name:     "foo is accepted",
		input:    "foo",
		expected: true,
	}, {
		name:     "starts with underscore and ends with number is accepted",
		input:    "_foo1",
		expected: true,
	}, {
		name:     "empty is not accepted",
		input:    "",
		expected: false,
	}, {
		name:     "starts with number is not accepted",
		input:    "0foo",
		expected: false,
	}, {
		name:     "contains emoji is not accepted",
		input:    "foo🙂",
		expected: false,
	}}

	for _, test := range tests {
		fn := isValidClassicLabelName(promslog.NewNopLogger())
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, fn(test.input))
		})
	}
}

func TestIsValidUTF8LabelName(t *testing.T) {
	tests := []struct {
		name     string
		input    model.LabelName
		expected bool
	}{{
		name:     "foo is accepted",
		input:    "foo",
		expected: true,
	}, {
		name:     "starts with underscore and ends with number is accepted",
		input:    "_foo1",
		expected: true,
	}, {
		name:     "starts with number is accepted",
		input:    "0foo",
		expected: true,
	}, {
		name:     "contains emoji is accepted",
		input:    "foo🙂",
		expected: true,
	}, {
		name:     "empty is not accepted",
		input:    "",
		expected: false,
	}}

	for _, test := range tests {
		fn := isValidUTF8LabelName(promslog.NewNopLogger())
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, fn(test.input))
		})
	}
}
