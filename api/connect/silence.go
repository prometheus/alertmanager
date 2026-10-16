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
	"maps"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
	"github.com/prometheus/alertmanager/api/silence/v3alpha/silencev3alphaconnect"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

var _ silencev3alphaconnect.SilenceServiceHandler = (*API)(nil)

func silenceFromProto(sil *silencepb.Silence, now time.Time) (*silencev3alpha.GettableSilence, error) {
	if sil == nil || sil.Id == "" || sil.StartsAt.CheckValid() != nil || sil.EndsAt.CheckValid() != nil || sil.UpdatedAt.CheckValid() != nil {
		return nil, errors.New("invalid silence timestamps or ID")
	}
	matcherSets, err := matcherSetsFromSilenceProto(sil.MatcherSets)
	if err != nil {
		return nil, err
	}
	result := &silencev3alpha.GettableSilence{
		Silence: &silencev3alpha.Silence{
			Id: sil.Id, StartsAt: proto.Clone(sil.StartsAt).(*timestamppb.Timestamp), EndsAt: proto.Clone(sil.EndsAt).(*timestamppb.Timestamp),
			CreatedBy: sil.CreatedBy, Comment: sil.Comment, Annotations: maps.Clone(sil.Annotations),
			MatcherSets: matcherSets,
		},
		UpdatedAt: proto.Clone(sil.UpdatedAt).(*timestamppb.Timestamp),
		Status:    &silencev3alpha.SilenceStatus{},
	}
	switch silence.StateAt(sil.StartsAt.AsTime(), sil.EndsAt.AsTime(), now) {
	case silence.SilenceStateActive:
		result.Status.State = silencev3alpha.SilenceState_SILENCE_STATE_ACTIVE
	case silence.SilenceStatePending:
		result.Status.State = silencev3alpha.SilenceState_SILENCE_STATE_PENDING
	case silence.SilenceStateExpired:
		result.Status.State = silencev3alpha.SilenceState_SILENCE_STATE_EXPIRED
	}
	return result, nil
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
