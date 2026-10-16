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
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

type silenceListFilter struct {
	matcherSets [][]*silencepb.Matcher
	states      []silencev3alpha.SilenceState
}

// ListSilences returns all matching stored silences in deterministic order.
func (api *API) ListSilences(ctx context.Context, request *connect.Request[silencev3alpha.ListSilencesRequest]) (*connect.Response[silencev3alpha.ListSilencesResponse], error) {
	if err := ctx.Err(); err != nil {
		return nil, normalizeContextError(ctx, err)
	}
	filter, err := parseSilenceListFilter(request.Msg)
	if err != nil {
		return nil, err
	}
	now := api.now()
	if api.silences == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("silence store is not available"))
	}
	stored, _, err := api.silences.Query(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, normalizeContextError(ctx, contextErr)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to query silences"))
	}
	result, err := filterAndConvertSilences(ctx, stored, filter, now)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(result, compareSilences)
	response := &silencev3alpha.ListSilencesResponse{Silences: result}
	if err := ctx.Err(); err != nil {
		return nil, normalizeContextError(ctx, err)
	}
	resp := connect.NewResponse(response)
	resp.Header().Set("Cache-Control", "no-store")
	return resp, nil
}

func filterAndConvertSilences(ctx context.Context, stored []*silencepb.Silence, filter silenceListFilter, now time.Time) ([]*silencev3alpha.GettableSilence, error) {
	result := make([]*silencev3alpha.GettableSilence, 0, len(stored))
	for _, sil := range stored {
		if err := ctx.Err(); err != nil {
			return nil, normalizeContextError(ctx, err)
		}
		if !filter.matchesDefinition(sil) {
			continue
		}
		converted, err := silenceFromProto(sil, now)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("invalid stored silence"))
		}
		if len(filter.states) == 0 || slices.Contains(filter.states, converted.GetStatus().GetState()) {
			result = append(result, converted)
		}
	}
	return result, nil
}

func invalidListField(field, description string) *connect.Error {
	result := connect.NewError(connect.CodeInvalidArgument, errors.New("invalid list request"))
	detail, err := connect.NewErrorDetail(&commonv3alpha.ValidationErrorDetail{
		Violations: []*commonv3alpha.FieldViolation{{Field: field, Description: description}},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, errors.New("failed to encode validation details"))
	}
	result.AddDetail(detail)
	return result
}

func parseSilenceListFilter(request *silencev3alpha.ListSilencesRequest) (silenceListFilter, error) {
	var filter silenceListFilter
	for i, set := range request.GetMatcherSets() {
		if set == nil {
			return filter, invalidListField(fmt.Sprintf("matcher_sets[%d]", i), "matcher set is required")
		}
		matchers := make([]*silencepb.Matcher, 0, len(set.Matchers))
		for j, matcher := range set.Matchers {
			converted, err := matcherToSilenceProto(matcher)
			if err != nil {
				return filter, invalidListField(fmt.Sprintf("matcher_sets[%d].matchers[%d]", i, j), err.Error())
			}
			matchers = append(matchers, converted)
		}
		filter.matcherSets = append(filter.matcherSets, matchers)
	}
	for i, state := range request.GetStates() {
		if state < silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE || state > silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED {
			return filter, invalidListField(fmt.Sprintf("states[%d]", i), "unsupported silence state")
		}
		filter.states = append(filter.states, state)
	}
	return filter, nil
}

func (filter silenceListFilter) matchesDefinition(sil *silencepb.Silence) bool {
	if len(filter.matcherSets) == 0 {
		return true
	}
	for _, requested := range filter.matcherSets {
		if len(requested) == 0 {
			return true
		}
		for _, stored := range sil.MatcherSets {
			if stored == nil {
				continue
			}
			if allMatcherDefinitionsPresent(requested, stored.Matchers) {
				return true
			}
		}
	}
	return false
}

func allMatcherDefinitionsPresent(requested, stored []*silencepb.Matcher) bool {
	for _, matcher := range requested {
		if !slices.ContainsFunc(stored, func(candidate *silencepb.Matcher) bool {
			return candidate != nil && matcher.Name == candidate.Name && matcher.Pattern == candidate.Pattern && matcher.Type == candidate.Type
		}) {
			return false
		}
	}
	return true
}

func compareSilences(a, b *silencev3alpha.GettableSilence) int {
	if order := cmp.Compare(a.Status.State, b.Status.State); order != 0 {
		return order
	}
	at, bt := a.Silence.EndsAt.AsTime(), b.Silence.EndsAt.AsTime()
	if a.Status.State == silencev3alpha.SilenceState_SILENCE_STATE_PENDING {
		at, bt = a.Silence.StartsAt.AsTime(), b.Silence.StartsAt.AsTime()
	}
	order := at.Compare(bt)
	if a.Status.State == silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED {
		order = -order
	}
	if order != 0 {
		return order
	}
	return cmp.Compare(a.Silence.Id, b.Silence.Id)
}
