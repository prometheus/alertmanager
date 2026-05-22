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

// This file contains pure-functional helpers that convert internal
// Alertmanager types into eventrecorderpb messages, plus convenience
// constructors for the EventData oneof variants.  None of these
// functions touch the Recorder; they are imported and called by the
// dispatch, silence, inhibit, and provider packages to build event
// payloads that are then handed to Recorder.RecordEvent.

package eventrecorder

import (
	"slices"
	"strings"

	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/pkg/labels"
	silencepb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

// LabelSetAsProto converts a model.LabelSet to an eventrecorderpb.LabelSet.
// Labels are sorted by name for deterministic output.
func LabelSetAsProto(ls model.LabelSet) *eventrecorderpb.LabelSet {
	names := make([]model.LabelName, 0, len(ls))
	for k := range ls {
		names = append(names, k)
	}
	slices.SortFunc(names, func(a, b model.LabelName) int {
		return strings.Compare(string(a), string(b))
	})
	pairs := make([]*eventrecorderpb.LabelPair, 0, len(ls))
	for _, k := range names {
		pairs = append(pairs, &eventrecorderpb.LabelPair{Key: string(k), Value: string(ls[k])})
	}
	return &eventrecorderpb.LabelSet{Labels: pairs}
}

// AlertAsProto converts a types.Alert to an eventrecorderpb.Alert.
func AlertAsProto(alert *types.Alert) *eventrecorderpb.Alert {
	return &eventrecorderpb.Alert{
		Fingerprint: uint64(alert.Fingerprint()),
		Name:        alert.Name(),
		Labels:      LabelSetAsProto(alert.Labels),
		Annotations: LabelSetAsProto(alert.Annotations),
		StartsAt:    timestamppb.New(alert.StartsAt),
		EndsAt:      timestamppb.New(alert.EndsAt),
		Resolved:    alert.Resolved(),
	}
}

// MatcherAsProto converts a single *labels.Matcher to its protobuf
// representation.
func MatcherAsProto(m *labels.Matcher) *eventrecorderpb.Matcher {
	var matcherType eventrecorderpb.Matcher_Type
	switch m.Type {
	case labels.MatchEqual:
		matcherType = eventrecorderpb.Matcher_TYPE_EQUAL
	case labels.MatchNotEqual:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_EQUAL
	case labels.MatchRegexp:
		matcherType = eventrecorderpb.Matcher_TYPE_REGEXP
	case labels.MatchNotRegexp:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_REGEXP
	default:
		matcherType = eventrecorderpb.Matcher_TYPE_UNSPECIFIED
	}
	return &eventrecorderpb.Matcher{
		Type:     matcherType,
		Name:     m.Name,
		Pattern:  m.Value,
		Rendered: m.String(),
	}
}

// MatchersAsProto converts a slice of matchers to their protobuf
// representations.
func MatchersAsProto(matchers labels.Matchers) []*eventrecorderpb.Matcher {
	result := make([]*eventrecorderpb.Matcher, len(matchers))
	for i, m := range matchers {
		result[i] = MatcherAsProto(m)
	}
	return result
}

// SilenceMatcherAsProto converts a silencepb.Matcher to an
// eventrecorderpb.Matcher.
func SilenceMatcherAsProto(m *silencepb.Matcher) *eventrecorderpb.Matcher {
	var matcherType eventrecorderpb.Matcher_Type
	switch m.Type {
	case silencepb.Matcher_EQUAL:
		matcherType = eventrecorderpb.Matcher_TYPE_EQUAL
	case silencepb.Matcher_REGEXP:
		matcherType = eventrecorderpb.Matcher_TYPE_REGEXP
	case silencepb.Matcher_NOT_EQUAL:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_EQUAL
	case silencepb.Matcher_NOT_REGEXP:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_REGEXP
	default:
		matcherType = eventrecorderpb.Matcher_TYPE_UNSPECIFIED
	}

	var rendered string
	var matchType labels.MatchType
	switch m.Type {
	case silencepb.Matcher_EQUAL:
		matchType = labels.MatchEqual
	case silencepb.Matcher_NOT_EQUAL:
		matchType = labels.MatchNotEqual
	case silencepb.Matcher_REGEXP:
		matchType = labels.MatchRegexp
	case silencepb.Matcher_NOT_REGEXP:
		matchType = labels.MatchNotRegexp
	default:
		matchType = labels.MatchEqual
	}
	if lm, err := labels.NewMatcher(matchType, m.Name, m.Pattern); err == nil {
		rendered = lm.String()
	}

	return &eventrecorderpb.Matcher{
		Type:     matcherType,
		Name:     m.Name,
		Pattern:  m.Pattern,
		Rendered: rendered,
	}
}

// SilenceAsProto converts a silencepb.Silence to an
// eventrecorderpb.Silence.
func SilenceAsProto(sil *silencepb.Silence) *eventrecorderpb.Silence {
	matcherSets := make([]*eventrecorderpb.MatcherSet, len(sil.MatcherSets))
	for i, ms := range sil.MatcherSets {
		matcherSet := &eventrecorderpb.MatcherSet{
			Matchers: make([]*eventrecorderpb.Matcher, len(ms.Matchers)),
		}
		for j, m := range ms.Matchers {
			matcherSet.Matchers[j] = SilenceMatcherAsProto(m)
		}
		matcherSets[i] = matcherSet
	}

	var matchers []*eventrecorderpb.Matcher
	if len(matcherSets) > 0 {
		matchers = matcherSets[0].Matchers
	}

	return &eventrecorderpb.Silence{
		Id:          sil.Id,
		Matchers:    matchers,
		MatcherSets: matcherSets,
		StartsAt:    sil.StartsAt,
		EndsAt:      sil.EndsAt,
		UpdatedAt:   sil.UpdatedAt,
		CreatedBy:   sil.CreatedBy,
		Comment:     sil.Comment,
	}
}

// InhibitRuleAsProto converts inhibit rule fields to an
// eventrecorderpb.InhibitRule.  It accepts the individual fields rather
// than the InhibitRule struct to avoid an import cycle.
func InhibitRuleAsProto(sourceMatchers, targetMatchers labels.Matchers, equal map[model.LabelName]struct{}) *eventrecorderpb.InhibitRule {
	equalLabels := make([]string, 0, len(equal))
	for label := range equal {
		equalLabels = append(equalLabels, string(label))
	}
	slices.Sort(equalLabels)
	return &eventrecorderpb.InhibitRule{
		SourceMatchers: MatchersAsProto(sourceMatchers),
		TargetMatchers: MatchersAsProto(targetMatchers),
		EqualLabels:    equalLabels,
	}
}

// NewAlertCreatedEvent constructs an AlertCreated event.
func NewAlertCreatedEvent(alert *types.Alert) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertCreated{
			AlertCreated: &eventrecorderpb.AlertCreatedEvent{
				Alert: AlertAsProto(alert),
			},
		},
	}
}

