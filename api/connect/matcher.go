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

var matcherTypes = map[commonv3alpha.MatcherType]silencepb.Matcher_Type{
	commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL:      silencepb.Matcher_EQUAL,
	commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL:  silencepb.Matcher_NOT_EQUAL,
	commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP:     silencepb.Matcher_REGEXP,
	commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP: silencepb.Matcher_NOT_REGEXP,
}

func matcherToSilenceProto(m *commonv3alpha.Matcher) (*silencepb.Matcher, error) {
	matcherType, ok := matcherTypes[m.GetType()]
	if !ok {
		return nil, fmt.Errorf("unknown matcher type %v", m.GetType())
	}
	return &silencepb.Matcher{Name: m.Name, Pattern: m.Value, Type: matcherType}, nil
}

func matcherFromSilenceProto(m *silencepb.Matcher) (*commonv3alpha.Matcher, error) {
	if m == nil {
		return nil, errors.New("missing stored matcher")
	}
	for public, stored := range matcherTypes {
		if stored == m.Type {
			return &commonv3alpha.Matcher{Name: m.Name, Value: m.Pattern, Type: public}, nil
		}
	}
	return nil, fmt.Errorf("unknown stored matcher type %v", m.Type)
}
