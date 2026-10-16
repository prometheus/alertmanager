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

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/api/silence/v3alpha/silencev3alphaconnect"
)

func createV2Silence(t testing.TB, inst *instance, name string, end time.Time) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"matchers": []map[string]any{{"name": "alertname", "value": name, "isRegex": false, "isEqual": true}},
		"startsAt": time.Now().Add(-time.Hour), "endsAt": end,
		"createdBy": "operator", "comment": "maintenance", "annotations": map[string]string{"ticket": "123"},
	})
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, inst.webURL("/api/v2/silences"), bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := inst.httpClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var result struct {
		SilenceID string `json:"silenceID"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.NotEmpty(t, result.SilenceID)
	return result.SilenceID
}

func TestListSilences(t *testing.T) {
	for _, prefix := range []string{"", "/alertmanager"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			for _, transport := range []struct {
				name   string
				native bool
				opts   []connect.ClientOption
			}{
				{"Connect", false, nil},
				{"Connect GET", false, []connect.ClientOption{connect.WithHTTPGet()}},
				{"gRPC-Web", false, []connect.ClientOption{connect.WithGRPCWeb()}},
				{"gRPC", true, []connect.ClientOption{connect.WithGRPC()}},
			} {
				t.Run(transport.name, func(t *testing.T) {
					inst := startInstance(t, prefix)
					httpClient := connect.HTTPClient(inst.httpClient)
					path := inst.apiPath()
					if transport.native {
						httpClient, path = inst.h2cClient, ""
					}
					client := silencev3alphaconnect.NewSilenceServiceClient(httpClient, inst.baseURL+path, transport.opts...)
					empty, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
					require.NoError(t, err)
					require.Empty(t, empty.Msg.Silences)

					end := time.Now().Add(time.Hour)
					firstID := createV2Silence(t, inst, "First", end)
					secondID := createV2Silence(t, inst, "Second", end.Add(time.Hour))
					listed, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
					require.NoError(t, err)
					require.Len(t, listed.Msg.Silences, 2)
					require.Equal(t, firstID, listed.Msg.Silences[0].Silence.Id)
					require.Equal(t, secondID, listed.Msg.Silences[1].Silence.Id)
					require.Equal(t, map[string]string{"ticket": "123"}, listed.Msg.Silences[0].Silence.Annotations)
					require.Equal(t, silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE, listed.Msg.Silences[0].Status.State)
					require.Equal(t, "no-store", listed.Header().Get("Cache-Control"))
					require.NoError(t, inst.app.Reload())
					reloaded, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
					require.NoError(t, err)
					require.Len(t, reloaded.Msg.Silences, 2)
					require.Equal(t, firstID, reloaded.Msg.Silences[0].Silence.Id)
					require.Equal(t, secondID, reloaded.Msg.Silences[1].Silence.Id)

					filtered, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{
						MatcherSets: []*commonv3alpha.MatcherSet{{Matchers: []*commonv3alpha.Matcher{{Name: "alertname", Value: "Second", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL}}}},
					}))
					require.NoError(t, err)
					require.Len(t, filtered.Msg.Silences, 1)
					require.Equal(t, secondID, filtered.Msg.Silences[0].Silence.Id)
					_, err = client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{99}}))
					require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
					var connectErr *connect.Error
					require.ErrorAs(t, err, &connectErr)
					require.Len(t, connectErr.Details(), 1)

					request, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, inst.webURL("/api/v2/silence/"+firstID), nil)
					require.NoError(t, err)
					deleted, err := inst.httpClient.Do(request)
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, deleted.StatusCode)
					require.NoError(t, deleted.Body.Close())
					expired, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{States: []silencev3alpha.SilenceState{silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED}}))
					require.NoError(t, err)
					require.Len(t, expired.Msg.Silences, 1)
					require.Equal(t, firstID, expired.Msg.Silences[0].Silence.Id)

					v2, err := inst.httpClient.Get(inst.webURL("/api/v2/silences"))
					require.NoError(t, err)
					defer v2.Body.Close()
					require.Equal(t, http.StatusOK, v2.StatusCode)
					var v2Silences []json.RawMessage
					require.NoError(t, json.NewDecoder(v2.Body).Decode(&v2Silences))
					require.Len(t, v2Silences, 2)
				})
			}
		})
	}
}

func TestListSilencesRejectsWrongPrefix(t *testing.T) {
	inst := startInstance(t, "/alertmanager")
	client := silencev3alphaconnect.NewSilenceServiceClient(inst.httpClient, inst.baseURL+"/api")
	_, err := client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
	require.Error(t, err)
	client = silencev3alphaconnect.NewSilenceServiceClient(inst.h2cClient, inst.baseURL+inst.apiPath(), connect.WithGRPC())
	_, err = client.ListSilences(t.Context(), connect.NewRequest(&silencev3alpha.ListSilencesRequest{}))
	require.Error(t, err)
}
