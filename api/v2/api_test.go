// Copyright 2019 Prometheus Team
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

package v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	general_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/general"
	silence_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/silence"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"

	"github.com/go-kit/log"
)

// If api.peers == nil, Alertmanager cluster feature is disabled. Make sure to
// not try to access properties of peer, which would trigger a nil pointer
// dereference.
func TestGetStatusHandlerWithNilPeer(t *testing.T) {
	api := API{
		uptime:             time.Now(),
		peer:               nil,
		alertmanagerConfig: &config.Config{},
	}

	// Test ensures this method call does not panic.
	status := api.getStatusHandler(general_ops.GetStatusParams{}).(*general_ops.GetStatusOK)

	c := status.Payload.Cluster

	if c == nil || c.Status == nil {
		t.Fatal("expected cluster status not to be nil, violating the openapi specification")
	}

	if c.Peers == nil {
		t.Fatal("expected cluster peers to be not nil when api.peer is nil, violating the openapi specification")
	}
	if len(c.Peers) != 0 {
		t.Fatal("expected cluster peers to be empty when api.peer is nil, violating the openapi specification")
	}

	if c.Name != "" {
		t.Fatal("expected cluster name to be empty, violating the openapi specification")
	}
}

func assertEqualStrings(t *testing.T, expected string, actual string) {
	if expected != actual {
		t.Fatal("expected: ", expected, ", actual: ", actual)
	}
}

var (
	testComment = "comment"
	createdBy   = "test"
)

func newSilences(t *testing.T) *silence.Silences {
	silences, err := silence.New(silence.Options{})
	require.NoError(t, err)

	return silences
}

func gettableSilence(id string, state string,
	updatedAt string, start string, end string,
) *open_api_models.GettableSilence {

	updAt, err := strfmt.ParseDateTime(updatedAt)
	if err != nil {
		panic(err)
	}
	strAt, err := strfmt.ParseDateTime(start)
	if err != nil {
		panic(err)
	}
	endAt, err := strfmt.ParseDateTime(end)
	if err != nil {
		panic(err)
	}
	return &open_api_models.GettableSilence{
		Silence: open_api_models.Silence{
			StartsAt:  &strAt,
			EndsAt:    &endAt,
			Comment:   &testComment,
			CreatedBy: &createdBy,
		},
		ID:        &id,
		UpdatedAt: &updAt,
		Status: &open_api_models.SilenceStatus{
			State: &state,
		},
	}
}

