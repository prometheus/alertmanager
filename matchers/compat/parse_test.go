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
		total             float64
		disagreeTotal     float64
		incompatibleTotal float64
		invalidTotal      float64
	}{{
		name:     "input is accepted",
		input:    "foo=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
		total:    1,
	}, {
		name:         "input is accepted in neither",
		input:        "foo!bar",
		err:          "bad matcher format: foo!bar",
		total:        1,
		invalidTotal: 1,
	}, {
		name:     "input is accepted in matchers/parse but not pkg/labels",
		input:    "foo🙂=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
		total:    1,
	}, {
		name:              "input is accepted in pkg/labels but not matchers/parse",
		input:             "foo=!bar\\n",
		expected:          mustNewMatcher(t, labels.MatchEqual, "foo", "!bar\n"),
		total:             1,
		incompatibleTotal: 1,
	}, {
		// This input causes disagreement because \xf0\x9f\x99\x82 is the byte sequence for 🙂,
		// which is not understood by pkg/labels but is understood by matchers/parse. In such cases,
		// the fallback parser returns the result from pkg/labels.
		name:          "input causes disagreement",
		input:         "foo=\"\\xf0\\x9f\\x99\\x82\"",
		expected:      mustNewMatcher(t, labels.MatchEqual, "foo", "\\xf0\\x9f\\x99\\x82"),
		total:         1,
		disagreeTotal: 1,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := NewMetrics(prometheus.NewRegistry())
			f := FallbackMatcherParser(log.NewNopLogger(), m)
			matcher, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, test.expected, matcher)
			}
			requireMetric(t, test.total, m.Total)
			requireMetric(t, test.disagreeTotal, m.DisagreeTotal)
			requireMetric(t, test.incompatibleTotal, m.IncompatibleTotal)
			requireMetric(t, test.invalidTotal, m.InvalidTotal)
		})
	}
}

func TestFallbackMatchersParser(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expected          labels.Matchers
		err               string
		total             float64
		disagreeTotal     float64
		incompatibleTotal float64
		invalidTotal      float64
	}{{
		name:  "input is accepted",
		input: "{foo=bar,bar=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz"),
		},
		total: 1,
	}, {
		name:         "input is accepted in neither",
		input:        "{foo!bar}",
		err:          "bad matcher format: foo!bar",
		total:        1,
		invalidTotal: 1,
	}, {
		name:  "input is accepted in matchers/parse but not pkg/labels",
		input: "{foo🙂=bar,bar=baz🙂}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz🙂"),
		},
		total: 1,
	}, {
		name:  "is accepted in pkg/labels but not matchers/parse",
		input: "{foo=!bar,bar=$baz\\n}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "!bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "$baz\n"),
		},
		total:             1,
		incompatibleTotal: 1,
	}, {
		// This input causes disagreement because \xf0\x9f\x99\x82 is the byte sequence for 🙂,
		// which is not understood by pkg/labels but is understood by matchers/parse. In such cases,
		// the fallback parser returns the result from pkg/labels.
		name:  "input causes disagreement",
		input: "{foo=\"\\xf0\\x9f\\x99\\x82\"}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "\\xf0\\x9f\\x99\\x82"),
		},
		total:         1,
		disagreeTotal: 1,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := NewMetrics(prometheus.NewRegistry())
			f := FallbackMatchersParser(log.NewNopLogger(), m)
			matchers, err := f(test.input, "test")
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, test.expected, matchers)
			}
			requireMetric(t, test.total, m.Total)
			requireMetric(t, test.disagreeTotal, m.DisagreeTotal)
			requireMetric(t, test.incompatibleTotal, m.IncompatibleTotal)
			requireMetric(t, test.invalidTotal, m.InvalidTotal)
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

func requireMetric(t *testing.T, expected float64, m *prometheus.GaugeVec) {
	if expected == 0 {
		require.Equal(t, 0, testutil.CollectAndCount(m))
	} else {
		require.Equal(t, 1, testutil.CollectAndCount(m))
		require.Equal(t, expected, testutil.ToFloat64(m))
	}
}
