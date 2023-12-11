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

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestFallbackMatcherParser(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expected          *labels.Matcher
		err               string
		total             int
		disagreeTotal     int
		incompatibleTotal int
		invalidTotal      int
	}{{
		name:     "is accepted in both",
		input:    "foo=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
		total:    1,
	}, {
		name:     "is accepted in new parser but not old",
		input:    "foo🙂=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
		total:    1,
	}, {
		name:              "is accepted in old parser but not new",
		input:             "foo=!bar\\n",
		expected:          mustNewMatcher(t, labels.MatchEqual, "foo", "!bar\n"),
		total:             1,
		incompatibleTotal: 1,
	}, {
		name:         "is accepted in neither",
		input:        "foo!bar",
		err:          "bad matcher format: foo!bar",
		total:        1,
		invalidTotal: 1,
	}}

	for _, test := range tests {
		m := NewMetrics(prometheus.NewRegistry())
		f := FallbackMatcherParser(log.NewNopLogger(), m)
		t.Run(test.name, func(t *testing.T) {
			matcher, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, test.expected, matcher)
			}
			require.Equal(t, test.total, testutil.CollectAndCount(m.Total))
			require.Equal(t, test.disagreeTotal, testutil.CollectAndCount(m.DisagreeTotal))
			require.Equal(t, test.incompatibleTotal, testutil.CollectAndCount(m.IncompatibleTotal))
			require.Equal(t, test.invalidTotal, testutil.CollectAndCount(m.InvalidTotal))
		})
	}
}

func TestFallbackMatchersParser(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expected          labels.Matchers
		err               string
		total             int
		disagreeTotal     int
		incompatibleTotal int
		invalidTotal      int
	}{{
		name:  "is accepted in both",
		input: "{foo=bar,bar=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz"),
		},
		total: 1,
	}, {
		name:  "is accepted in new parser but not old",
		input: "{foo🙂=bar,bar=baz🙂}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz🙂"),
		},
		total: 1,
	}, {
		name:  "is accepted in old parser but not new",
		input: "{foo=!bar,bar=$baz\\n}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "!bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "$baz\n"),
		},
		total:             1,
		incompatibleTotal: 1,
	}, {
		name:         "is accepted in neither",
		input:        "{foo!bar}",
		err:          "bad matcher format: foo!bar",
		total:        1,
		invalidTotal: 1,
	}}

	for _, test := range tests {
		m := NewMetrics(prometheus.NewRegistry())
		f := FallbackMatchersParser(log.NewNopLogger(), m)
		t.Run(test.name, func(t *testing.T) {
			matchers, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, test.expected, matchers)
			}
			require.Equal(t, test.total, testutil.CollectAndCount(m.Total))
			require.Equal(t, test.disagreeTotal, testutil.CollectAndCount(m.DisagreeTotal))
			require.Equal(t, test.incompatibleTotal, testutil.CollectAndCount(m.IncompatibleTotal))
			require.Equal(t, test.invalidTotal, testutil.CollectAndCount(m.InvalidTotal))
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
		name:     "is accepted",
		input:    "foo",
		expected: true,
	}, {
		name:     "is also accepted",
		input:    "_foo1",
		expected: true,
	}, {
		name:     "is not accepted",
		input:    "0foo",
		expected: false,
	}, {
		name:     "is also not accepted",
		input:    "foo🙂",
		expected: false,
	}}

	for _, test := range tests {
		fn := isValidClassicLabelName(log.NewNopLogger())
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
		name:     "is accepted",
		input:    "foo",
		expected: true,
	}, {
		name:     "is also accepted",
		input:    "_foo1",
		expected: true,
	}, {
		name:     "is accepted in UTF-8",
		input:    "0foo",
		expected: true,
	}, {
		name:     "is also accepted with UTF-8",
		input:    "foo🙂",
		expected: true,
	}}

	for _, test := range tests {
		fn := isValidUTF8LabelName(log.NewNopLogger())
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, fn(test.input))
		})
	}
}
