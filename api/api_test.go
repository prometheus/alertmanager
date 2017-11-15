package api

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestAlertFiltering(t *testing.T) {
	type test struct {
		alert    *model.Alert
		msg      string
		expected bool
	}

	// Equal
	equal, err := labels.NewMatcher(labels.MatchEqual, "label1", "test1")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests := []test{
		{&model.Alert{Labels: model.LabelSet{"label1": "test1"}}, "label1=test1", true},
		{&model.Alert{Labels: model.LabelSet{"label1": "test2"}}, "label1=test2", false},
		{&model.Alert{Labels: model.LabelSet{"label2": "test2"}}, "label2=test2", false},
	}

	for _, test := range tests {
		actual := alertMatchesFilterLabels(test.alert, []*labels.Matcher{equal})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Not Equal
	notEqual, err := labels.NewMatcher(labels.MatchNotEqual, "label1", "test1")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{&model.Alert{Labels: model.LabelSet{"label1": "test1"}}, "label1!=test1", false},
		{&model.Alert{Labels: model.LabelSet{"label1": "test2"}}, "label1!=test2", true},
		{&model.Alert{Labels: model.LabelSet{"label2": "test2"}}, "label2!=test2", true},
	}

	for _, test := range tests {
		actual := alertMatchesFilterLabels(test.alert, []*labels.Matcher{notEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Regexp Equal
	regexpEqual, err := labels.NewMatcher(labels.MatchRegexp, "label1", "tes.*")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{&model.Alert{Labels: model.LabelSet{"label1": "test1"}}, "label1=~test1", true},
		{&model.Alert{Labels: model.LabelSet{"label1": "test2"}}, "label1=~test2", true},
		{&model.Alert{Labels: model.LabelSet{"label2": "test2"}}, "label2=~test2", false},
	}

	for _, test := range tests {
		actual := alertMatchesFilterLabels(test.alert, []*labels.Matcher{regexpEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Regexp Not Equal
	regexpNotEqual, err := labels.NewMatcher(labels.MatchNotRegexp, "label1", "tes.*")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{&model.Alert{Labels: model.LabelSet{"label1": "test1"}}, "label1!~test1", false},
		{&model.Alert{Labels: model.LabelSet{"label1": "test2"}}, "label1!~test2", false},
		{&model.Alert{Labels: model.LabelSet{"label2": "test2"}}, "label2!~test2", true},
	}

	for _, test := range tests {
		actual := alertMatchesFilterLabels(test.alert, []*labels.Matcher{regexpNotEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}
}

func TestSilenceFiltering(t *testing.T) {
	type test struct {
		silence  *types.Silence
		msg      string
		expected bool
	}

	// Equal
	equal, err := labels.NewMatcher(labels.MatchEqual, "label1", "test1")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests := []test{
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test1"})},
			"label1=test1",
			true,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test2"})},
			"label1=test2",
			false,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label2": "test2"})},
			"label2=test2",
			false,
		},
	}

	for _, test := range tests {
		actual := silenceMatchesFilterLabels(test.silence, []*labels.Matcher{equal})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Not Equal
	notEqual, err := labels.NewMatcher(labels.MatchNotEqual, "label1", "test1")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test1"})},
			"label1!=test1",
			false,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test2"})},
			"label1!=test2",
			true,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label2": "test2"})},
			"label2!=test2",
			true,
		},
	}

	for _, test := range tests {
		actual := silenceMatchesFilterLabels(test.silence, []*labels.Matcher{notEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Regexp Equal
	regexpEqual, err := labels.NewMatcher(labels.MatchRegexp, "label1", "tes.*")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test1"})},
			"label1=~test1",
			true,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test2"})},
			"label1=~test2",
			true,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label2": "test2"})},
			"label2=~test2",
			false,
		},
	}

	for _, test := range tests {
		actual := silenceMatchesFilterLabels(test.silence, []*labels.Matcher{regexpEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}

	// Regexp Not Equal
	regexpNotEqual, err := labels.NewMatcher(labels.MatchNotRegexp, "label1", "tes.*")
	if err != nil {
		t.Error("Unexpected error %v", err)
	}

	tests = []test{
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test1"})},
			"label1!~test1",
			false,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label1": "test2"})},
			"label1!~test2",
			false,
		},
		{
			&types.Silence{Matchers: newMatcher(model.LabelSet{"label2": "test2"})},
			"label2!~test2",
			true,
		},
	}

	for _, test := range tests {
		actual := silenceMatchesFilterLabels(test.silence, []*labels.Matcher{regexpNotEqual})
		msg := fmt.Sprintf("Expected %t for %s", test.expected, test.msg)
		require.Equal(t, test.expected, actual, msg)
	}
}

func TestReceiversMatchFilter(t *testing.T) {
	receivers := []string{"pagerduty", "slack", "hipchat"}

	filter, err := regexp.Compile(fmt.Sprintf("^(?:%s)$", "hip.*"))
	if err != nil {
		t.Error("Unexpected error %v", err)
	}
	require.True(t, receiversMatchFilter(receivers, filter))

	filter, err = regexp.Compile(fmt.Sprintf("^(?:%s)$", "hip"))
	if err != nil {
		t.Error("Unexpected error %v", err)
	}
	require.False(t, receiversMatchFilter(receivers, filter))
}

func newMatcher(labelSet model.LabelSet) types.Matchers {
	matchers := make([]*types.Matcher, 0, len(labelSet))
	for key, val := range labelSet {
		matchers = append(matchers, types.NewMatcher(key, string(val)))
	}
	return matchers
}
