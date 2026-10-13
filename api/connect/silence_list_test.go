// Copyright The Prometheus Authors
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

package apiconnect

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/api/silence/v3alpha/silencev3alphaconnect"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

func mergeTestSilences(t testing.TB, store *silence.Silences, silences ...*silencepb.Silence) {
	t.Helper()
	var encoded bytes.Buffer
	for _, sil := range silences {
		_, err := protodelim.MarshalTo(&encoded, &silencepb.MeshSilence{Silence: sil, ExpiresAt: timestamppb.New(silenceTestTime.Add(24 * time.Hour))})
		require.NoError(t, err)
	}
	require.NoError(t, store.Merge(encoded.Bytes()))
}

func newSilenceListAPI(t testing.TB, silences ...*silencepb.Silence) *API {
	t.Helper()
	store, err := silence.New(silence.Options{Metrics: prometheus.NewRegistry(), Retention: 24 * time.Hour})
	require.NoError(t, err)
	mergeTestSilences(t, store, silences...)
	api := NewAPI(Options{Silences: store})
	api.now = func() time.Time { return silenceTestTime }
	return api
}

func listSilenceIDs(t testing.TB, api *API, request *silencev3alpha.ListSilencesRequest) ([]string, *silencev3alpha.ListSilencesResponse) {
	t.Helper()
	response, err := api.ListSilences(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	ids := make([]string, 0, len(response.Msg.Silences))
	for _, sil := range response.Msg.Silences {
		ids = append(ids, sil.Id)
	}
	return ids, response.Msg
}

func TestListSilences(t *testing.T) {
	t.Parallel()

	t.Run("empty store", func(t *testing.T) {
		t.Parallel()
		ids, _ := listSilenceIDs(t, newSilenceListAPI(t), &silencev3alpha.ListSilencesRequest{})
		require.Empty(t, ids)
	})

	t.Run("state and time ordering with ID ties", func(t *testing.T) {
		t.Parallel()
		now := silenceTestTime
		api := newSilenceListAPI(t,
			storedTestSilence("active-b", now.Add(-time.Hour), now.Add(time.Hour)),
			storedTestSilence("expired-old", now.Add(-3*time.Hour), now.Add(-time.Hour)),
			storedTestSilence("pending-b", now.Add(time.Minute), now.Add(time.Hour)),
			storedTestSilence("active-later", now.Add(-time.Hour), now.Add(2*time.Hour)),
			storedTestSilence("active-a", now.Add(-time.Hour), now.Add(time.Hour)),
			storedTestSilence("expired-boundary", now.Add(-time.Hour), now),
			storedTestSilence("pending-a", now.Add(time.Minute), now.Add(time.Hour)),
			storedTestSilence("expired-zero", now, now),
		)
		ids, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
		require.Equal(t, []string{"active-a", "active-b", "active-later", "pending-a", "pending-b", "expired-boundary", "expired-zero", "expired-old"}, ids)
		for _, state := range []silencev3alpha.SilenceState{
			silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE,
			silencev3alpha.SilenceState_SILENCE_STATE_PENDING,
			silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED,
		} {
			_, filtered := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{state}})
			require.NotEmpty(t, filtered.Silences)
			for _, sil := range filtered.Silences {
				require.Equal(t, state, sil.Status.State)
			}
		}
		require.Equal(t, silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED, response.Silences[5].Status.State)
	})

	t.Run("multi-set HA reads", func(t *testing.T) {
		t.Parallel()
		sil := storedTestSilence("multi", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
		sil.MatcherSets = append(sil.MatcherSets, &silencepb.MatcherSet{Matchers: []*silencepb.Matcher{{Name: "service", Pattern: "api|worker", Type: silencepb.Matcher_REGEXP}}})
		sil.ReceiverMatcherSets = []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{Name: "team", Pattern: "platform", Type: silencepb.Matcher_EQUAL}}}}
		api := newSilenceListAPI(t, sil)
		_, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
		got := response.Silences[0]
		require.Len(t, got.MatcherSets, 2)
		require.Equal(t, "operator", got.CreatedBy)
		require.Equal(t, "maintenance", got.Comment)
		require.Equal(t, map[string]string{"ticket": "123"}, got.Annotations)
		require.Equal(t, commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP, got.MatcherSets[1].Matchers[0].Type)
		require.Equal(t, "Foo", got.MatcherSets[0].Matchers[0].Value)
		require.Equal(t, "api|worker", got.MatcherSets[1].Matchers[0].Value)
		require.Equal(t, sil.UpdatedAt.AsTime(), got.UpdatedAt.AsTime())
		require.Len(t, got.ReceiverMatcherSets, 1)
		require.Len(t, got.ReceiverMatcherSets[0].Matchers, 1)
		require.Equal(t, "team", got.ReceiverMatcherSets[0].Matchers[0].Name)
		require.Equal(t, "platform", got.ReceiverMatcherSets[0].Matchers[0].Value)
		require.Equal(t, commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL, got.ReceiverMatcherSets[0].Matchers[0].Type)
		stored, _, err := api.silences.QueryReadOnlyAt(t.Context(), silenceTestTime)
		require.NoError(t, err)
		require.Same(t, stored[0].StartsAt, got.StartsAt)
	})

	t.Run("definition filters do not evaluate alert label values", func(t *testing.T) {
		t.Parallel()
		sil := storedTestSilence("multi", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
		sil.MatcherSets = []*silencepb.MatcherSet{
			{Matchers: []*silencepb.Matcher{{Name: "alertname", Pattern: "Foo", Type: silencepb.Matcher_EQUAL}}},
			{Matchers: []*silencepb.Matcher{{Name: "severity", Pattern: "critical", Type: silencepb.Matcher_NOT_EQUAL}}},
		}
		api := newSilenceListAPI(t, sil)
		foo := &commonv3alpha.Matcher{Name: "alertname", Value: "Foo", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL}
		severity := &commonv3alpha.Matcher{Name: "severity", Value: "critical", Type: commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL}
		for _, tc := range []struct {
			name string
			sets []*commonv3alpha.MatcherSet
			want []string
		}{
			{"omitted", nil, []string{"multi"}},
			{"empty set", []*commonv3alpha.MatcherSet{{}}, []string{"multi"}},
			{"exact", []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{severity}}}, []string{"multi"}},
			{"OR", []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{foo}}, {Matchers: []*commonv3alpha.Matcher{severity}}}, []string{"multi"}},
			{"AND must stay within a stored set", []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{foo, severity}}}, []string{}},
			{"regex must have identical type and value", []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname", Value: "Fo.*", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP}}}}, []string{}},
			{"invalid regex is compared literally", []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname", Value: "[", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP}}}}, []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ids, _ := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{MatcherFilters: tc.sets})
				require.Equal(t, tc.want, ids)
			})
		}
	})

	t.Run("invalid filters have field details", func(t *testing.T) {
		t.Parallel()
		api := newSilenceListAPI(t)
		srv := newTestServer(t, api.Handler(), false)
		client := silencev3alphaconnect.NewSilenceServiceClient(srv.Client(), srv.URL)
		for _, tc := range []struct {
			name    string
			request *silencev3alpha.ListSilencesRequest
			fields  []string
		}{
			{"unspecified state", &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{0}}, []string{"states[0]"}},
			{"unknown state", &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{99}}, []string{"states[0]"}},
			{"missing matcher fields", &silencev3alpha.ListSilencesRequest{MatcherFilters: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{}}}}}, []string{"matcher_filters[0].matchers[0].name", "matcher_filters[0].matchers[0].type"}},
			{"unknown matcher type", &silencev3alpha.ListSilencesRequest{MatcherFilters: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname", Type: 99}}}}}, []string{"matcher_filters[0].matchers[0].type"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := client.ListSilences(t.Context(), connect.NewRequest(tc.request))
				require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				require.Len(t, connectErr.Details(), 1)
				value, err := connectErr.Details()[0].Value()
				require.NoError(t, err)
				detail, ok := value.(*errdetails.BadRequest)
				require.True(t, ok)
				fields := make([]string, 0, len(detail.FieldViolations))
				for _, violation := range detail.FieldViolations {
					fields = append(fields, violation.Field)
					require.NotEmpty(t, violation.Description)
					require.Regexp(t, `^[A-Z][A-Z0-9_]+$`, violation.Reason)
				}
				require.ElementsMatch(t, tc.fields, fields)
			})
		}
	})
}

func TestListSilencesContextErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cause error
		code  connect.Code
	}{
		{"canceled", context.Canceled, connect.CodeCanceled},
		{"deadline exceeded", context.DeadlineExceeded, connect.CodeDeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := newSilenceListAPI(t)
			var logs bytes.Buffer
			api.logger = slog.New(slog.NewTextHandler(&logs, nil))
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(tc.cause)
			_, err := api.ListSilences(ctx, connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
			require.Equal(t, tc.code, connect.CodeOf(err))
			require.Empty(t, logs.String())
		})
	}
}

func TestListSilencesLogsInvalidStoredData(t *testing.T) {
	t.Parallel()

	sil := storedTestSilence("invalid", silenceTestTime, silenceTestTime.Add(time.Hour))
	sil.EndsAt.Nanos = -1
	api := newSilenceListAPI(t, sil)
	var logs bytes.Buffer
	api.logger = slog.New(slog.NewTextHandler(&logs, nil))
	_, err := api.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
	require.EqualError(t, err, "internal: invalid stored silence")
	require.Contains(t, logs.String(), "invalid silence timestamps or ID")
	require.Contains(t, logs.String(), "silence_id=invalid")
}

func TestListSilencesConcurrentMerge(t *testing.T) {
	t.Parallel()
	sil := storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
	api := newSilenceListAPI(t, sil)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 30 {
				response, err := api.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
				require.NoError(t, err)
				_, err = proto.Marshal(response.Msg)
				require.NoError(t, err)
			}
		})
	}
	for i := range 30 {
		sil.UpdatedAt = timestamppb.New(silenceTestTime.Add(time.Duration(i+1) * time.Second))
		mergeTestSilences(t, api.silences, sil)
	}
	group.Wait()
}

