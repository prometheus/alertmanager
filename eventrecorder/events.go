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
	"maps"
	"slices"
	"time"

	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/alert"
	events "github.com/prometheus/alertmanager/eventrecorder/events/v2"
	"github.com/prometheus/alertmanager/pkg/labels"
	silencepb "github.com/prometheus/alertmanager/silence/silencepb"
)

// EventData is an immutable event recorder payload without envelope metadata.
// Its protobuf representation is private so producers cannot construct invalid
// output events.
type EventData struct {
	message   *events.EventData
	eventType string
}

// Event is an immutable event recorder output with envelope metadata. Its
// protobuf representation is private so destinations cannot be passed arbitrary
// protobuf messages.
type Event struct {
	message   *events.Event
	eventType string
}

// MarshalJSON serializes the event using its protobuf schema.
func (e Event) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(e.message)
}

// MarshalProtobuf serializes the event using its protobuf schema.
func (e Event) MarshalProtobuf() ([]byte, error) {
	return proto.Marshal(e.message)
}

func (d EventData) typeName() string {
	return eventTypeName(d.eventType)
}

func (e Event) typeName() string {
	return eventTypeName(e.eventType)
}

func eventTypeName(eventType string) string {
	if eventType == "" {
		return "unknown"
	}
	return eventType
}

func (e Event) protoMessage() proto.Message {
	return e.message
}

func (d EventData) withMetadata(timestamp *timestamppb.Timestamp, instance string, clusterPosition uint64) Event {
	return Event{
		message: &events.Event{
			Timestamp: timestamp, Instance: instance, Data: d.message, ClusterPosition: clusterPosition,
		},
		eventType: d.eventType,
	}
}

// AlertGroup is an immutable aggregation-group snapshot.
type AlertGroup struct {
	message *events.AlertGroupInfo
}

// GroupedAlert is an immutable grouped-alert snapshot.
type GroupedAlert struct {
	message *events.GroupedAlert
}

// InhibitRule is an immutable inhibition-rule snapshot.
type InhibitRule struct {
	message *events.InhibitRule
}

// NotificationReason describes why a notification was sent.
type NotificationReason int

const (
	NotificationReasonUnspecified NotificationReason = iota
	NotificationReasonFirstNotification
	NotificationReasonNewAlertsInGroup
	NotificationReasonNewResolvedAlerts
	NotificationReasonAllAlertsResolved
	NotificationReasonRepeatIntervalElapsed
)

// Notification contains the snapshots used to construct a notification event.
type Notification struct {
	Alerts         []GroupedAlert
	FiringAlerts   []GroupedAlert
	ResolvedAlerts []GroupedAlert
	MutedAlerts    []GroupedAlert
	Group          AlertGroup
	RepeatInterval time.Duration
	Reason         NotificationReason
	FlushID        uint64
	Integration    string
	IntegrationIdx int64
}

// NewAlertGroup snapshots aggregation-group metadata.
func NewAlertGroup(groupKey string, groupLabels model.LabelSet, groupID, receiverName string, matchers labels.Matchers, groupUUID string) AlertGroup {
	return AlertGroup{message: &events.AlertGroupInfo{
		GroupKey: groupKey, GroupLabels: labelSetMap(groupLabels), GroupId: groupID,
		ReceiverName: receiverName, Matchers: matchersToEvents(matchers), GroupUuid: groupUUID,
	}}
}

// NewGroupedAlert snapshots an alert and its notification-pipeline hash.
func NewGroupedAlert(hash uint64, a *alert.Alert) GroupedAlert {
	return GroupedAlert{message: &events.GroupedAlert{Hash: hash, Details: alertToEvents(a)}}
}

// NewGroupedAlertReference snapshots a hash-only grouped-alert reference.
func NewGroupedAlertReference(hash uint64) GroupedAlert {
	return GroupedAlert{message: &events.GroupedAlert{Hash: hash}}
}

