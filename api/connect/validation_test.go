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

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/require"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	silencev3alpha "github.com/prometheus/alertmanager/api/silence/v3alpha"
)

func TestSilenceValidation(t *testing.T) {
	t.Parallel()

	validator, err := protovalidate.New(protovalidate.WithMessages(&silencev3alpha.Silence{}))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		mutate func(*silencev3alpha.Silence)
		fields []string
	}{
		{"valid", nil, nil},
		{"default start", func(s *silencev3alpha.Silence) { s.StartsAt = nil }, nil},
		{"no matcher sets", func(s *silencev3alpha.Silence) { s.MatcherSets = nil }, []string{"matcher_sets"}},
		{"empty matcher set", func(s *silencev3alpha.Silence) { s.MatcherSets = []*commonv3alpha.MatcherSet{{}} }, []string{"matcher_sets[0]"}},
		{"reversed time range", func(s *silencev3alpha.Silence) { s.StartsAt, s.EndsAt = s.EndsAt, s.StartsAt }, []string{""}},
		{"missing end", func(s *silencev3alpha.Silence) { s.EndsAt = nil }, []string{"ends_at"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sil, err := silenceFromProto(storedTestSilence("a", silenceTestTime, silenceTestTime.Add(time.Hour)), silenceTestTime)
			require.NoError(t, err)
			if tc.mutate != nil {
				tc.mutate(sil)
			}
			err = validator.Validate(sil)
			if len(tc.fields) == 0 {
				require.NoError(t, err)
				return
			}
			var validationErr *protovalidate.ValidationError
			require.ErrorAs(t, err, &validationErr)
			fields := make([]string, 0, len(validationErr.Violations))
			for _, violation := range validationErr.Violations {
				fields = append(fields, protovalidate.FieldPathString(violation.Proto.GetField()))
			}
			require.ElementsMatch(t, tc.fields, fields)
		})
	}
}

func TestListSilencesResponseValidation(t *testing.T) {
	t.Parallel()

	api := newSilenceListAPI(t)
	sil := storedTestSilence("", silenceTestTime, silenceTestTime.Add(time.Hour))
	sil.CreatedBy, sil.Comment = "", ""
	require.NoError(t, api.silences.Set(t.Context(), sil))
	require.NoError(t, api.silences.Expire(t.Context(), sil.Id))

	_, response := listSilenceIDs(t, api, &silencev3alpha.ListSilencesRequest{})
	require.Len(t, response.Silences, 1)
	require.NoError(t, protovalidate.Validate(response))
}
