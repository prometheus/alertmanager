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
	"errors"
	"time"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/api/silence/v3alpha/silencev3alphaconnect"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

var _ silencev3alphaconnect.SilenceServiceHandler = (*API)(nil)

var silenceStates = map[silencev3alpha.SilenceState]silence.SilenceState{
	silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE:  silence.SilenceStateActive,
	silencev3alpha.SilenceState_SILENCE_STATE_PENDING: silence.SilenceStatePending,
	silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED: silence.SilenceStateExpired,
}

func silenceFromProto(sil *silencepb.Silence, now time.Time) (*silencev3alpha.Silence, error) {
	if sil == nil || sil.Id == "" || sil.StartsAt.CheckValid() != nil || sil.EndsAt.CheckValid() != nil || sil.UpdatedAt.CheckValid() != nil {
		return nil, errors.New("invalid silence timestamps or ID")
	}
	matcherSets, err := matcherSetsFromSilenceProto(sil.MatcherSets)
	if err != nil {
		return nil, err
	}
	receiverMatcherSets, err := matcherSetsFromSilenceProto(sil.ReceiverMatcherSets)
	if err != nil {
		return nil, err
	}
	// The response shares read-only timestamps and annotations with the store.
	return &silencev3alpha.Silence{
		Id:                  sil.Id,
		MatcherSets:         matcherSets,
		StartsAt:            sil.StartsAt,
		EndsAt:              sil.EndsAt,
		CreatedBy:           sil.CreatedBy,
		Comment:             sil.Comment,
		Annotations:         sil.Annotations,
		UpdatedAt:           sil.UpdatedAt,
		Status:              &silencev3alpha.SilenceStatus{State: stateToProto(silence.StateFromProtoAt(sil, now))},
		ReceiverMatcherSets: receiverMatcherSets,
	}, nil
}

func stateToProto(state silence.SilenceState) silencev3alpha.SilenceState {
	for public, stored := range silenceStates {
		if stored == state {
			return public
		}
	}
	return silencev3alpha.SilenceState_SILENCE_STATE_UNSPECIFIED
}

func matcherSetsFromSilenceProto(sets []*silencepb.MatcherSet) ([]*commonv3alpha.MatcherSet, error) {
	result := make([]*commonv3alpha.MatcherSet, 0, len(sets))
	for _, set := range sets {
		if set == nil {
			return nil, errors.New("missing stored matcher set")
		}
		converted := &commonv3alpha.MatcherSet{Matchers: make([]*commonv3alpha.Matcher, 0, len(set.Matchers))}
		for _, matcher := range set.Matchers {
			m, err := matcherFromSilenceProto(matcher)
			if err != nil {
				return nil, err
			}
			converted.Matchers = append(converted.Matchers, m)
		}
		result = append(result, converted)
	}
	return result, nil
}
