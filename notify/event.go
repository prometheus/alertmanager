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

// This file contains helpers for constructing event log protobuf messages
// from the notification pipeline context.  It lives in the notify package
// because it accesses unexported context keys (keyFiringAlerts, etc.).

import (
	"context"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/prometheus/alertmanager/eventlog"
	"github.com/prometheus/alertmanager/eventlog/eventlogpb"
	"github.com/prometheus/alertmanager/types"
)

func groupedAlertAsProto(alert *types.Alert) *eventlogpb.GroupedAlert {
	return &eventlogpb.GroupedAlert{
		Hash:    hashAlert(alert),
		Details: eventlog.AlertAsProto(alert),
	}
}

func extractAlertGroupInfo(ctx context.Context) *eventlogpb.AlertGroupInfo {
	groupKey, _ := ExtractGroupKey(ctx)
	receiverName, _ := ReceiverName(ctx)
	groupLabels, _ := GroupLabels(ctx)
	groupMatchers, _ := GroupMatchers(ctx)
	routeLabels, _ := RouteLabels(ctx)
	aggrGroupID, _ := AggrGroupID(ctx)

	return &eventlogpb.AlertGroupInfo{
		GroupKey:     groupKey.String(),
		GroupLabels:  eventlog.LabelSetAsProto(groupLabels),
		GroupId:      groupKey.Hash(),
		ReceiverName: receiverName,
		Matchers:     eventlog.MatchersAsProto(groupMatchers),
		RouteLabels:  eventlog.LabelSetAsProto(routeLabels),
		GroupUuid:    aggrGroupID,
	}
}

func extractGroupedAlerts(ctx context.Context, key notifyKey) []*eventlogpb.GroupedAlert {
	var result []*eventlogpb.GroupedAlert
	if list, ok := ctx.Value(key).([]uint64); ok {
		for _, hash := range list {
			result = append(result, &eventlogpb.GroupedAlert{Hash: hash})
		}
	}
	return result
}

func extractMutedGroupedAlerts(ctx context.Context) []*eventlogpb.GroupedAlert {
	var result []*eventlogpb.GroupedAlert
	if muted, ok := MutedAlerts(ctx); ok {
		for hash := range muted {
			result = append(result, &eventlogpb.GroupedAlert{Hash: hash})
		}
	}
	return result
}

func notifyReasonToProto(reason NotifyReason) eventlogpb.NotifyReason {
	switch reason {
	case ReasonFirstNotification:
		return eventlogpb.NotifyReason_NOTIFY_REASON_FIRST_NOTIFICATION
	case ReasonNewAlertsInGroup:
		return eventlogpb.NotifyReason_NOTIFY_REASON_NEW_ALERTS_IN_GROUP
	case ReasonAllAlertsResolved:
		return eventlogpb.NotifyReason_NOTIFY_REASON_ALL_ALERTS_RESOLVED
	case ReasonNewResolvedAlerts:
		return eventlogpb.NotifyReason_NOTIFY_REASON_NEW_RESOLVED_ALERTS
	case ReasonRepeatIntervalElapsed:
		return eventlogpb.NotifyReason_NOTIFY_REASON_REPEAT_INTERVAL_ELAPSED
	default:
		return eventlogpb.NotifyReason_NOTIFY_REASON_UNSPECIFIED
	}
}

// NewNotificationEvent constructs a NotificationEvent from the pipeline
// context after a successful notification delivery.
func NewNotificationEvent(ctx context.Context, alerts []*types.Alert, integration Integration) *eventlogpb.EventData {
	groupedAlerts := make([]*eventlogpb.GroupedAlert, 0, len(alerts))
	for _, alert := range alerts {
		groupedAlerts = append(groupedAlerts, groupedAlertAsProto(alert))
	}

	notifyReason, _ := NotificationReason(ctx)
	repeatInterval, _ := RepeatInterval(ctx)
	flushID, _ := FlushID(ctx)

	notification := &eventlogpb.NotificationEvent{
		Alerts:         groupedAlerts,
		FiringAlerts:   extractGroupedAlerts(ctx, keyFiringAlerts),
		ResolvedAlerts: extractGroupedAlerts(ctx, keyResolvedAlerts),
		MutedAlerts:    extractMutedGroupedAlerts(ctx),
		GroupInfo:      extractAlertGroupInfo(ctx),
		RepeatInterval: durationpb.New(repeatInterval),
		Reason:         notifyReasonToProto(notifyReason),
		FlushId:        flushID,
		Integration: &eventlogpb.Integration{
			Name:  integration.Name(),
			Index: int64(integration.Index()),
		},
	}

	return &eventlogpb.EventData{
		EventType: &eventlogpb.EventData_Notification{Notification: notification},
	}
}

func NewAlertResolvedEvent(groupInfo *eventlogpb.AlertGroupInfo, alert *types.Alert) *eventlogpb.EventData {
	return &eventlogpb.EventData{
		EventType: &eventlogpb.EventData_AlertResolved{
			AlertResolved: &eventlogpb.AlertResolvedEvent{
				Alert:     groupedAlertAsProto(alert),
				GroupInfo: groupInfo,
			},
		},
	}
}

func NewAlertGroupedEvent(groupInfo *eventlogpb.AlertGroupInfo, alert *types.Alert) *eventlogpb.EventData {
	return &eventlogpb.EventData{
		EventType: &eventlogpb.EventData_AlertGrouped{
			AlertGrouped: &eventlogpb.AlertGroupedEvent{
				Alert:     groupedAlertAsProto(alert),
				GroupInfo: groupInfo,
			},
		},
	}
}
