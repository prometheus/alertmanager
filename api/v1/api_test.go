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

package v1

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

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

// fakeAlerts is a struct implementing the provider.Alerts interface for tests.
type fakeAlerts struct {
	fps    map[model.Fingerprint]int
	alerts []*types.Alert
	err    error
}

func newFakeAlerts(alerts []*types.Alert, withErr bool) *fakeAlerts {
	fps := make(map[model.Fingerprint]int)
	for i, a := range alerts {
		fps[a.Fingerprint()] = i
	}
	f := &fakeAlerts{
		alerts: alerts,
		fps:    fps,
	}
	if withErr {
		f.err = errors.New("Error occured")
	}
	return f
}

func (f *fakeAlerts) Subscribe() provider.AlertIterator           { return nil }
func (f *fakeAlerts) Get(model.Fingerprint) (*types.Alert, error) { return nil, nil }
func (f *fakeAlerts) Put(alerts ...*types.Alert) error {
	return f.err
}
func (f *fakeAlerts) GetPending() provider.AlertIterator {
	ch := make(chan *types.Alert)
	done := make(chan struct{})
	go func() {
		defer close(ch)
		for _, a := range f.alerts {
			ch <- a
		}
	}()
	return provider.NewAlertIterator(ch, done, f.err)
}

func newGetAlertStatus(f *fakeAlerts) func(model.Fingerprint) types.AlertStatus {
	return func(fp model.Fingerprint) types.AlertStatus {
		status := types.AlertStatus{SilencedBy: []string{}, InhibitedBy: []string{}}

		i, ok := f.fps[fp]
		if !ok {
			return status
		}
		alert := f.alerts[i]
		switch alert.Labels["state"] {
		case "active":
			status.State = types.AlertStateActive
		case "unprocessed":
			status.State = types.AlertStateUnprocessed
		case "suppressed":
			status.State = types.AlertStateSuppressed
		}
		if alert.Labels["silenced_by"] != "" {
			status.SilencedBy = append(status.SilencedBy, string(alert.Labels["silenced_by"]))
		}
		if alert.Labels["inhibited_by"] != "" {
			status.InhibitedBy = append(status.InhibitedBy, string(alert.Labels["inhibited_by"]))
		}
		return status
	}
}

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

		alertsProvider := newFakeAlerts([]*types.Alert{}, tc.err)
		api := New(alertsProvider, nil, newGetAlertStatus(alertsProvider), nil, nil)

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

func TestListAlerts(t *testing.T) {
	now := time.Now()
	alerts := []*types.Alert{
		&types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "active", "alertname": "alert1"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		&types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "unprocessed", "alertname": "alert2"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		&types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "silenced_by": "abc", "alertname": "alert3"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		&types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "inhibited_by": "abc", "alertname": "alert4"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		&types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert5"},
				StartsAt: now.Add(-2 * time.Minute),
				EndsAt:   now.Add(-time.Minute),
			},
		},
	}

	for i, tc := range []struct {
		err    bool
		params map[string]string

		code   int
		anames []string
	}{
		{
			false,
			map[string]string{},
			200,
			[]string{"alert1", "alert2", "alert3", "alert4"},
		},
		{
			false,
			map[string]string{"active": "true", "unprocessed": "true", "silenced": "true", "inhibited": "true"},
			200,
			[]string{"alert1", "alert2", "alert3", "alert4"},
		},
		{
			false,
			map[string]string{"active": "false", "unprocessed": "true", "silenced": "true", "inhibited": "true"},
			200,
			[]string{"alert2", "alert3", "alert4"},
		},
		{
			false,
			map[string]string{"active": "true", "unprocessed": "false", "silenced": "true", "inhibited": "true"},
			200,
			[]string{"alert1", "alert3", "alert4"},
		},
		{
			false,
			map[string]string{"active": "true", "unprocessed": "true", "silenced": "false", "inhibited": "true"},
			200,
			[]string{"alert1", "alert2", "alert4"},
		},
		{
			false,
			map[string]string{"active": "true", "unprocessed": "true", "silenced": "true", "inhibited": "false"},
			200,
			[]string{"alert1", "alert2", "alert3"},
		},
		{
			false,
			map[string]string{"filter": "{alertname=\"alert3\""},
			200,
			[]string{"alert3"},
		},
		{
			false,
			map[string]string{"filter": "{alertname"},
			400,
			[]string{},
		},
		{
			false,
			map[string]string{"receiver": "other"},
			200,
			[]string{},
		},
		{
			false,
			map[string]string{"active": "invalid"},
			400,
			[]string{},
		},
		{
			true,
			map[string]string{},
			500,
			[]string{},
		},
	} {
		alertsProvider := newFakeAlerts(alerts, tc.err)
		api := New(alertsProvider, nil, newGetAlertStatus(alertsProvider), nil, nil)
		api.route = dispatch.NewRoute(&config.Route{Receiver: "def-receiver"}, nil)

		r, err := http.NewRequest("GET", "/api/v1/alerts", nil)
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}
		q := r.URL.Query()
		for k, v := range tc.params {
			q.Add(k, v)
		}
		r.URL.RawQuery = q.Encode()
		w := httptest.NewRecorder()

		api.listAlerts(w, r)
		body, _ := ioutil.ReadAll(w.Result().Body)

		var res response
		err = json.Unmarshal(body, &res)
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}

		require.Equal(t, tc.code, w.Code, fmt.Sprintf("test case: %d, response: %s", i, string(body)))
		if w.Code != 200 {
			continue
		}

		// Data needs to be serialized/deserialized to be converted to the real type.
		b, err := json.Marshal(res.Data)
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}
		retAlerts := []*Alert{}
		err = json.Unmarshal(b, &retAlerts)
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}

		anames := []string{}
		for _, a := range retAlerts {
			name, ok := a.Labels["alertname"]
			if ok {
				anames = append(anames, string(name))
			}
		}
		require.Equal(t, tc.anames, anames, fmt.Sprintf("test case: %d, alert names are not equal", i))
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

func TestMatchFilterLabels(t *testing.T) {
	testCases := []struct {
		matcher  labels.MatchType
		expected bool
	}{
		{labels.MatchEqual, true},
		{labels.MatchRegexp, true},
		{labels.MatchNotEqual, false},
		{labels.MatchNotRegexp, false},
	}

	for _, tc := range testCases {
		l, err := labels.NewMatcher(tc.matcher, "foo", "")
		require.NoError(t, err)
		sms := map[string]string{
			"baz": "bar",
		}
		ls := []*labels.Matcher{l}

		require.Equal(t, tc.expected, matchFilterLabels(ls, sms))

		l, err = labels.NewMatcher(tc.matcher, "foo", "")
		require.NoError(t, err)
		sms = map[string]string{
			"baz": "bar",
			"foo": "quux",
		}
		ls = []*labels.Matcher{l}
		require.NotEqual(t, tc.expected, matchFilterLabels(ls, sms))
	}
}

func newMatcher(labelSet model.LabelSet) types.Matchers {
	matchers := make([]*types.Matcher, 0, len(labelSet))
	for key, val := range labelSet {
		matchers = append(matchers, types.NewMatcher(key, string(val)))
	}
	return matchers
}
