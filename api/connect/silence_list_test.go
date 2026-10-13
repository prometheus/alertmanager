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
	"fmt"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
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
		ids = append(ids, sil.Silence.Id)
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

	t.Run("multi-set HA reads and independent response values", func(t *testing.T) {
		t.Parallel()
		sil := storedTestSilence("multi", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
		sil.MatcherSets = append(sil.MatcherSets, &silencepb.MatcherSet{Matchers: []*silencepb.Matcher{{Name: "service", Pattern: "api|worker", Type: silencepb.Matcher_REGEXP}}})
		api := newSilenceListAPI(t, sil)
		_, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
		got := response.Silences[0]
		require.Len(t, got.Silence.MatcherSets, 2)
		require.Equal(t, "operator", got.Silence.CreatedBy)
		require.Equal(t, "maintenance", got.Silence.Comment)
		require.Equal(t, map[string]string{"ticket": "123"}, got.Silence.Annotations)
		require.Equal(t, commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP, got.Silence.MatcherSets[1].Matchers[0].Type)
		got.Silence.Annotations["ticket"] = "changed"
		got.Silence.MatcherSets[0].Matchers[0].Value = "changed"
		got.Silence.StartsAt.Seconds = 0
		_, unchanged := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
		require.Equal(t, "123", unchanged.Silences[0].Silence.Annotations["ticket"])
		require.Equal(t, "Foo", unchanged.Silences[0].Silence.MatcherSets[0].Matchers[0].Value)
		require.True(t, proto.Equal(sil.StartsAt, unchanged.Silences[0].Silence.StartsAt))
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
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ids, _ := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{MatcherSets: tc.sets})
				require.Equal(t, tc.want, ids)
			})
		}
	})

	t.Run("invalid filters have field details", func(t *testing.T) {
		t.Parallel()
		api := newSilenceListAPI(t)
		for _, tc := range []struct {
			name    string
			request *silencev3alpha.ListSilencesRequest
			field   string
		}{
			{"unspecified state", &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{0}}, "states[0]"},
			{"unknown state", &silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{99}}, "states[0]"},
			{"nil set", &silencev3alpha.ListSilencesRequest{MatcherSets: []*commonv3alpha.MatcherSet{nil}}, "matcher_sets[0]"},
			{"nil matcher", &silencev3alpha.ListSilencesRequest{MatcherSets: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{nil}}}}, "matcher_sets[0].matchers[0]"},
			{"invalid name", &silencev3alpha.ListSilencesRequest{MatcherSets: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL}}}}}, "matcher_sets[0].matchers[0]"},
			{"invalid type", &silencev3alpha.ListSilencesRequest{MatcherSets: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname"}}}}}, "matcher_sets[0].matchers[0]"},
			{"invalid regex", &silencev3alpha.ListSilencesRequest{MatcherSets: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname", Value: "[", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP}}}}}, "matcher_sets[0].matchers[0]"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := api.ListSilences(t.Context(), connect.NewRequest(tc.request))
				require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				require.Len(t, connectErr.Details(), 1)
				value, err := connectErr.Details()[0].Value()
				require.NoError(t, err)
				detail, ok := value.(*commonv3alpha.ValidationErrorDetail)
				require.True(t, ok)
				require.Equal(t, tc.field, detail.Violations[0].Field)
			})
		}
	})

	t.Run("store absence and canceled contexts", func(t *testing.T) {
		t.Parallel()
		_, err := NewAPI(Options{}).ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		api := newSilenceListAPI(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = api.ListSilences(ctx, connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
		ctx, cancel = context.WithDeadline(t.Context(), silenceTestTime.Add(-100*365*24*time.Hour))
		defer cancel()
		_, err = api.ListSilences(ctx, connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
	})
}

func TestListSilencesReturnsAllMatches(t *testing.T) {
	t.Parallel()
	sils := make([]*silencepb.Silence, 1001)
	for i := range sils {
		sils[i] = storedTestSilence(fmt.Sprintf("%04d", i), silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
	}
	api := newSilenceListAPI(t, sils...)
	ids, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
	require.Len(t, response.Silences, len(sils))
	for i, id := range ids {
		require.Equal(t, fmt.Sprintf("%04d", i), id)
	}
}

func TestListSilencesConcurrentMerge(t *testing.T) {
	t.Parallel()
	sil := storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
	api := newSilenceListAPI(t, sil)
	var encoded bytes.Buffer
	sil.UpdatedAt = timestamppb.New(silenceTestTime.Add(time.Second))
	_, err := protodelim.MarshalTo(&encoded, &silencepb.MeshSilence{Silence: sil, ExpiresAt: timestamppb.New(silenceTestTime.Add(24 * time.Hour))})
	require.NoError(t, err)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 30 {
				_, err := api.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
				require.NoError(t, err)
			}
		})
	}
	for range 30 {
		require.NoError(t, api.silences.Merge(encoded.Bytes()))
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
			api := newSilenceListAPI(t, storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour)))
			srv := newTestServer(t, api.Handler(), true)
			client := silencev3alphaconnect.NewSilenceServiceClient(newH2CClient(t, 5*time.Second), srv.URL, tc.opts...)
			response, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
			require.NoError(t, err)
			require.Len(t, response.Msg.Silences, 1)
			require.Equal(t, "a", response.Msg.Silences[0].Silence.Id)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			_, err = client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{99}}))
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			var connectErr *connect.Error
			require.ErrorAs(t, err, &connectErr)
			require.Len(t, connectErr.Details(), 1)
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

func TestFilterAndConvertSilences(t *testing.T) {
	t.Parallel()

	t.Run("filters definitions and states without reordering", func(t *testing.T) {
		t.Parallel()
		other := storedTestSilence("other", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
		other.MatcherSets[0].Matchers[0].Pattern = "Bar"
		stored := []*silencepb.Silence{
			storedTestSilence("active-b", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour)),
			storedTestSilence("pending", silenceTestTime.Add(time.Hour), silenceTestTime.Add(2*time.Hour)),
			other,
			storedTestSilence("expired", silenceTestTime.Add(-time.Hour), silenceTestTime),
			storedTestSilence("active-a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour)),
		}
		result, err := filterAndConvertSilences(t.Context(), stored, silenceListFilter{
			matcherSets: [][]*silencepb.Matcher{{{Name: "alertname", Pattern: "Foo", Type: silencepb.Matcher_EQUAL}}},
			states:      []silencev3alpha.SilenceState{silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE, silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED},
		}, silenceTestTime)
		require.NoError(t, err)
		require.Len(t, result, 3)
		require.Equal(t, "active-b", result[0].Silence.Id)
		require.Equal(t, "expired", result[1].Silence.Id)
		require.Equal(t, "active-a", result[2].Silence.Id)
	})

	t.Run("preserves cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		stored := []*silencepb.Silence{storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))}
		_, err := filterAndConvertSilences(ctx, stored, silenceListFilter{}, silenceTestTime)
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	})

	t.Run("hides invalid stored data details", func(t *testing.T) {
		t.Parallel()
		_, err := filterAndConvertSilences(t.Context(), []*silencepb.Silence{{}}, silenceListFilter{}, silenceTestTime)
		require.EqualError(t, err, "internal: invalid stored silence")
	})
}
