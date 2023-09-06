package adapter

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestFallbackMatcherParser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *labels.Matcher
		err      string
	}{{
		name:     "is accepted in both",
		input:    "foo=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
	}, {
		name:     "is accepted in new parser but not old",
		input:    "foo🙂=bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
	}, {
		name:     "is accepted in old parser but not new",
		input:    "foo=!bar",
		expected: mustNewMatcher(t, labels.MatchEqual, "foo", "!bar"),
	}, {
		name:  "is accepted in neither",
		input: "foo!bar",
		err:   "bad matcher format: foo!bar",
	}}
	f := FallbackMatcherParser(log.NewNopLogger())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher, err := f(test.input)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.Nil(t, err)
				require.EqualValues(t, test.expected, matcher)
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
		name:  "is accepted in both",
		input: "{foo=bar,bar=baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz"),
		},
	}, {
		name:  "is accepted in new parser but not old",
		input: "{foo🙂=bar,bar=baz🙂}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo🙂", "bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "baz🙂"),
		},
	}, {
		name:  "is accepted in old parser but not new",
		input: "{foo=!bar,bar=$baz}",
		expected: labels.Matchers{
			mustNewMatcher(t, labels.MatchEqual, "foo", "!bar"),
			mustNewMatcher(t, labels.MatchEqual, "bar", "$baz"),
		},
	}, {
		name:  "is accepted in neither",
		input: "{foo!bar}",
		err:   "bad matcher format: foo!bar",
	}}
	f := FallbackMatchersParser(log.NewNopLogger())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchers, err := f(test.input)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.Nil(t, err)
				require.EqualValues(t, test.expected, matchers)
			}
		})
	}
}

func mustNewMatcher(t *testing.T, op labels.MatchType, name, value string) *labels.Matcher {
	m, err := labels.NewMatcher(op, name, value)
	require.NoError(t, err)
	return m
}