func TestGetSilencesHandler(t *testing.T) {

	updateTime := "2019-01-01T12:00:00+00:00"
	silences := []*open_api_models.GettableSilence{
		gettableSilence("silence-6-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T11:00:00+00:00"),
		gettableSilence("silence-1-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T13:00:00+00:00"),
		gettableSilence("silence-7-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T10:00:00+00:00"),
		gettableSilence("silence-5-expired", "expired", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-0-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-4-pending", "pending", updateTime,
			"2019-01-01T13:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-3-pending", "pending", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T12:00:00+00:00"),
		gettableSilence("silence-2-active", "active", updateTime,
			"2019-01-01T12:00:00+00:00", "2019-01-01T14:00:00+00:00"),
	}
	SortSilences(open_api_models.GettableSilences(silences))

	for i, sil := range silences {
		assertEqualStrings(t, "silence-"+strconv.Itoa(i)+"-"+*sil.Status.State, *sil.ID)
	}
}

func TestDeleteSilenceHandler(t *testing.T) {
	now := time.Now()
	silences := newSilences(t)

	m := &silencepb.Matcher{Type: silencepb.Matcher_EQUAL, Name: "a", Pattern: "b"}

	unexpiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now,
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	unexpiredSid, err := silences.Set(unexpiredSil)
	require.NoError(t, err)

	expiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	expiredSid, err := silences.Set(expiredSil)
	require.NoError(t, err)
	require.NoError(t, silences.Expire(expiredSid))

	for i, tc := range []struct {
		sid          string
		expectedCode int
	}{
		{
			"unknownSid",
			500,
		},
		{
			unexpiredSid,
			200,
		},
		{
			expiredSid,
			200,
		},
	} {
		api := API{
			uptime:   time.Now(),
			silences: silences,
			logger:   log.NewNopLogger(),
		}

		r, err := http.NewRequest("DELETE", "/api/v2/silence/${tc.sid}", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		p := runtime.TextProducer()
		responder := api.deleteSilenceHandler(silence_ops.DeleteSilenceParams{
			SilenceID:   strfmt.UUID(tc.sid),
			HTTPRequest: r,
		})
		responder.WriteResponse(w, p)
		body, _ := ioutil.ReadAll(w.Result().Body)

		require.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("test case: %d, response: %s", i, string(body)))
	}
}

func TestPostSilencesHandler(t *testing.T) {
	now := time.Now()
	silences := newSilences(t)

	m := &silencepb.Matcher{Type: silencepb.Matcher_EQUAL, Name: "a", Pattern: "b"}

	unexpiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now,
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	unexpiredSid, err := silences.Set(unexpiredSil)
	require.NoError(t, err)

	expiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	expiredSid, err := silences.Set(expiredSil)
	require.NoError(t, err)
	require.NoError(t, silences.Expire(expiredSid))

	for i, tc := range []struct {
		sid          string
		start, end   time.Time
		expectedCode int
	}{
		{
			"unknownSid",
			now.Add(time.Hour),
			now.Add(time.Hour * 2),
			404,
		},
		{
			"",
			now.Add(time.Hour),
			now.Add(time.Hour * 2),
			200,
		},
		{
			unexpiredSid,
			now.Add(time.Hour),
			now.Add(time.Hour * 2),
			200,
		},
		{
			expiredSid,
			now.Add(time.Hour),
			now.Add(time.Hour * 2),
			200,
		},
	} {
		createdBy := "silenceCreator"
		comment := "test"
		matcherName := "a"
		matcherValue := "b"
		isRegex := false
		startsAt := strfmt.DateTime(tc.start)
		endsAt := strfmt.DateTime(tc.end)

		sil := open_api_models.PostableSilence{
			ID: tc.sid,
			Silence: open_api_models.Silence{
				Matchers:  open_api_models.Matchers{&open_api_models.Matcher{Name: &matcherName, Value: &matcherValue, IsRegex: &isRegex}},
				StartsAt:  &startsAt,
				EndsAt:    &endsAt,
				CreatedBy: &createdBy,
				Comment:   &comment,
			},
		}
		b, err := json.Marshal(&sil)
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}

		api := API{
			uptime:   time.Now(),
			silences: silences,
			logger:   log.NewNopLogger(),
		}

		r, err := http.NewRequest("POST", "/api/v2/silence/${tc.sid}", bytes.NewReader(b))
		require.NoError(t, err)

		w := httptest.NewRecorder()
		p := runtime.TextProducer()
		responder := api.postSilencesHandler(silence_ops.PostSilencesParams{
			HTTPRequest: r,
			Silence:     &sil,
		})
		responder.WriteResponse(w, p)
		body, _ := ioutil.ReadAll(w.Result().Body)

		require.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("test case: %d, response: %s", i, string(body)))
	}
}

func createSilenceMatcher(name string, pattern string, matcherType silencepb.Matcher_Type) *silencepb.Matcher {
	return &silencepb.Matcher{
		Name:    name,
		Pattern: pattern,
		Type:    matcherType,
	}
}

func createLabelMatcher(name string, value string, matchType labels.MatchType) *labels.Matcher {
	matcher, _ := labels.NewMatcher(matchType, name, value)
	return matcher
}

