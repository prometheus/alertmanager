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
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	events "github.com/prometheus/alertmanager/eventrecorder/events/v2"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

func TestAlertEventSnapshotsLabels(t *testing.T) {
	a := &alert.Alert{Alert: model.Alert{
		Labels: model.LabelSet{"alertname": "Down", "severity": "warning"}, Annotations: model.LabelSet{"summary": "test"},
		StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour),
	}}
	event := NewAlertCreatedEvent(a)

	a.Labels["severity"] = "critical"
	a.Annotations["summary"] = "changed"

	got := event.message.GetAlertCreated().Alert
	require.Equal(t, "warning", got.Labels["severity"])
	require.Equal(t, "test", got.Annotations["summary"])
}

func TestSilenceEventSnapshotsAnnotationsAndMatchers(t *testing.T) {
	silence := &silencepb.Silence{
		Annotations: map[string]string{"owner": "ops"},
		MatcherSets: []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{
			Type: silencepb.Matcher_EQUAL, Name: "service", Pattern: "api",
		}}}},
		ReceiverMatcherSets: []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{
			Type: silencepb.Matcher_EQUAL, Name: "team", Pattern: "platform",
		}}}},
	}
	event := NewSilenceCreatedEvent(silence)

	silence.Annotations["owner"] = "changed"
	silence.MatcherSets[0].Matchers[0].Pattern = "changed"
	silence.ReceiverMatcherSets[0].Matchers[0].Pattern = "changed"

	got := event.message.GetSilenceCreated().Silence
	require.Equal(t, "ops", got.Annotations["owner"])
	require.Equal(t, "api", got.Matchers[0].Pattern)
	require.Equal(t, "platform", got.ReceiverMatcherSets[0].Matchers[0].Pattern)
}

func TestInhibitRuleSnapshot(t *testing.T) {
	source, err := labels.NewMatcher(labels.MatchEqual, "severity", "critical")
	require.NoError(t, err)
	target, err := labels.NewMatcher(labels.MatchEqual, "severity", "warning")
	require.NoError(t, err)
	equal := map[model.LabelName]struct{}{"cluster": {}, "alertname": {}}

	rule := NewInhibitRule("my-rule", labels.Matchers{source}, labels.Matchers{target}, equal)
	delete(equal, "cluster")

	require.Equal(t, "my-rule", rule.message.Name)
	require.Equal(t, []string{"alertname", "cluster"}, rule.message.EqualLabels)
	require.Equal(t, "severity", rule.message.SourceMatchers[0].Name)
}

func TestEventTypeName(t *testing.T) {
	require.Equal(t, "unknown", (EventData{}).typeName())
	require.Equal(t, "unknown", (Event{}).typeName())
	require.Equal(t, "alert_created", NewAlertCreatedEvent(nil).typeName())
}

func TestConstructorsHandleNilMatchers(t *testing.T) {
	require.NotPanics(t, func() {
		group := NewAlertGroup("", nil, "", "", labels.Matchers{nil}, "")
		require.Empty(t, group.message.Matchers)
		event := NewSilenceCreatedEvent(&silencepb.Silence{
			Matchers:            []*silencepb.Matcher{nil},
			MatcherSets:         []*silencepb.MatcherSet{nil, {Matchers: []*silencepb.Matcher{nil}}},
			ReceiverMatcherSets: []*silencepb.MatcherSet{nil, {Matchers: []*silencepb.Matcher{nil}}},
		})
		silence := event.message.GetSilenceCreated().Silence
		require.Empty(t, silence.Matchers)
		require.Len(t, silence.MatcherSets, 1)
		require.Empty(t, silence.MatcherSets[0].Matchers)
		require.Len(t, silence.ReceiverMatcherSets, 1)
		require.Empty(t, silence.ReceiverMatcherSets[0].Matchers)
		enveloped := event.withMetadata(nil, "", 0)
		_, err := enveloped.MarshalJSON()
		require.NoError(t, err)
		_, err = enveloped.MarshalProtobuf()
		require.NoError(t, err)
	})
}

func TestSilenceEventPreservesLegacyMatchers(t *testing.T) {
	event := NewSilenceCreatedEvent(&silencepb.Silence{
		Matchers: []*silencepb.Matcher{{Type: silencepb.Matcher_REGEXP, Name: "service", Pattern: "api.*"}},
		MatcherSets: []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{{
			Type: silencepb.Matcher_EQUAL, Name: "fallback", Pattern: "ignored",
		}}}},
	})

	got := event.message.GetSilenceCreated().Silence.Matchers
	require.Len(t, got, 1)
	require.Equal(t, "service", got[0].Name)
	require.Equal(t, "api.*", got[0].Pattern)
}

func TestSilenceMatcherUnknownTypeHasNoRenderedValue(t *testing.T) {
	matcher := silenceMatcherToEvents(&silencepb.Matcher{Type: silencepb.Matcher_Type(99), Name: "service", Pattern: "api"})
	require.Equal(t, events.Matcher_TYPE_UNSPECIFIED, matcher.Type)
	require.Empty(t, matcher.Rendered)
}

func TestNilMatchersAreAbsentFromJSON(t *testing.T) {
	event := NewAlertGroupedEvent(NewAlertGroup("", nil, "", "", labels.Matchers{nil}, ""), NewGroupedAlertReference(1)).withMetadata(nil, "", 0)
	data, err := event.MarshalJSON()
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	groupInfo := decoded["data"].(map[string]any)["alertGrouped"].(map[string]any)["groupInfo"].(map[string]any)
	require.NotContains(t, groupInfo, "matchers")
}

func TestEventConstructors(t *testing.T) {
	group := NewAlertGroup("", nil, "", "", nil, "")
	grouped := NewGroupedAlertReference(1)
	rule := NewInhibitRule("", nil, nil, nil)
	constructed := []EventData{
		NewAlertmanagerStartupEvent("", ""),
		NewAlertmanagerShutdownEvent(),
		NewAlertCreatedEvent(nil),
		NewAlertGroupedEvent(group, grouped),
		NewAlertResolvedEvent(group, grouped),
		NewNotificationEvent(Notification{Group: group}),
		NewSilenceMutedAlertEvent(nil, 0, nil),
		NewSilenceCreatedEvent(nil),
		NewSilenceUpdatedEvent(nil),
		NewInhibitionMutedAlertEvent([]InhibitRule{rule}, 0, nil, nil),
	}

	for _, event := range constructed {
		require.NotNil(t, event.message.EventType)
	}
}
