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
	"fmt"

	"github.com/prometheus/common/model"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func matcherFromProto(matcher *commonv3alpha.Matcher) (*labels.Matcher, error) {
	if matcher == nil {
		return nil, fmt.Errorf("matcher is required")
	}
	if !compat.IsValidLabelName(model.LabelName(matcher.GetName())) {
		return nil, fmt.Errorf("invalid label name %q", matcher.GetName())
	}
	matcherType, err := matcherTypeFromProto(matcher.GetType())
	if err != nil {
		return nil, err
	}
	result, err := labels.NewMatcher(matcherType, matcher.GetName(), matcher.GetValue())
	if err != nil {
		return nil, fmt.Errorf("invalid matcher for label %q: %w", matcher.GetName(), err)
	}
	return result, nil
}

func matcherTypeFromProto(matcherType commonv3alpha.MatcherType) (labels.MatchType, error) {
	switch matcherType {
	case commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL:
		return labels.MatchEqual, nil
	case commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL:
		return labels.MatchNotEqual, nil
	case commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP:
		return labels.MatchRegexp, nil
	case commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP:
		return labels.MatchNotRegexp, nil
	default:
		return 0, fmt.Errorf("unsupported matcher type %q", matcherType)
	}
}

func matcherToProto(matcher *labels.Matcher) (*commonv3alpha.Matcher, error) {
	if matcher == nil {
		return nil, fmt.Errorf("matcher is required")
	}
	matcherType, err := matcherTypeToProto(matcher.Type)
	if err != nil {
		return nil, err
	}
	return &commonv3alpha.Matcher{
		Name:  matcher.Name,
		Value: matcher.Value,
		Type:  matcherType,
	}, nil
}

func matcherTypeToProto(matcherType labels.MatchType) (commonv3alpha.MatcherType, error) {
	switch matcherType {
	case labels.MatchEqual:
		return commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL, nil
	case labels.MatchNotEqual:
		return commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL, nil
	case labels.MatchRegexp:
		return commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP, nil
	case labels.MatchNotRegexp:
		return commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP, nil
	default:
		return commonv3alpha.MatcherType_MATCHER_TYPE_UNSPECIFIED, fmt.Errorf("unsupported internal matcher type %d", matcherType)
	}
}

func matcherSetsFromProto(sets []*commonv3alpha.MatcherSet) ([]labels.Matchers, error) {
	result := make([]labels.Matchers, 0, len(sets))
	for setIndex, set := range sets {
		if set == nil {
			return nil, fmt.Errorf("matcher set %d is required", setIndex)
		}
		matchers := make(labels.Matchers, 0, len(set.GetMatchers()))
		for matcherIndex, matcher := range set.GetMatchers() {
			converted, err := matcherFromProto(matcher)
			if err != nil {
				return nil, fmt.Errorf("invalid matcher %d in set %d: %w", matcherIndex, setIndex, err)
			}
			matchers = append(matchers, converted)
		}
		result = append(result, matchers)
	}
	return result, nil
}

func matcherSetsToProto(sets []labels.Matchers) ([]*commonv3alpha.MatcherSet, error) {
	result := make([]*commonv3alpha.MatcherSet, 0, len(sets))
	for setIndex, set := range sets {
		matchers := make([]*commonv3alpha.Matcher, 0, len(set))
		for matcherIndex, matcher := range set {
			converted, err := matcherToProto(matcher)
			if err != nil {
				return nil, fmt.Errorf("invalid matcher %d in set %d: %w", matcherIndex, setIndex, err)
			}
			matchers = append(matchers, converted)
		}
		result = append(result, &commonv3alpha.MatcherSet{Matchers: matchers})
	}
	return result, nil
}

func matchesMatcherSets(sets []labels.Matchers, labelSet model.LabelSet) bool {
	if len(sets) == 0 {
		return true
	}
	for _, set := range sets {
		if set.Matches(labelSet) {
			return true
		}
	}
	return false
}
