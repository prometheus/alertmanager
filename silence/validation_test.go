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

package silence

import (
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func TestValidateClassicMatcher(t *testing.T) {
	cases := []struct {
		m   *pb.Matcher
		err string
	}{
		{
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "00",
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label name",
		}, {
			m: &pb.Matcher{
				Name:    "\xf0\x9f\x99\x82", // U+1F642
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label name",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "((",
				Type:    pb.Matcher_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "))",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xff",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label value",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xf0\x9f\x99\x82", // U+1F642
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    333,
			},
			err: "unknown matcher type",
		},
	}

	for _, c := range cases {
		checkErr(t, c.err, ValidateMatcher(c.m))
	}
}

func TestValidateUTF8Matcher(t *testing.T) {
	cases := []struct {
		m   *pb.Matcher
		err string
	}{
		{
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "00",
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "\xf0\x9f\x99\x82", // U+1F642
				Pattern: "a",
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "((",
				Type:    pb.Matcher_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "))",
				Type:    pb.Matcher_NOT_REGEXP,
			},
			err: "invalid regular expression",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xff",
				Type:    pb.Matcher_EQUAL,
			},
			err: "invalid label value",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "\xf0\x9f\x99\x82", // U+1F642
				Type:    pb.Matcher_EQUAL,
			},
			err: "",
		}, {
			m: &pb.Matcher{
				Name:    "a",
				Pattern: "b",
				Type:    333,
			},
			err: "unknown matcher type",
		},
	}

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	for _, c := range cases {
		checkErr(t, c.err, ValidateMatcher(c.m))
	}
}

func TestValidateSilence(t *testing.T) {
	var (
		now            = time.Now().UTC()
		zeroTimestamp  *timestamppb.Timestamp // nil represents zero timestamp
		validTimestamp = timestamppb.New(now)
	)
	cases := []struct {
		s   *pb.Silence
		err string
	}{
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "matcher set 0 is empty",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: "b"},
					{Name: "00", Pattern: "b"},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid label matcher",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				Matchers: []*pb.Matcher{
					{Name: "a", Pattern: ""},
					{Name: "b", Pattern: ".*", Type: pb.Matcher_REGEXP},
				},
				StartsAt:  validTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "at least one matcher must not match the empty string",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  timestamppb.New(now),
				EndsAt:    timestamppb.New(now.Add(-time.Second)),
				UpdatedAt: validTimestamp,
			},
			err: "end time must not be before start time",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  zeroTimestamp,
				EndsAt:    validTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero start timestamp",
		},
		{
			s: &pb.Silence{
				Id: "some_id",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{
						{Name: "a", Pattern: "b"},
					},
				}},
				StartsAt:  validTimestamp,
				EndsAt:    zeroTimestamp,
				UpdatedAt: validTimestamp,
			},
			err: "invalid zero end timestamp",
		},
	}
	for _, c := range cases {
		checkErr(t, c.err, validateSilence(c.s))
	}
}