// NewAlertmanagerStartupEvent constructs startup event data.
func NewAlertmanagerStartupEvent(version, buildContext string) EventData {
	return newEventData("alertmanager_startup_event", &events.EventData{EventType: &events.EventData_AlertmanagerStartupEvent{
		AlertmanagerStartupEvent: &events.AlertmanagerStartupEvent{Version: version, BuildContext: buildContext},
	}})
}

// NewAlertmanagerShutdownEvent constructs shutdown event data.
func NewAlertmanagerShutdownEvent() EventData {
	return newEventData("alertmanager_shutdown_event", &events.EventData{EventType: &events.EventData_AlertmanagerShutdownEvent{
		AlertmanagerShutdownEvent: &events.AlertmanagerShutdownEvent{},
	}})
}

// NewAlertCreatedEvent constructs alert-created event data.
func NewAlertCreatedEvent(a *alert.Alert) EventData {
	return newEventData("alert_created", &events.EventData{EventType: &events.EventData_AlertCreated{
		AlertCreated: &events.AlertCreatedEvent{Alert: alertToEvents(a)},
	}})
}

// NewAlertGroupedEvent constructs alert-grouped event data.
func NewAlertGroupedEvent(group AlertGroup, groupedAlert GroupedAlert) EventData {
	return newEventData("alert_grouped", &events.EventData{EventType: &events.EventData_AlertGrouped{
		AlertGrouped: &events.AlertGroupedEvent{Alert: groupedAlert.message, GroupInfo: group.message},
	}})
}

// NewAlertResolvedEvent constructs alert-resolved event data.
func NewAlertResolvedEvent(group AlertGroup, groupedAlert GroupedAlert) EventData {
	return newEventData("alert_resolved", &events.EventData{EventType: &events.EventData_AlertResolved{
		AlertResolved: &events.AlertResolvedEvent{Alert: groupedAlert.message, GroupInfo: group.message},
	}})
}

// NewNotificationEvent constructs notification event data.
func NewNotificationEvent(notification Notification) EventData {
	return newEventData("notification", &events.EventData{EventType: &events.EventData_Notification{
		Notification: &events.NotificationEvent{
			Alerts: groupedAlertsToEvents(notification.Alerts), FiringAlerts: groupedAlertsToEvents(notification.FiringAlerts),
			ResolvedAlerts: groupedAlertsToEvents(notification.ResolvedAlerts), MutedAlerts: groupedAlertsToEvents(notification.MutedAlerts),
			GroupInfo: notification.Group.message, RepeatInterval: durationpb.New(notification.RepeatInterval),
			Reason: notificationReasonToEvents(notification.Reason), FlushId: notification.FlushID,
			Integration: &events.Integration{Name: notification.Integration, Index: notification.IntegrationIdx},
		},
	}})
}

// NewSilenceMutedAlertEvent constructs silence-muted-alert event data.
func NewSilenceMutedAlertEvent(silence *silencepb.Silence, fp model.Fingerprint, labelSet model.LabelSet) EventData {
	return newEventData("silence_muted_alert", &events.EventData{EventType: &events.EventData_SilenceMutedAlert{
		SilenceMutedAlert: &events.SilenceMutedAlertEvent{Silence: silenceToEvents(silence), MutedAlert: &events.MutedAlert{
			Fingerprint: uint64(fp), Labels: labelSetMap(labelSet),
		}},
	}})
}

// NewSilenceCreatedEvent constructs silence-created event data.
func NewSilenceCreatedEvent(silence *silencepb.Silence) EventData {
	return newEventData("silence_created", &events.EventData{EventType: &events.EventData_SilenceCreated{
		SilenceCreated: &events.SilenceCreatedEvent{Silence: silenceToEvents(silence)},
	}})
}

// NewSilenceUpdatedEvent constructs silence-updated event data.
func NewSilenceUpdatedEvent(silence *silencepb.Silence) EventData {
	return newEventData("silence_updated", &events.EventData{EventType: &events.EventData_SilenceUpdated{
		SilenceUpdated: &events.SilenceUpdatedEvent{Silence: silenceToEvents(silence)},
	}})
}

