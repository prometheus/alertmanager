package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

type fakeAlerts struct {
	putWithErr bool
}

func newEmptyIterator() provider.AlertIterator {
	return provider.NewAlertIterator(make(chan *types.Alert), make(chan struct{}), nil)
}

func (f *fakeAlerts) Subscribe() provider.AlertIterator           { return newEmptyIterator() }
func (f *fakeAlerts) GetPending() provider.AlertIterator          { return newEmptyIterator() }
func (f *fakeAlerts) Get(model.Fingerprint) (*types.Alert, error) { return nil, nil }
func (f *fakeAlerts) Put(alerts ...*types.Alert) error {
	if f.putWithErr {
		return errors.New("Error occured")
	}
	return nil
}

func groupAlerts([]*labels.Matcher) dispatch.AlertOverview { return dispatch.AlertOverview{} }
func getAlertStatus(model.Fingerprint) types.AlertStatus   { return types.AlertStatus{} }

func TestAddAlerts(t *testing.T) {
	now := func(offset int) time.Time {
		return time.Now().Add(time.Duration(offset) * time.Second)
	}

	for i, tc := range []struct {
		start, end time.Time
		err        bool
		code       int
	}{
		{time.Time{}, time.Time{}, false, 200},
		{now(0), time.Time{}, false, 200},
		{time.Time{}, now(-1), false, 200},
		{time.Time{}, now(0), false, 200},
		{time.Time{}, now(1), false, 200},
		{now(-2), now(-1), false, 200},
		{now(1), now(2), false, 200},
		{now(1), now(0), false, 400},
		{now(0), time.Time{}, true, 500},
	} {
		alerts := []model.Alert{{
			StartsAt:    tc.start,
			EndsAt:      tc.end,
			Labels:      model.LabelSet{"label1": "test1"},
			Annotations: model.LabelSet{"annotation1": "some text"},
		}}
		b, err := json.Marshal(&alerts)
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}

		api := New(&fakeAlerts{putWithErr: tc.err}, nil, groupAlerts, getAlertStatus, nil, nil)

		r, err := http.NewRequest("POST", "/api/v1/alerts", bytes.NewReader(b))
		w := httptest.NewRecorder()
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}

		api.addAlerts(w, r)
		res := w.Result()
		body, _ := ioutil.ReadAll(res.Body)

		require.Equal(t, tc.code, w.Code, fmt.Sprintf("test case: %d, StartsAt %v, EndsAt %v, Response: %s", i, tc.start, tc.end, string(body)))
	}
}

func TestAlertFiltering(t *testing.T) {
	type test struct {
		alert    *model.Alert
		msg      string
		expected bool
	}

	// Equal
	equal, err := labels.NewMatcher(labels.MatchEqual, "label1", "test1")
	if err != nil {
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
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
		t.Errorf("Unexpected error %v", err)
	}
	require.True(t, receiversMatchFilter(receivers, filter))

	filter, err = regexp.Compile(fmt.Sprintf("^(?:%s)$", "hip"))
	if err != nil {
		t.Errorf("Unexpected error %v", err)
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
