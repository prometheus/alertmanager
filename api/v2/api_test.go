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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	alert_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alert"
	alertgroup_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroup"
	alertgroupinfolist_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroupinfolist"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/util/callback"

	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	general_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/general"
	receiver_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/receiver"
	silence_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/silence"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
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

func assertEqualStrings(t *testing.T, expected, actual string) {
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

func gettableSilence(id, state string,
	updatedAt, start, end string,
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

func convertIntToPointerInt64(x int64) *int64 {
	return &x
}

func convertStringToPointer(x string) *string {
	return &x
}

func TestGetAlertGroupInfosHandler(t *testing.T) {
	aginfos := dispatch.AlertGroupInfos{
		&dispatch.AlertGroupInfo{
			Labels: model.LabelSet{
				"alertname": "TestingAlert",
				"service":   "api",
			},
			Receiver: "testing",
			ID:       "478b4114226224a35910d449fdba8186ebfb441f",
		},
		&dispatch.AlertGroupInfo{
			Labels: model.LabelSet{
				"alertname": "HighErrorRate",
				"service":   "api",
				"cluster":   "bb",
			},
			Receiver: "prod",
			ID:       "7f4084a078a3fe29d6de82fad15af8f1411e803f",
		},
		&dispatch.AlertGroupInfo{
			Labels: model.LabelSet{
				"alertname": "OtherAlert",
			},
			Receiver: "prod",
			ID:       "d525244929240cbdb75a497913c1890ab8de1962",
		},
		&dispatch.AlertGroupInfo{
			Labels: model.LabelSet{
				"alertname": "HighErrorRate",
				"service":   "api",
				"cluster":   "aa",
			},
			Receiver: "prod",
			ID:       "d73984d43949112ae1ea59dcc5af4af7b630a5b1",
		},
	}
	for _, tc := range []struct {
		maxResult    *int64
		nextToken    *string
		body         string
		expectedCode int
	}{
		// Invalid next token.
		{
			convertIntToPointerInt64(int64(1)),
			convertStringToPointer("$$$"),
			`failed to parse NextToken param: $$$`,
			400,
		},
		// Invalid next token.
		{
			convertIntToPointerInt64(int64(1)),
			convertStringToPointer("1234s"),
			`failed to parse NextToken param: 1234s`,
			400,
		},
		// Invalid MaxResults.
		{
			convertIntToPointerInt64(int64(-1)),
			convertStringToPointer("478b4114226224a35910d449fdba8186ebfb441f"),
			`failed to parse MaxResults param: -1`,
			400,
		},
		// One item to return, no next token.
		{
			convertIntToPointerInt64(int64(1)),
			nil,
			`{"alertGroupInfoList":[{"id":"478b4114226224a35910d449fdba8186ebfb441f","labels":{"alertname":"TestingAlert","service":"api"},"receiver":{"name":"testing"}}],"nextToken":"478b4114226224a35910d449fdba8186ebfb441f"}`,
			200,
		},
		// One item to return, has next token.
		{
			convertIntToPointerInt64(int64(1)),
			convertStringToPointer("478b4114226224a35910d449fdba8186ebfb441f"),
			`{"alertGroupInfoList":[{"id":"7f4084a078a3fe29d6de82fad15af8f1411e803f","labels":{"alertname":"HighErrorRate","cluster":"bb","service":"api"},"receiver":{"name":"prod"}}],"nextToken":"7f4084a078a3fe29d6de82fad15af8f1411e803f"}`,
			200,
		},
		// Five item to return, has next token.
		{
			convertIntToPointerInt64(int64(5)),
			convertStringToPointer("7f4084a078a3fe29d6de82fad15af8f1411e803f"),
			`{"alertGroupInfoList":[{"id":"d525244929240cbdb75a497913c1890ab8de1962","labels":{"alertname":"OtherAlert"},"receiver":{"name":"prod"}},{"id":"d73984d43949112ae1ea59dcc5af4af7b630a5b1","labels":{"alertname":"HighErrorRate","cluster":"aa","service":"api"},"receiver":{"name":"prod"}}]}`,
			200,
		},
		// Return all results.
		{
			nil,
			nil,
			`{"alertGroupInfoList":[{"id":"478b4114226224a35910d449fdba8186ebfb441f","labels":{"alertname":"TestingAlert","service":"api"},"receiver":{"name":"testing"}},{"id":"7f4084a078a3fe29d6de82fad15af8f1411e803f","labels":{"alertname":"HighErrorRate","cluster":"bb","service":"api"},"receiver":{"name":"prod"}},{"id":"d525244929240cbdb75a497913c1890ab8de1962","labels":{"alertname":"OtherAlert"},"receiver":{"name":"prod"}},{"id":"d73984d43949112ae1ea59dcc5af4af7b630a5b1","labels":{"alertname":"HighErrorRate","cluster":"aa","service":"api"},"receiver":{"name":"prod"}}]}`,
			200,
		},
		// return 0 result
		{
			convertIntToPointerInt64(int64(0)),
			nil,
			`{"alertGroupInfoList":[]}`,
			200,
		},
	} {
		api := API{
			uptime: time.Now(),
			alertGroupInfos: func(f func(*dispatch.Route) bool) dispatch.AlertGroupInfos {
				return aginfos
			},
			logger: log.NewNopLogger(),
		}
		r, err := http.NewRequest("GET", "/api/v2/alertgroups", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		p := runtime.TextProducer()
		responder := api.getAlertGroupInfoListHandler(alertgroupinfolist_ops.GetAlertGroupInfoListParams{
			MaxResults:  tc.maxResult,
			NextToken:   tc.nextToken,
			HTTPRequest: r,
		})
		responder.WriteResponse(w, p)
		body, _ := io.ReadAll(w.Result().Body)

		require.Equal(t, tc.expectedCode, w.Code)
		require.Equal(t, tc.body, string(body))
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
	require.NoError(t, silences.Set(unexpiredSil))

	expiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	require.NoError(t, silences.Set(expiredSil))
	require.NoError(t, silences.Expire(expiredSil.Id))

	for i, tc := range []struct {
		sid          string
		expectedCode int
	}{
		{
			"unknownSid",
			404,
		},
		{
			unexpiredSil.Id,
			200,
		},
		{
			expiredSil.Id,
			200,
		},
	} {
		api := API{
			uptime:   time.Now(),
			silences: silences,
			logger:   promslog.NewNopLogger(),
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
		body, _ := io.ReadAll(w.Result().Body)

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
	require.NoError(t, silences.Set(unexpiredSil))

	expiredSil := &silencepb.Silence{
		Matchers:  []*silencepb.Matcher{m},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(time.Hour),
		UpdatedAt: now,
	}
	require.NoError(t, silences.Set(expiredSil))
	require.NoError(t, silences.Expire(expiredSil.Id))

	t.Run("Silences CRUD", func(t *testing.T) {
		for i, tc := range []struct {
			name         string
			sid          string
			start, end   time.Time
			expectedCode int
		}{
			{
				"with an non-existent silence ID - it returns 404",
				"unknownSid",
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				404,
			},
			{
				"with no silence ID - it creates the silence",
				"",
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				200,
			},
			{
				"with an active silence ID - it extends the silence",
				unexpiredSil.Id,
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				200,
			},
			{
				"with an expired silence ID - it re-creates the silence",
				expiredSil.Id,
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				200,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				api := API{
					uptime:   time.Now(),
					silences: silences,
					logger:   promslog.NewNopLogger(),
				}

				sil := createSilence(t, tc.sid, "silenceCreator", tc.start, tc.end)
				w := httptest.NewRecorder()
				postSilences(t, w, api.postSilencesHandler, sil)
				body, _ := io.ReadAll(w.Result().Body)
				require.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("test case: %d, response: %s", i, string(body)))
			})
		}
	})
}

func TestPostSilencesHandlerMissingIdCreatesSilence(t *testing.T) {
	now := time.Now()
	silences := newSilences(t)
	api := API{
		uptime:   time.Now(),
		silences: silences,
		logger:   promslog.NewNopLogger(),
	}

	// Create a new silence. It should be assigned a random UUID.
	sil := createSilence(t, "", "silenceCreator", now.Add(time.Hour), now.Add(time.Hour*2))
	w := httptest.NewRecorder()
	postSilences(t, w, api.postSilencesHandler, sil)
	require.Equal(t, http.StatusOK, w.Code)

	// Get the silences from the API.
	w = httptest.NewRecorder()
	getSilences(t, w, api.getSilencesHandler)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []open_api_models.GettableSilence
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp, 1)

	// Change the ID. It should return 404 Not Found.
	sil = open_api_models.PostableSilence{
		ID:      "unknownID",
		Silence: resp[0].Silence,
	}
	w = httptest.NewRecorder()
	postSilences(t, w, api.postSilencesHandler, sil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Remove the ID. It should duplicate the silence with a different UUID.
	sil = open_api_models.PostableSilence{
		ID:      "",
		Silence: resp[0].Silence,
	}
	w = httptest.NewRecorder()
	postSilences(t, w, api.postSilencesHandler, sil)
	require.Equal(t, http.StatusOK, w.Code)

	// Get the silences from the API. There should now be 2 silences.
	w = httptest.NewRecorder()
	getSilences(t, w, api.getSilencesHandler)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp, 2)
	require.NotEqual(t, resp[0].ID, resp[1].ID)
}

func getSilences(
	t *testing.T,
	w *httptest.ResponseRecorder,
	handlerFunc func(params silence_ops.GetSilencesParams) middleware.Responder,
) {
	r, err := http.NewRequest("GET", "/api/v2/silences", nil)
	require.NoError(t, err)

	p := runtime.TextProducer()
	responder := handlerFunc(silence_ops.GetSilencesParams{
		HTTPRequest: r,
		Filter:      nil,
	})
	responder.WriteResponse(w, p)
}

func postSilences(
	t *testing.T,
	w *httptest.ResponseRecorder,
	handlerFunc func(params silence_ops.PostSilencesParams) middleware.Responder,
	sil open_api_models.PostableSilence,
) {
	b, err := json.Marshal(sil)
	require.NoError(t, err)

	r, err := http.NewRequest("POST", "/api/v2/silences", bytes.NewReader(b))
	require.NoError(t, err)

	p := runtime.TextProducer()
	responder := handlerFunc(silence_ops.PostSilencesParams{
		HTTPRequest: r,
		Silence:     &sil,
	})
	responder.WriteResponse(w, p)
}

func TestCheckSilenceMatchesFilterLabels(t *testing.T) {
	type test struct {
		silenceMatchers []*silencepb.Matcher
		filterMatchers  []*labels.Matcher
		expected        bool
	}

	tests := []test{
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchEqual)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "novalue", labels.MatchEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "(foo|bar)", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "(foo|bar)", labels.MatchRegexp)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "foo", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "(foo|bar)", labels.MatchRegexp)},
			false,
		},

		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_NOT_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotEqual)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_NOT_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotRegexp)},
			true,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_NOT_EQUAL)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotRegexp)},
			false,
		},
		{
			[]*silencepb.Matcher{createSilenceMatcher(t, "label", "value", silencepb.Matcher_NOT_REGEXP)},
			[]*labels.Matcher{createLabelMatcher(t, "label", "value", labels.MatchNotEqual)},
			false,
		},
		{
			[]*silencepb.Matcher{
				createSilenceMatcher(t, "label", "(foo|bar)", silencepb.Matcher_REGEXP),
				createSilenceMatcher(t, "label", "value", silencepb.Matcher_EQUAL),
			},
			[]*labels.Matcher{createLabelMatcher(t, "label", "(foo|bar)", labels.MatchRegexp)},
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
		receivers = []string{"receiver1", "receiver2"}

		alert = &types.Alert{
			Alert: model.Alert{
				Labels:   model.LabelSet{"severity": "critical", "alertname": "alert1"},
				StartsAt: start,
			},
			UpdatedAt: updated,
		}
	)
	openAPIAlert := AlertToOpenAPIAlert(alert, types.AlertStatus{State: types.AlertStateActive}, receivers, nil)
	require.Equal(t, &open_api_models.GettableAlert{
		Annotations: open_api_models.LabelSet{},
		Alert: open_api_models.Alert{
			Labels: open_api_models.LabelSet{"severity": "critical", "alertname": "alert1"},
		},
		StartsAt:    convertDateTime(start),
		EndsAt:      convertDateTime(time.Time{}),
		UpdatedAt:   convertDateTime(updated),
		Fingerprint: &fp,
		Receivers: []*open_api_models.Receiver{
			{Name: &receivers[0]},
			{Name: &receivers[1]},
		},
		Status: &open_api_models.AlertStatus{
			State:       &active,
			InhibitedBy: []string{},
			SilencedBy:  []string{},
			MutedBy:     []string{},
		},
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

func TestGetReceiversHandler(t *testing.T) {
	in := `
route:
    receiver: team-X

receivers:
- name: 'team-X'
- name: 'team-Y'
`
	cfg, _ := config.Load(in)
	api := API{
		uptime:             time.Now(),
		logger:             promslog.NewNopLogger(),
		alertmanagerConfig: cfg,
	}

	for _, tc := range []struct {
		body         string
		expectedCode int
	}{
		{
			`[{"name":"team-X"},{"name":"team-Y"}]`,
			200,
		},
	} {
		r, err := http.NewRequest("GET", "/api/v2/receivers", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		p := runtime.TextProducer()
		responder := api.getReceiversHandler(receiver_ops.GetReceiversParams{
			HTTPRequest: r,
		})
		responder.WriteResponse(w, p)
		body, _ := io.ReadAll(w.Result().Body)

		require.Equal(t, tc.expectedCode, w.Code)
		require.Equal(t, tc.body, string(body))
	}
}

func TestListAlertsHandler(t *testing.T) {
	now := time.Now()
	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert1"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert2"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert3"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert4"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert5"},
				StartsAt: now.Add(-time.Minute),
			},
		},
	}

	for _, tc := range []struct {
		name         string
		expectedCode int
		anames       []string
		callback     callback.Callback
	}{
		{
			"no call back",
			200,
			[]string{"alert3", "alert2", "alert1", "alert5", "alert4"},
			callback.NoopAPICallback{},
		},
		{
			"callback: only return 1 alert",
			200,
			[]string{"alert3"},
			limitNumberOfAlertsReturnedCallback{limit: 1},
		},
		{
			"callback: only return 3 alert",
			200,
			[]string{"alert3", "alert2", "alert1"},
			limitNumberOfAlertsReturnedCallback{limit: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alertsProvider := newFakeAlerts(alerts)
			api := API{
				uptime:         time.Now(),
				getAlertStatus: getAlertStatus,
				logger:         log.NewNopLogger(),
				apiCallback:    tc.callback,
				alerts:         alertsProvider,
				setAlertStatus: func(model.LabelSet) {},
			}
			api.route = dispatch.NewRoute(&config.Route{Receiver: "def-receiver"}, nil)
			r, err := http.NewRequest("GET", "/api/v2/alerts", nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			p := runtime.TextProducer()
			silence := false
			inhibited := false
			active := true
			responder := api.getAlertsHandler(alert_ops.GetAlertsParams{
				HTTPRequest: r,
				Silenced:    &silence,
				Inhibited:   &inhibited,
				Active:      &active,
			})
			responder.WriteResponse(w, p)
			body, _ := io.ReadAll(w.Result().Body)

			require.Equal(t, tc.expectedCode, w.Code)
			retAlerts := open_api_models.GettableAlerts{}
			err = json.Unmarshal(body, &retAlerts)
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
			require.Equal(t, tc.anames, anames)
		})
	}
}

func TestGetAlertGroupsHandler(t *testing.T) {
	var startAt time.Time
	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "active", "alertname": "alert1"},
				StartsAt: startAt,
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "unprocessed", "alertname": "alert2"},
				StartsAt: startAt,
			},
		},
	}
	aginfos := dispatch.AlertGroups{
		&dispatch.AlertGroup{
			Labels: model.LabelSet{
				"alertname": "TestingAlert",
			},
			Receiver: "testing",
			Alerts:   alerts[:1],
		},
		&dispatch.AlertGroup{
			Labels: model.LabelSet{
				"alertname": "HighErrorRate",
			},
			Receiver: "prod",
			Alerts:   alerts[:2],
		},
	}
	for _, tc := range []struct {
		name         string
		numberOfAG   int
		expectedCode int
		callback     callback.Callback
	}{
		{
			"no call back",
			2,
			200,
			callback.NoopAPICallback{},
		},
		{
			"callback: only return 1 alert group",
			1,
			200,
			limitNumberOfAlertsReturnedCallback{limit: 1},
		},
		{
			"callback: only return 2 alert group",
			2,
			200,
			limitNumberOfAlertsReturnedCallback{limit: 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := API{
				uptime: time.Now(),
				alertGroups: func(func(*dispatch.Route) bool, func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string) {
					return aginfos, nil
				},
				getAlertStatus: getAlertStatus,
				logger:         log.NewNopLogger(),
				apiCallback:    tc.callback,
			}
			r, err := http.NewRequest("GET", "/api/v2/alertgroups", nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			p := runtime.TextProducer()
			silence := false
			inhibited := false
			active := true
			responder := api.getAlertGroupsHandler(alertgroup_ops.GetAlertGroupsParams{
				HTTPRequest: r,
				Silenced:    &silence,
				Inhibited:   &inhibited,
				Active:      &active,
			})
			responder.WriteResponse(w, p)
			body, _ := io.ReadAll(w.Result().Body)

			require.Equal(t, tc.expectedCode, w.Code)
			retAlertGroups := open_api_models.AlertGroups{}
			err = json.Unmarshal(body, &retAlertGroups)
			if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
			require.Equal(t, tc.numberOfAG, len(retAlertGroups))
		})
	}
}

type limitNumberOfAlertsReturnedCallback struct {
	limit int
}

func (n limitNumberOfAlertsReturnedCallback) V2GetAlertsCallback(alerts open_api_models.GettableAlerts) (open_api_models.GettableAlerts, error) {
	return alerts[:n.limit], nil
}

func (n limitNumberOfAlertsReturnedCallback) V2GetAlertGroupsCallback(alertgroups open_api_models.AlertGroups) (open_api_models.AlertGroups, error) {
	return alertgroups[:n.limit], nil
}

func getAlertStatus(model.Fingerprint) types.AlertStatus {
	status := types.AlertStatus{SilencedBy: []string{}, InhibitedBy: []string{}}
	status.State = types.AlertStateActive
	return status
}