// NewInhibitRule snapshots an inhibition rule.
func NewInhibitRule(name string, sourceMatchers, targetMatchers labels.Matchers, equal map[model.LabelName]struct{}) InhibitRule {
	equalLabels := make([]string, 0, len(equal))
	for label := range equal {
		equalLabels = append(equalLabels, string(label))
	}
	slices.Sort(equalLabels)
	return InhibitRule{message: &events.InhibitRule{
		Name: name, SourceMatchers: matchersToEvents(sourceMatchers), TargetMatchers: matchersToEvents(targetMatchers), EqualLabels: equalLabels,
	}}
}

// NewInhibitionMutedAlertEvent constructs inhibition-muted-alert event data.
func NewInhibitionMutedAlertEvent(rules []InhibitRule, fp model.Fingerprint, labelSet model.LabelSet, inhibitingFPs []model.Fingerprint) EventData {
	fps := make([]uint64, len(inhibitingFPs))
	for i, fingerprint := range inhibitingFPs {
		fps[i] = uint64(fingerprint)
	}
	eventRules := make([]*events.InhibitRule, len(rules))
	for i, rule := range rules {
		eventRules[i] = rule.message
	}
	return newEventData("inhibition_muted_alert", &events.EventData{EventType: &events.EventData_InhibitionMutedAlert{
		InhibitionMutedAlert: &events.InhibitionMutedAlertEvent{
			InhibitRules: eventRules, MutedAlert: &events.MutedAlert{Fingerprint: uint64(fp), Labels: labelSetMap(labelSet)},
			InhibitingFingerprints: fps,
		},
	}})
}

func newEventData(eventType string, message *events.EventData) EventData {
	return EventData{message: message, eventType: eventType}
}

func labelSetMap(labelSet model.LabelSet) map[string]string {
	result := make(map[string]string, len(labelSet))
	for name, value := range labelSet {
		result[string(name)] = string(value)
	}
	return result
}

func stringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	maps.Copy(result, values)
	return result
}

func alertToEvents(a *alert.Alert) *events.Alert {
	if a == nil {
		return nil
	}
	return &events.Alert{
		Fingerprint: uint64(a.Fingerprint()), Name: a.Name(), Labels: labelSetMap(a.Labels), Annotations: labelSetMap(a.Annotations),
		StartsAt: timestamppb.New(a.StartsAt), EndsAt: timestamppb.New(a.EndsAt), Resolved: a.Resolved(),
	}
}

func matchersToEvents(matchers labels.Matchers) []*events.Matcher {
	result := make([]*events.Matcher, 0, len(matchers))
	for _, matcher := range matchers {
		if matcher == nil {
			continue
		}
		result = append(result, &events.Matcher{Type: matcherTypeToEvents(matcher.Type), Name: matcher.Name, Pattern: matcher.Value, Rendered: matcher.String()})
	}
	return result
}

func matcherTypeToEvents(matcherType labels.MatchType) events.Matcher_Type {
	switch matcherType {
	case labels.MatchEqual:
		return events.Matcher_TYPE_EQUAL
	case labels.MatchNotEqual:
		return events.Matcher_TYPE_NOT_EQUAL
	case labels.MatchRegexp:
		return events.Matcher_TYPE_REGEXP
	case labels.MatchNotRegexp:
		return events.Matcher_TYPE_NOT_REGEXP
	default:
		return events.Matcher_TYPE_UNSPECIFIED
	}
}

func silenceToEvents(silence *silencepb.Silence) *events.Silence {
	if silence == nil {
		return nil
	}
	matcherSets := silenceMatcherSetsToEvents(silence.MatcherSets)
	receiverMatcherSets := silenceMatcherSetsToEvents(silence.ReceiverMatcherSets)
	matchers := silenceMatchersToEvents(silence.Matchers)
	if len(matchers) == 0 && len(matcherSets) > 0 {
		matchers = matcherSets[0].Matchers
	}
	return &events.Silence{
		Id: silence.Id, Matchers: matchers, Annotations: stringMap(silence.Annotations), StartsAt: cloneTimestamp(silence.StartsAt),
		EndsAt: cloneTimestamp(silence.EndsAt), UpdatedAt: cloneTimestamp(silence.UpdatedAt), CreatedBy: silence.CreatedBy,
		Comment: silence.Comment, MatcherSets: matcherSets, ReceiverMatcherSets: receiverMatcherSets,
	}
}

