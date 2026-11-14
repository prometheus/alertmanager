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
	"cmp"
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

// ListSilences returns matching stored silences in deterministic order.
// The response references read-only store data. Callers must not modify it.
func (api *API) ListSilences(ctx context.Context, request *connect.Request[silencev3alpha.ListSilencesRequest]) (*connect.Response[silencev3alpha.ListSilencesResponse], error) {
	params, err := listSilencesParams(request.Msg)
	if err != nil {
		return nil, internalError(ctx, api.logger, "failed to convert silence filters", err)
	}
	now := api.now()
	stored, _, err := api.silences.QueryReadOnlyAt(ctx, now, params...)
	if err != nil {
		return nil, internalError(ctx, api.logger, "failed to query silences", err)
	}
	result := make([]*silencev3alpha.Silence, 0, len(stored))
	for _, sil := range stored {
		if err := ctx.Err(); err != nil {
			return nil, normalizeContextError(ctx, err)
		}
		converted, err := silenceFromProto(sil, now)
		if err != nil {
			return nil, internalError(ctx, api.logger.With("silence_id", sil.GetId()), "invalid stored silence", err)
		}
		result = append(result, converted)
	}
	if err := ctx.Err(); err != nil {
		return nil, normalizeContextError(ctx, err)
	}
	slices.SortFunc(result, compareSilences)
	return connect.NewResponse(&silencev3alpha.ListSilencesResponse{
		Silences:    result,
		EvaluatedAt: timestamppb.New(now),
	}), nil
}

func listSilencesParams(request *silencev3alpha.ListSilencesRequest) ([]silence.QueryParam, error) {
	var params []silence.QueryParam
	if len(request.GetMatcherFilters()) > 0 {
		sets := make([]*silencepb.MatcherSet, 0, len(request.MatcherFilters))
		for _, set := range request.MatcherFilters {
			converted := &silencepb.MatcherSet{Matchers: make([]*silencepb.Matcher, 0, len(set.GetMatchers()))}
			for _, matcher := range set.GetMatchers() {
				m, err := matcherToSilenceProto(matcher)
				if err != nil {
					return nil, err
				}
				converted.Matchers = append(converted.Matchers, m)
			}
			sets = append(sets, converted)
		}
		params = append(params, silence.QHasMatchers(sets...))
	}
	if len(request.GetStates()) > 0 {
		states := make([]silence.SilenceState, 0, len(request.States))
		for _, state := range request.States {
			converted, ok := silenceStates[state]
			if !ok {
				return nil, fmt.Errorf("unknown silence state %v", state)
			}
			states = append(states, converted)
		}
		params = append(params, silence.QState(states...))
	}
	return params, nil
}

func compareSilences(a, b *silencev3alpha.Silence) int {
	if order := cmp.Compare(a.Status.State, b.Status.State); order != 0 {
		return order
	}
	at, bt := a.EndsAt.AsTime(), b.EndsAt.AsTime()
	if a.Status.State == silencev3alpha.SilenceState_SILENCE_STATE_PENDING {
		at, bt = a.StartsAt.AsTime(), b.StartsAt.AsTime()
	}
	order := at.Compare(bt)
	if a.Status.State == silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED {
		order = -order
	}
	if order != 0 {
		return order
	}
	return cmp.Compare(a.Id, b.Id)
}