func TestListSilencesOverHTTP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []connect.ClientOption
	}{
		{"Connect", nil},
		{"Connect GET", []connect.ClientOption{connect.WithHTTPGet()}},
		{"gRPC-Web", []connect.ClientOption{connect.WithGRPCWeb()}},
		{"gRPC", []connect.ClientOption{connect.WithGRPC()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sil := storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
			sil.ReceiverMatcherSets = []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{Name: "team", Pattern: "platform", Type: silencepb.Matcher_EQUAL}}}}
			api := newSilenceListAPI(t, sil)
			srv := newTestServer(t, api.Handler(), true)
			client := silencev3alphaconnect.NewSilenceServiceClient(newH2CClient(t, 5*time.Second), srv.URL, tc.opts...)
			response, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
			require.NoError(t, err)
			require.Len(t, response.Msg.Silences, 1)
			require.Equal(t, "a", response.Msg.Silences[0].Id)
			require.Len(t, response.Msg.Silences[0].ReceiverMatcherSets, 1)
			require.Len(t, response.Msg.Silences[0].ReceiverMatcherSets[0].Matchers, 1)
			require.Equal(t, "platform", response.Msg.Silences[0].ReceiverMatcherSets[0].Matchers[0].Value)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			_, err = client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{99}}))
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			require.Len(t, connectErr.Details(), 1)
			value, err := connectErr.Details()[0].Value()
			require.NoError(t, err)
			require.IsType(t, &errdetails.BadRequest{}, value)
		})
	}
}

func TestListSilencesUsesOneEvaluationTime(t *testing.T) {
	t.Parallel()
	api := newSilenceListAPI(t,
		storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Second)),
		storedTestSilence("b", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Second)),
	)
	calls := 0
	api.now = func() time.Time {
		calls++
		return silenceTestTime.Add(time.Duration(calls-1) * time.Second)
	}
	ids, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE}})
	require.Equal(t, []string{"a", "b"}, ids)
	require.Equal(t, 1, calls)
	require.Equal(t, silenceTestTime, response.EvaluatedAt.AsTime())
	for _, sil := range response.Silences {
		require.Equal(t, silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE, sil.Status.State)
	}
}

func TestListSilencesMessageLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		options Options
		request *silencev3alpha.ListSilencesRequest
	}{
		{"read", Options{ReadMaxBytes: 1}, &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE}}},
		{"send", Options{SendMaxBytes: 1}, &silencev3alpha.ListSilencesRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.options.Silences = newSilenceListAPI(t, storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))).silences
			api := NewAPI(tc.options)
			srv := newTestServer(t, api.Handler(), false)
			client := silencev3alphaconnect.NewSilenceServiceClient(srv.Client(), srv.URL)
			_, err := client.ListSilences(t.Context(), connect.NewRequest(tc.request))
			require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
		})
	}
}
