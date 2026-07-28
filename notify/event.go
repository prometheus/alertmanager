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

// This file contains helpers for constructing event recorder events
// from the notification pipeline context.  It lives in the notify package
// because it accesses unexported context keys (keyFiringAlerts, etc.).

import (
	"context"

	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/types"
)

func groupedAlertEvent(alert *types.Alert) eventrecorder.GroupedAlert {
	return eventrecorder.NewGroupedAlert(hashAlert(alert), alert)
}

func extractAlertGroupInfo(ctx context.Context) eventrecorder.AlertGroup {
	groupKey, _ := ExtractGroupKey(ctx)
	receiverName, _ := ReceiverName(ctx)
	groupLabels, _ := GroupLabels(ctx)
	groupMatchers, _ := GroupMatchers(ctx)
	aggrGroupID, _ := AggrGroupID(ctx)

	return eventrecorder.NewAlertGroup(
		groupKey.String(), groupLabels, groupKey.Hash(), receiverName, groupMatchers, aggrGroupID,
	)
}

func extractGroupedAlerts(ctx context.Context, key notifyKey) []eventrecorder.GroupedAlert {
	var result []eventrecorder.GroupedAlert
	if list, ok := ctx.Value(key).([]uint64); ok {
		for _, hash := range list {
			result = append(result, eventrecorder.NewGroupedAlertReference(hash))
		}
	}
	return result
}

func extractMutedGroupedAlerts(ctx context.Context) []eventrecorder.GroupedAlert {
	var result []eventrecorder.GroupedAlert
	if muted, ok := MutedAlerts(ctx); ok {
		for hash := range muted {
			result = append(result, eventrecorder.NewGroupedAlertReference(hash))
		}
	}
	return result
}

func notifyReasonToEvent(reason NotifyReason) eventrecorder.NotificationReason {
	switch reason {
	case ReasonFirstNotification:
		return eventrecorder.NotificationReasonFirstNotification
	case ReasonNewAlertsInGroup:
		return eventrecorder.NotificationReasonNewAlertsInGroup
	case ReasonAllAlertsResolved:
		return eventrecorder.NotificationReasonAllAlertsResolved
	case ReasonNewResolvedAlerts:
		return eventrecorder.NotificationReasonNewResolvedAlerts
	case ReasonRepeatIntervalElapsed:
		return eventrecorder.NotificationReasonRepeatIntervalElapsed
	default:
		return eventrecorder.NotificationReasonUnspecified
	}
}

// NewNotificationEvent constructs notification event data from the pipeline
// context after a successful notification delivery.
func NewNotificationEvent(ctx context.Context, alerts []*types.Alert, integration Integration) eventrecorder.EventData {
	groupedAlerts := make([]eventrecorder.GroupedAlert, 0, len(alerts))
	for _, alert := range alerts {
		groupedAlerts = append(groupedAlerts, groupedAlertEvent(alert))
	}

	notifyReason, _ := NotificationReason(ctx)
	repeatInterval, _ := RepeatInterval(ctx)
	flushID, _ := FlushID(ctx)

	return eventrecorder.NewNotificationEvent(eventrecorder.Notification{
		Alerts:         groupedAlerts,
		FiringAlerts:   extractGroupedAlerts(ctx, keyFiringAlerts),
		ResolvedAlerts: extractGroupedAlerts(ctx, keyResolvedAlerts),
		MutedAlerts:    extractMutedGroupedAlerts(ctx),
		Group:          extractAlertGroupInfo(ctx),
		RepeatInterval: repeatInterval,
		Reason:         notifyReasonToEvent(notifyReason),
		FlushID:        flushID,
		Integration:    integration.Name(),
		IntegrationIdx: int64(integration.Index()),
	})
}

// NewAlertResolvedEvent constructs alert-resolved event data.
func NewAlertResolvedEvent(groupInfo eventrecorder.AlertGroup, alert *types.Alert) eventrecorder.EventData {
	return eventrecorder.NewAlertResolvedEvent(groupInfo, groupedAlertEvent(alert))
}

// NewAlertGroupedEvent constructs alert-grouped event data.
func NewAlertGroupedEvent(groupInfo eventrecorder.AlertGroup, alert *types.Alert) eventrecorder.EventData {
	return eventrecorder.NewAlertGroupedEvent(groupInfo, groupedAlertEvent(alert))
}
