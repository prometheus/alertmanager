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

package notify

// This file contains helpers for constructing event recorder protobuf messages
// from the notification pipeline context.  It lives in the notify package
// because it accesses unexported context keys (keyFiringAlerts, etc.).

import (
	"context"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/types"
)

func groupedAlertAsProto(alert *types.Alert) *eventrecorderpb.GroupedAlert {
	return &eventrecorderpb.GroupedAlert{
		Hash:    hashAlert(alert),
		Details: eventrecorder.AlertAsProto(alert),
	}
}

func extractAlertGroupInfo(ctx context.Context) *eventrecorderpb.AlertGroupInfo {
	groupKey, _ := ExtractGroupKey(ctx)
	receiverName, _ := ReceiverName(ctx)
	groupLabels, _ := GroupLabels(ctx)
	groupMatchers, _ := GroupMatchers(ctx)
	aggrGroupID, _ := AggrGroupID(ctx)

	return &eventrecorderpb.AlertGroupInfo{
		GroupKey:     groupKey.String(),
		GroupLabels:  eventrecorder.LabelSetAsProto(groupLabels),
		GroupId:      groupKey.Hash(),
		ReceiverName: receiverName,
		Matchers:     eventrecorder.MatchersAsProto(groupMatchers),
		GroupUuid:    aggrGroupID,
	}
}

func extractGroupedAlerts(ctx context.Context, key notifyKey) []*eventrecorderpb.GroupedAlert {
	var result []*eventrecorderpb.GroupedAlert
	if list, ok := ctx.Value(key).([]uint64); ok {
		for _, hash := range list {
			result = append(result, &eventrecorderpb.GroupedAlert{Hash: hash})
		}
	}
	return result
}

func extractMutedGroupedAlerts(ctx context.Context) []*eventrecorderpb.GroupedAlert {
	var result []*eventrecorderpb.GroupedAlert
	if muted, ok := MutedAlerts(ctx); ok {
		for hash := range muted {
			result = append(result, &eventrecorderpb.GroupedAlert{Hash: hash})
		}
	}
	return result
}

func notifyReasonToProto(reason NotifyReason) eventrecorderpb.NotifyReason {
	switch reason {
	case ReasonFirstNotification:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_FIRST_NOTIFICATION
	case ReasonNewAlertsInGroup:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_NEW_ALERTS_IN_GROUP
	case ReasonAllAlertsResolved:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_ALL_ALERTS_RESOLVED
	case ReasonNewResolvedAlerts:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_NEW_RESOLVED_ALERTS
	case ReasonRepeatIntervalElapsed:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_REPEAT_INTERVAL_ELAPSED
	default:
		return eventrecorderpb.NotifyReason_NOTIFY_REASON_UNSPECIFIED
	}
}

// NewNotificationEvent constructs a NotificationEvent from the pipeline
// context after a successful notification delivery.
func NewNotificationEvent(ctx context.Context, alerts []*types.Alert, integration Integration) *eventrecorderpb.EventData {
	groupedAlerts := make([]*eventrecorderpb.GroupedAlert, 0, len(alerts))
	for _, alert := range alerts {
		groupedAlerts = append(groupedAlerts, groupedAlertAsProto(alert))
	}

	notifyReason, _ := NotificationReason(ctx)
	repeatInterval, _ := RepeatInterval(ctx)
	flushID, _ := FlushID(ctx)

	notification := &eventrecorderpb.NotificationEvent{
		Alerts:         groupedAlerts,
		FiringAlerts:   extractGroupedAlerts(ctx, keyFiringAlerts),
		ResolvedAlerts: extractGroupedAlerts(ctx, keyResolvedAlerts),
		MutedAlerts:    extractMutedGroupedAlerts(ctx),
		GroupInfo:      extractAlertGroupInfo(ctx),
		RepeatInterval: durationpb.New(repeatInterval),
		Reason:         notifyReasonToProto(notifyReason),
		FlushId:        flushID,
		Integration: &eventrecorderpb.Integration{
			Name:  integration.Name(),
			Index: int64(integration.Index()),
		},
	}

	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_Notification{Notification: notification},
	}
}

func NewAlertResolvedEvent(groupInfo *eventrecorderpb.AlertGroupInfo, alert *types.Alert) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertResolved{
			AlertResolved: &eventrecorderpb.AlertResolvedEvent{
				Alert:     groupedAlertAsProto(alert),
				GroupInfo: groupInfo,
			},
		},
	}
}

func NewAlertGroupedEvent(groupInfo *eventrecorderpb.AlertGroupInfo, alert *types.Alert) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertGrouped{
			AlertGrouped: &eventrecorderpb.AlertGroupedEvent{
				Alert:     groupedAlertAsProto(alert),
				GroupInfo: groupInfo,
			},
		},
	}
}