func TestCheckSilenceMatchesFilterLabels(t *testing.T) {
	type test struct {
		silenceMatchers []*silencepb.Matcher
		filterMatchers  []*labels.Matcher
		expected        bool
	}

	tests := []test{
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchEqual)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "novalue", labels.MatchEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "(foo|bar)", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "(foo|bar)", labels.MatchRegexp)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "foo", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "(foo|bar)", labels.MatchRegexp)},
			false,
		},

		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_NOT_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotEqual)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_NOT_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotRegexp)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_NOT_EQUAL)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher("label", "value", silencepb.Matcher_NOT_REGEXP)},
			[]*labels.Matcher{createLabelMatcher("label", "value", labels.MatchNotEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{
				createSilenceMatcher("label", "(foo|bar)", silencepb.Matcher_REGEXP),
				createSilenceMatcher("label", "value", silencepb.Matcher_EQUAL),
			},
			[]*labels.Matcher{createLabelMatcher("label", "(foo|bar)", labels.MatchRegexp)},
			true,
		},
	}

	for _, test := range tests {
		silence := silencepb.Silence{
			Matchers: test.silenceMatchers,
		}
		actual := CheckSilenceMatchesFilterLabels(&silence, test.filterMatchers)
		if test.expected != actual {
			t.Fatal("unexpected match result between silence and filter. expected:", test.expected, ", actual:", actual)
		}
	}
}

func convertDateTime(ts time.Time) *strfmt.DateTime {
	dt := strfmt.DateTime(ts)
	return &dt
}

func TestAlertToOpenAPIAlert(t *testing.T) {
	var (
		start     = time.Now().Add(-time.Minute)
		updated   = time.Now()
		active    = "active"
		fp        = "0223b772b51c29e1"
		receivers = []types.Receiver{{Name: "receiver1", Status: "active"}, {Name: "receiver2", Status: "active"}}
		alert     = &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"severity": "critical", "alertname": "alert1"},
				StartsAt: start,
			},
			UpdatedAt: updated,
		}
	)
	openAPIAlert := AlertToOpenAPIAlert(alert, types.AlertStatus{State: types.AlertStateActive}, receivers)
	var apiReceivers []*open_api_models.Receiver

	for _, r := range receivers {
		name := r.Name
		status := string(r.Status)
		receiver := open_api_models.Receiver{Name: &name, Status: &status}
		apiReceivers = append(apiReceivers, &receiver)
	}

	require.Equal(t, &open_api_models.GettableAlert{
		Annotations: open_api_models.LabelSet{},
		Alert: open_api_models.Alert{
			Labels: open_api_models.LabelSet{"severity": "critical", "alertname": "alert1"},
		},
		Status: &open_api_models.AlertStatus{
			State:       &active,
			InhibitedBy: []string{},
			SilencedBy:  []string{},
		},
		StartsAt:    convertDateTime(start),
		EndsAt:      convertDateTime(time.Time{}),
		UpdatedAt:   convertDateTime(updated),
		Fingerprint: &fp,
		Receivers:   apiReceivers,
	}, openAPIAlert)
}

func TestMatchFilterLabels(t *testing.T) {
	sms := map[string]string{
		"foo": "bar",
	}

	testCases := []struct {
		matcher  labels.MatchType
		name     string
		val      string
		expected bool
	}{
		{labels.MatchEqual, "foo", "bar", true},
		{labels.MatchEqual, "baz", "", true},
		{labels.MatchEqual, "baz", "qux", false},
		{labels.MatchEqual, "baz", "qux|", false},
		{labels.MatchRegexp, "foo", "bar", true},
		{labels.MatchRegexp, "baz", "", true},
		{labels.MatchRegexp, "baz", "qux", false},
		{labels.MatchRegexp, "baz", "qux|", true},
		{labels.MatchNotEqual, "foo", "bar", false},
		{labels.MatchNotEqual, "baz", "", false},
		{labels.MatchNotEqual, "baz", "qux", true},
		{labels.MatchNotEqual, "baz", "qux|", true},
		{labels.MatchNotRegexp, "foo", "bar", false},
		{labels.MatchNotRegexp, "baz", "", false},
		{labels.MatchNotRegexp, "baz", "qux", true},
		{labels.MatchNotRegexp, "baz", "qux|", false},
	}

	for _, tc := range testCases {
		m, err := labels.NewMatcher(tc.matcher, tc.name, tc.val)
		require.NoError(t, err)

		ms := []*labels.Matcher{m}
		require.Equal(t, tc.expected, matchFilterLabels(ms, sms))
	}
}