// NewSilenceMutedAlertEvent constructs a SilenceMutedAlert event.
func NewSilenceMutedAlertEvent(silence *eventrecorderpb.Silence, fp model.Fingerprint, lset model.LabelSet) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceMutedAlert{
			SilenceMutedAlert: &eventrecorderpb.SilenceMutedAlertEvent{
				Silence: silence,
				MutedAlert: &eventrecorderpb.MutedAlert{
					Fingerprint: uint64(fp),
					Labels:      LabelSetAsProto(lset),
				},
			},
		},
	}
}

// NewSilenceCreatedEvent constructs a SilenceCreated event.
func NewSilenceCreatedEvent(silence *eventrecorderpb.Silence) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceCreated{
			SilenceCreated: &eventrecorderpb.SilenceCreatedEvent{
				Silence: silence,
			},
		},
	}
}

// NewSilenceUpdatedEvent constructs a SilenceUpdated event.
func NewSilenceUpdatedEvent(silence *eventrecorderpb.Silence) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceUpdated{
			SilenceUpdated: &eventrecorderpb.SilenceUpdatedEvent{
				Silence: silence,
			},
		},
	}
}

// NewInhibitionMutedAlertEvent constructs an InhibitionMutedAlert event.
func NewInhibitionMutedAlertEvent(rules []*eventrecorderpb.InhibitRule, fp model.Fingerprint, lset model.LabelSet, inhibitingFPs []model.Fingerprint) *eventrecorderpb.EventData {
	fps := make([]uint64, len(inhibitingFPs))
	for i, f := range inhibitingFPs {
		fps[i] = uint64(f)
	}
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_InhibitionMutedAlert{
			InhibitionMutedAlert: &eventrecorderpb.InhibitionMutedAlertEvent{
				InhibitRules: rules,
				MutedAlert: &eventrecorderpb.MutedAlert{
					Fingerprint: uint64(fp),
					Labels:      LabelSetAsProto(lset),
				},
				InhibitingFingerprints: fps,
			},
		},
	}
}

// extractEventType returns the proto oneof field name for the event
// type (e.g. "alert_created", "notification").  It uses a type switch
// on the generated oneof wrapper types, avoiding proto reflection.
func extractEventType(event *eventrecorderpb.EventData) string {
	switch event.EventType.(type) {
	case *eventrecorderpb.EventData_AlertmanagerStartupEvent:
		return "alertmanager_startup_event"
	case *eventrecorderpb.EventData_AlertmanagerShutdownEvent:
		return "alertmanager_shutdown_event"
	case *eventrecorderpb.EventData_AlertCreated:
		return "alert_created"
	case *eventrecorderpb.EventData_AlertResolved:
		return "alert_resolved"
	case *eventrecorderpb.EventData_AlertGrouped:
		return "alert_grouped"
	case *eventrecorderpb.EventData_Notification:
		return "notification"
	case *eventrecorderpb.EventData_SilenceCreated:
		return "silence_created"
	case *eventrecorderpb.EventData_SilenceUpdated:
		return "silence_updated"
	case *eventrecorderpb.EventData_SilenceMutedAlert:
		return "silence_muted_alert"
	case *eventrecorderpb.EventData_InhibitionMutedAlert:
		return "inhibition_muted_alert"
	default:
		return "unknown"
	}
}
