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
	"errors"
	"fmt"
	"regexp"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/matcher/compat"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

func validateMatcher(m *pb.Matcher) error {
	if !compat.IsValidLabelName(model.LabelName(m.Name)) {
		return fmt.Errorf("invalid label name %q", m.Name)
	}
	switch m.Type {
	case pb.Matcher_EQUAL, pb.Matcher_NOT_EQUAL:
		if !model.LabelValue(m.Pattern).IsValid() {
			return fmt.Errorf("invalid label value %q", m.Pattern)
		}
	case pb.Matcher_REGEXP, pb.Matcher_NOT_REGEXP:
		if _, err := regexp.Compile(m.Pattern); err != nil {
			return fmt.Errorf("invalid regular expression %q: %w", m.Pattern, err)
		}
	default:
		return fmt.Errorf("unknown matcher type %q", m.Type)
	}
	return nil
}

func matchesEmpty(m *pb.Matcher) bool {
	switch m.Type {
	case pb.Matcher_EQUAL:
		return m.Pattern == ""
	case pb.Matcher_REGEXP:
		matched, _ := regexp.MatchString(m.Pattern, "")
		return matched
	default:
		return false
	}
}

func validateSilence(s *pb.Silence) error {
	// Convert old-style Matchers to MatcherSets for backward compatibility
	postprocessUnmarshalledSilence(s)

	if len(s.MatcherSets) == 0 {
		return errors.New("at least one matcher set required")
	}

	for setIdx, ms := range s.MatcherSets {
		if len(ms.Matchers) == 0 {
			return fmt.Errorf("matcher set %d is empty", setIdx)
		}
		allMatchEmpty := true

		for i, m := range ms.Matchers {
			if err := validateMatcher(m); err != nil {
				return fmt.Errorf("invalid label matcher %d in set %d: %w", i, setIdx, err)
			}
			allMatchEmpty = allMatchEmpty && matchesEmpty(m)
		}
		if allMatchEmpty {
			return fmt.Errorf("matcher set %d: at least one matcher must not match the empty string", setIdx)
		}
	}

	if s.StartsAt == nil || s.StartsAt.AsTime().IsZero() {
		return errors.New("invalid zero start timestamp")
	}
	if s.EndsAt == nil || s.EndsAt.AsTime().IsZero() {
		return errors.New("invalid zero end timestamp")
	}
	if s.EndsAt.AsTime().Before(s.StartsAt.AsTime()) {
		return errors.New("end time must not be before start time")
	}
	return nil
}
