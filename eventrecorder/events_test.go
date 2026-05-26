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

package eventrecorder

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestExtractEventType(t *testing.T) {
	tests := []struct {
		name     string
		event    *eventrecorderpb.EventData
		expected string
	}{
		{
			name:     "startup",
			event:    startupEvent(),
			expected: "alertmanager_startup_event",
		},
		{
			name: "shutdown",
			event: &eventrecorderpb.EventData{
				EventType: &eventrecorderpb.EventData_AlertmanagerShutdownEvent{},
			},
			expected: "alertmanager_shutdown_event",
		},
		{
			name: "alert_created",
			event: &eventrecorderpb.EventData{
				EventType: &eventrecorderpb.EventData_AlertCreated{},
			},
			expected: "alert_created",
		},
		{
			name:     "unknown",
			event:    &eventrecorderpb.EventData{},
			expected: "unknown",
		},
		{
			name:     "nil",
			event:    nil,
			expected: "unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, extractEventType(tc.event))
		})
	}
}

func TestLabelSetAsProto(t *testing.T) {
	ls := model.LabelSet{"foo": "bar", "baz": "qux"}
	proto := LabelSetAsProto(ls)

	require.Len(t, proto.Labels, 2)
	found := map[string]string{}
	for _, lp := range proto.Labels {
		found[lp.Key] = lp.Value
	}
	require.Equal(t, "bar", found["foo"])
	require.Equal(t, "qux", found["baz"])
}

func TestMatcherAsProto(t *testing.T) {
	m, err := labels.NewMatcher(labels.MatchRegexp, "job", "api.*")
	require.NoError(t, err)

	proto := MatcherAsProto(m)
	require.Equal(t, eventrecorderpb.Matcher_TYPE_REGEXP, proto.Type)
	require.Equal(t, "job", proto.Name)
	require.Equal(t, "api.*", proto.Pattern)
	require.NotEmpty(t, proto.Rendered)
}

func TestMatchersAsProto(t *testing.T) {
	m1, err := labels.NewMatcher(labels.MatchEqual, "env", "prod")
	require.NoError(t, err)
	m2, err := labels.NewMatcher(labels.MatchNotEqual, "team", "")
	require.NoError(t, err)

	protos := MatchersAsProto(labels.Matchers{m1, m2})
	require.Len(t, protos, 2)
	require.Equal(t, eventrecorderpb.Matcher_TYPE_EQUAL, protos[0].Type)
	require.Equal(t, eventrecorderpb.Matcher_TYPE_NOT_EQUAL, protos[1].Type)
}
