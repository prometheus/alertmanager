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
// from notification pipeline state that is internal to the notify package.

import (
	"context"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
)

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

// mutedAlertDetails returns the alerts a mute stage removed from the pipeline,
// in hash order so that an unchanged muted set always produces the same event.
func mutedAlertDetails(ctx context.Context) []*alert.Alert {
	hashes, muted := sortedMutedAlerts(ctx)
	if len(hashes) == 0 {
		return nil
	}

	result := make([]*alert.Alert, 0, len(hashes))
	for _, hash := range hashes {
		result = append(result, muted[hash])
	}
	return result
}

func alertDetailsByHash(alerts []*alert.Alert) map[uint64]*alert.Alert {
	result := make(map[uint64]*alert.Alert, len(alerts))
	for _, alrt := range alerts {
		result[hashAlert(alrt)] = alrt
	}
	return result
}

func alertDetailsForHashes(alerts map[uint64]*alert.Alert, hashes []uint64) []*alert.Alert {
	result := make([]*alert.Alert, 0, len(hashes))
	for _, hash := range hashes {
		if alrt, ok := alerts[hash]; ok {
			result = append(result, alrt)
		}
	}
	return result
}

func groupedAlertsWithDetails(alerts []*alert.Alert) []eventrecorder.GroupedAlert {
	result := make([]eventrecorder.GroupedAlert, 0, len(alerts))
	for _, alrt := range alerts {
		result = append(result, eventrecorder.NewGroupedAlert(alrt))
	}
	return result
}

func groupedAlertReferences(alerts []*alert.Alert) []eventrecorder.GroupedAlert {
	result := make([]eventrecorder.GroupedAlert, 0, len(alerts))
	for _, alrt := range alerts {
		result = append(result, eventrecorder.NewGroupedAlertReference(alrt.Fingerprint()))
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

func newNotificationEvent(ctx context.Context, alerts []*alert.Alert, integration Integration) eventrecorder.EventData {
	notifyReason, _ := NotificationReason(ctx)
	repeatInterval, _ := RepeatInterval(ctx)
	flushID, _ := FlushID(ctx)
	firingHashes, _ := FiringAlerts(ctx)
	resolvedHashes, _ := ResolvedAlerts(ctx)
	// The two halves of the group: the alerts the receiver was shown, and the
	// alerts a mute stage removed from the pipeline.
	visibleDetails := alertDetailsByHash(alerts)
	mutedDetails := mutedAlertDetails(ctx)
	allAlerts := make([]*alert.Alert, 0, len(alerts)+len(mutedDetails))
	allAlerts = append(allAlerts, alerts...)
	allAlerts = append(allAlerts, mutedDetails...)

	return eventrecorder.NewNotificationEvent(eventrecorder.Notification{
		Alerts:         groupedAlertsWithDetails(allAlerts),
		FiringAlerts:   groupedAlertReferences(alertDetailsForHashes(visibleDetails, firingHashes)),
		ResolvedAlerts: groupedAlertReferences(alertDetailsForHashes(visibleDetails, resolvedHashes)),
		MutedAlerts:    groupedAlertReferences(mutedDetails),
		Group:          extractAlertGroupInfo(ctx),
		RepeatInterval: repeatInterval,
		Reason:         notifyReasonToEvent(notifyReason),
		FlushID:        flushID,
		Integration:    integration.Name(),
		IntegrationIdx: int64(integration.Index()),
	})
}

// NewAlertResolvedEvent constructs alert-resolved event data.
func NewAlertResolvedEvent(groupInfo eventrecorder.AlertGroup, alrt *alert.Alert) eventrecorder.EventData {
	return eventrecorder.NewAlertResolvedEvent(groupInfo, eventrecorder.NewGroupedAlert(alrt))
}

// NewAlertGroupedEvent constructs alert-grouped event data.
func NewAlertGroupedEvent(groupInfo eventrecorder.AlertGroup, alrt *alert.Alert) eventrecorder.EventData {
	return eventrecorder.NewAlertGroupedEvent(groupInfo, eventrecorder.NewGroupedAlert(alrt))
}
