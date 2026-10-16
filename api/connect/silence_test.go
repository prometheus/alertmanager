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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

var silenceTestTime = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

func storedTestSilence(id string, start, end time.Time) *silencepb.Silence {
	return &silencepb.Silence{
		Id: id, StartsAt: timestamppb.New(start), EndsAt: timestamppb.New(end), UpdatedAt: timestamppb.New(silenceTestTime),
		CreatedBy: "operator", Comment: "maintenance", Annotations: map[string]string{"ticket": "123"},
		MatcherSets: []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{Name: "alertname", Pattern: "Foo", Type: silencepb.Matcher_EQUAL}}}},
	}
}

func TestSilenceMatcherConversion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		public commonv3alpha.MatcherType
		stored silencepb.Matcher_Type
	}{
		{commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL, silencepb.Matcher_EQUAL},
		{commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL, silencepb.Matcher_NOT_EQUAL},
		{commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP, silencepb.Matcher_REGEXP},
		{commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP, silencepb.Matcher_NOT_REGEXP},
	} {
		t.Run(tc.public.String(), func(t *testing.T) {
			t.Parallel()
			input := &commonv3alpha.Matcher{Name: "alertname", Value: "Foo", Type: tc.public}
			stored, err := matcherToSilenceProto(input)
			require.NoError(t, err)
			require.Equal(t, tc.stored, stored.Type)
			output, err := matcherFromSilenceProto(stored)
			require.NoError(t, err)
			require.True(t, proto.Equal(input, output))
		})
	}
}

func TestSilenceConversionRejectsInvalidStoredData(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*silencepb.Silence)
	}{
		{"missing ID", func(s *silencepb.Silence) { s.Id = "" }},
		{"missing start", func(s *silencepb.Silence) { s.StartsAt = nil }},
		{"bad end", func(s *silencepb.Silence) { s.EndsAt = &timestamppb.Timestamp{Nanos: -1} }},
		{"missing update", func(s *silencepb.Silence) { s.UpdatedAt = nil }},
		{"missing set", func(s *silencepb.Silence) { s.MatcherSets = []*silencepb.MatcherSet{nil} }},
		{"missing matcher", func(s *silencepb.Silence) { s.MatcherSets[0].Matchers[0] = nil }},
		{"bad matcher type", func(s *silencepb.Silence) { s.MatcherSets[0].Matchers[0].Type = 99 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sil := storedTestSilence("a", silenceTestTime.Add(-time.Hour), silenceTestTime.Add(time.Hour))
			tc.mutate(sil)
			_, err := silenceFromProto(sil, silenceTestTime)
			require.Error(t, err)
		})
	}
	_, err := silenceFromProto(nil, silenceTestTime)
	require.Error(t, err)
}

func TestMatcherSetsFromSilenceProto(t *testing.T) {
	t.Parallel()

	t.Run("empty sets", func(t *testing.T) {
		t.Parallel()
		sets, err := matcherSetsFromSilenceProto(nil)
		require.NoError(t, err)
		require.NotNil(t, sets)
		require.Empty(t, sets)
	})

	t.Run("preserves sets and copies matchers", func(t *testing.T) {
		t.Parallel()
		stored := []*silencepb.MatcherSet{
			{Matchers: []*silencepb.Matcher{{Name: "alertname", Pattern: "Foo", Type: silencepb.Matcher_EQUAL}}},
			{Matchers: []*silencepb.Matcher{{Name: "service", Pattern: "api|worker", Type: silencepb.Matcher_REGEXP}}},
		}
		sets, err := matcherSetsFromSilenceProto(stored)
		require.NoError(t, err)
		require.Len(t, sets, 2)
		require.Equal(t, "Foo", sets[0].Matchers[0].Value)
		require.Equal(t, "api|worker", sets[1].Matchers[0].Value)
		require.Equal(t, commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP, sets[1].Matchers[0].Type)
		sets[0].Matchers[0].Value = "changed"
		require.Equal(t, "Foo", stored[0].Matchers[0].Pattern)
	})
}