func silenceMatcherSetsToEvents(sets []*silencepb.MatcherSet) []*events.MatcherSet {
	result := make([]*events.MatcherSet, 0, len(sets))
	for _, set := range sets {
		if set == nil {
			continue
		}
		result = append(result, &events.MatcherSet{Matchers: silenceMatchersToEvents(set.Matchers)})
	}
	return result
}

func silenceMatchersToEvents(matchers []*silencepb.Matcher) []*events.Matcher {
	result := make([]*events.Matcher, 0, len(matchers))
	for _, matcher := range matchers {
		if matcher == nil {
			continue
		}
		result = append(result, silenceMatcherToEvents(matcher))
	}
	return result
}

func silenceMatcherToEvents(matcher *silencepb.Matcher) *events.Matcher {
	if matcher == nil {
		return nil
	}
	eventType := events.Matcher_TYPE_UNSPECIFIED
	switch matcher.Type {
	case silencepb.Matcher_EQUAL:
		eventType = events.Matcher_TYPE_EQUAL
	case silencepb.Matcher_REGEXP:
		eventType = events.Matcher_TYPE_REGEXP
	case silencepb.Matcher_NOT_EQUAL:
		eventType = events.Matcher_TYPE_NOT_EQUAL
	case silencepb.Matcher_NOT_REGEXP:
		eventType = events.Matcher_TYPE_NOT_REGEXP
	}
	return &events.Matcher{Type: eventType, Name: matcher.Name, Pattern: matcher.Pattern, Rendered: silenceMatcherRendered(matcher)}
}

func silenceMatcherRendered(matcher *silencepb.Matcher) string {
	var matcherType labels.MatchType
	switch matcher.Type {
	case silencepb.Matcher_EQUAL:
		matcherType = labels.MatchEqual
	case silencepb.Matcher_REGEXP:
		matcherType = labels.MatchRegexp
	case silencepb.Matcher_NOT_EQUAL:
		matcherType = labels.MatchNotEqual
	case silencepb.Matcher_NOT_REGEXP:
		matcherType = labels.MatchNotRegexp
	default:
		return ""
	}
	rendered := ""
	if parsed, err := labels.NewMatcher(matcherType, matcher.Name, matcher.Pattern); err == nil {
		rendered = parsed.String()
	}
	return rendered
}

func cloneTimestamp(timestamp *timestamppb.Timestamp) *timestamppb.Timestamp {
	if timestamp == nil {
		return nil
	}
	return timestamppb.New(timestamp.AsTime())
}

func groupedAlertsToEvents(alerts []GroupedAlert) []*events.GroupedAlert {
	result := make([]*events.GroupedAlert, len(alerts))
	for i, groupedAlert := range alerts {
		result[i] = groupedAlert.message
	}
	return result
}

func notificationReasonToEvents(reason NotificationReason) events.NotifyReason {
	switch reason {
	case NotificationReasonFirstNotification:
		return events.NotifyReason_NOTIFY_REASON_FIRST_NOTIFICATION
	case NotificationReasonNewAlertsInGroup:
		return events.NotifyReason_NOTIFY_REASON_NEW_ALERTS_IN_GROUP
	case NotificationReasonNewResolvedAlerts:
		return events.NotifyReason_NOTIFY_REASON_NEW_RESOLVED_ALERTS
	case NotificationReasonAllAlertsResolved:
		return events.NotifyReason_NOTIFY_REASON_ALL_ALERTS_RESOLVED
	case NotificationReasonRepeatIntervalElapsed:
		return events.NotifyReason_NOTIFY_REASON_REPEAT_INTERVAL_ELAPSED
	default:
		return events.NotifyReason_NOTIFY_REASON_UNSPECIFIED
	}
}
