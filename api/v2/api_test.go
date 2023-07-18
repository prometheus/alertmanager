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
	alert_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alert"
	alert_info_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertinfo"
	"github.com/prometheus/alertmanager/dispatch"
	"io"
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
	receiver_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/receiver"
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
			404,
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
				unexpiredSid,
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				200,
			},
			{
				"with an expired silence ID - it re-creates the silence",
				expiredSid,
				now.Add(time.Hour),
				now.Add(time.Hour * 2),
				200,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				silence, silenceBytes := createSilence(t, tc.sid, "silenceCreator", tc.start, tc.end)

				api := API{
					uptime:   time.Now(),
					silences: silences,
					logger:   log.NewNopLogger(),
				}

				r, err := http.NewRequest("POST", "/api/v2/silence/${tc.sid}", bytes.NewReader(silenceBytes))
				require.NoError(t, err)

				w := httptest.NewRecorder()
				p := runtime.TextProducer()
				responder := api.postSilencesHandler(silence_ops.PostSilencesParams{
					HTTPRequest: r,
					Silence:     &silence,
				})
				responder.WriteResponse(w, p)
				body, _ := io.ReadAll(w.Result().Body)

				require.Equal(t, tc.expectedCode, w.Code, fmt.Sprintf("test case: %d, response: %s", i, string(body)))
			})
		}
	})
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
	openAPIAlert := AlertToOpenAPIAlert(alert, types.AlertStatus{State: types.AlertStateActive}, receivers)
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
		Receivers: []*open_api_models.Receiver{
			{Name: &receivers[0]},
			{Name: &receivers[1]},
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
		logger:             log.NewNopLogger(),
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
				Labels:   model.LabelSet{"state": "active", "alertname": "alert1"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "unprocessed", "alertname": "alert2"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "silenced_by": "abc", "alertname": "alert3"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "inhibited_by": "abc", "alertname": "alert4"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "alert5"},
				StartsAt: now.Add(-2 * time.Minute),
				EndsAt:   now.Add(-time.Minute),
			},
		},
	}

	for _, tc := range []struct {
		name          string
		booleanParams map[string]*bool
		expectedCode  int
		anames        []string
	}{
		{
			"no filter",
			map[string]*bool{},
			200,
			[]string{"alert1", "alert2", "alert3", "alert4"},
		},
		{
			"status filter",
			map[string]*bool{"active": BoolPointer(true), "silenced": BoolPointer(true), "inhibited": BoolPointer(true)},
			200,
			[]string{"alert1", "alert2", "alert3", "alert4"},
		},
		{
			"status filter - active false",
			map[string]*bool{"active": BoolPointer(false), "silenced": BoolPointer(true), "inhibited": BoolPointer(true)},
			200,
			[]string{"alert2", "alert3", "alert4"},
		},
		{
			"status filter - silenced false",
			map[string]*bool{"active": BoolPointer(true), "silenced": BoolPointer(false), "inhibited": BoolPointer(true)},
			200,
			[]string{"alert1", "alert2", "alert4"},
		},
		{
			"status filter - inhibited false",
			map[string]*bool{"active": BoolPointer(true), "unprocessed": BoolPointer(true), "silenced": BoolPointer(true), "inhibited": BoolPointer(false)},
			200,
			[]string{"alert1", "alert2", "alert3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alertsProvider := newFakeAlerts(alerts)
			api := API{
				uptime:         time.Now(),
				getAlertStatus: newGetAlertStatus(alertsProvider),
				logger:         log.NewNopLogger(),
				alerts:         alertsProvider,
				setAlertStatus: func(model.LabelSet) {},
			}
			api.route = dispatch.NewRoute(&config.Route{Receiver: "def-receiver"}, nil)
			r, err := http.NewRequest("GET", "/api/v2/alerts", nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			p := runtime.TextProducer()
			silence := tc.booleanParams["silenced"]
			if silence == nil {
				silence = BoolPointer(true)
			}
			inhibited := tc.booleanParams["inhibited"]
			if inhibited == nil {
				inhibited = BoolPointer(true)
			}
			active := tc.booleanParams["active"]
			if active == nil {
				active = BoolPointer(true)
			}
			responder := api.getAlertsHandler(alert_ops.GetAlertsParams{
				HTTPRequest: r,
				Silenced:    silence,
				Inhibited:   inhibited,
				Active:      active,
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

func TestListAlertInfosHandler(t *testing.T) {
	now := time.Now()
	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "active", "alertname": "alert1"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "unprocessed", "alertname": "alert2"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "silenced_by": "abc", "alertname": "alert3"},
				StartsAt: now.Add(-time.Minute),
			},
		},
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"state": "suppressed", "inhibited_by": "abc", "alertname": "alert4"},
				StartsAt: now.Add(-time.Minute),
			},
		},
	}

	for _, tc := range []struct {
		name            string
		booleanParams   map[string]*bool
		expectedCode    int
		maxResult       *int64
		nextToken       *string
		anames          []string
		expectNextToken string
	}{
		{
			"no filter",
			map[string]*bool{},
			200,
			nil,
			nil,
			[]string{"alert1", "alert2", "alert3", "alert4"},
			"",
		},
		{
			"status filter",
			map[string]*bool{"active": BoolPointer(true), "silenced": BoolPointer(true), "inhibited": BoolPointer(true)},
			200,
			nil,
			nil,
			[]string{"alert1", "alert2", "alert3", "alert4"},
			"",
		},
		{
			"status filter - active false",
			map[string]*bool{"active": BoolPointer(false), "silenced": BoolPointer(true), "inhibited": BoolPointer(true)},
			200,
			nil,
			nil,
			[]string{"alert2", "alert3", "alert4"},
			"",
		},
		{
			"status filter - silenced false",
			map[string]*bool{"active": BoolPointer(true), "silenced": BoolPointer(false), "inhibited": BoolPointer(true)},
			200,
			nil,
			nil,
			[]string{"alert1", "alert2", "alert4"},
			"",
		},
		{
			"status filter - inhibited false",
			map[string]*bool{"active": BoolPointer(true), "unprocessed": BoolPointer(true), "silenced": BoolPointer(true), "inhibited": BoolPointer(false)},
			200,
			nil,
			nil,
			[]string{"alert1", "alert2", "alert3"},
			"",
		},
		{
			"MaxResults - only 1 alert return",
			map[string]*bool{},
			200,
			convertIntToPointerInt64(int64(1)),
			nil,
			[]string{"alert1"},
			alerts[0].Fingerprint().String(),
		},
		{
			"MaxResults - all alert return",
			map[string]*bool{},
			200,
			convertIntToPointerInt64(int64(8)),
			nil,
			[]string{"alert1", "alert2", "alert3", "alert4"},
			"",
		},
		{
			"MaxResults - has begin next token, max 2 alerts",
			map[string]*bool{},
			200,
			convertIntToPointerInt64(int64(2)),
			convertStringToPointer(alerts[0].Fingerprint().String()),
			[]string{"alert2", "alert3"},
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alertsProvider := newFakeAlerts(alerts)
			api := API{
				uptime:         time.Now(),
				getAlertStatus: newGetAlertStatus(alertsProvider),
				logger:         log.NewNopLogger(),
				alerts:         alertsProvider,
				setAlertStatus: func(model.LabelSet) {},
			}
			api.route = dispatch.NewRoute(&config.Route{Receiver: "def-receiver"}, nil)
			r, err := http.NewRequest("GET", "/api/v2/alerts", nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			p := runtime.TextProducer()
			silence := tc.booleanParams["silenced"]
			if silence == nil {
				silence = BoolPointer(true)
			}
			inhibited := tc.booleanParams["inhibited"]
			if inhibited == nil {
				inhibited = BoolPointer(true)
			}
			active := tc.booleanParams["active"]
			if active == nil {
				active = BoolPointer(true)
			}
			responder := api.getAlertInfosHandler(alert_info_ops.GetAlertInfosParams{
				HTTPRequest: r,
				Silenced:    silence,
				Inhibited:   inhibited,
				Active:      active,
				MaxResults:  tc.maxResult,
				NextToken:   tc.nextToken,
			})
			responder.WriteResponse(w, p)
			body, _ := io.ReadAll(w.Result().Body)

			require.Equal(t, tc.expectedCode, w.Code)
			response := open_api_models.GettableAlertInfos{}
			err = json.Unmarshal(body, &response)
			if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
			anames := []string{}
			for _, a := range response.Alerts {
				name, ok := a.Labels["alertname"]
				if ok {
					anames = append(anames, string(name))
				}
			}
			require.Equal(t, tc.anames, anames)
			require.Equal(t, tc.expectNextToken, response.NextToken)
		})
	}
}

func BoolPointer(b bool) *bool {
	return &b
}

func convertIntToPointerInt64(x int64) *int64 {
	return &x
}

func convertStringToPointer(x string) *string {
	return &x
}
