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
	"fmt"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

func matcherToSilenceProto(m *commonv3alpha.Matcher) (*silencepb.Matcher, error) {
	var matcherType silencepb.Matcher_Type
	switch m.GetType() {
	case commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL:
		matcherType = silencepb.Matcher_EQUAL
	case commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL:
		matcherType = silencepb.Matcher_NOT_EQUAL
	case commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP:
		matcherType = silencepb.Matcher_REGEXP
	case commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP:
		matcherType = silencepb.Matcher_NOT_REGEXP
	default:
		return nil, fmt.Errorf("unknown matcher type %v", m.GetType())
	}
	return &silencepb.Matcher{Name: m.Name, Pattern: m.Value, Type: matcherType}, nil
}

func matcherFromSilenceProto(m *silencepb.Matcher) (*commonv3alpha.Matcher, error) {
	if m == nil {
		return nil, errors.New("missing stored matcher")
	}
	var matcherType commonv3alpha.MatcherType
	switch m.Type {
	case silencepb.Matcher_EQUAL:
		matcherType = commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL
	case silencepb.Matcher_NOT_EQUAL:
		matcherType = commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL
	case silencepb.Matcher_REGEXP:
		matcherType = commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP
	case silencepb.Matcher_NOT_REGEXP:
		matcherType = commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP
	default:
		return nil, fmt.Errorf("unknown stored matcher type %v", m.Type)
	}
	return &commonv3alpha.Matcher{Name: m.Name, Value: m.Pattern, Type: matcherType}, nil
}
