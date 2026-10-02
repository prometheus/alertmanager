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

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
)

// TestNotifyReasonToEvent pins the mapping, including the two reasons the muted
// alerts feature adds. They have no event enum value of their own yet, so they
// fall through to unspecified; this fails once the eventrecorder proto gains
// them and this switch is not updated with it.
func TestNotifyReasonToEvent(t *testing.T) {
	tests := []struct {
		reason NotifyReason
		want   eventrecorder.NotificationReason
	}{
		{ReasonFirstNotification, eventrecorder.NotificationReasonFirstNotification},
		{ReasonNewAlertsInGroup, eventrecorder.NotificationReasonNewAlertsInGroup},
		{ReasonAllAlertsResolved, eventrecorder.NotificationReasonAllAlertsResolved},
		{ReasonNewResolvedAlerts, eventrecorder.NotificationReasonNewResolvedAlerts},
		{ReasonRepeatIntervalElapsed, eventrecorder.NotificationReasonRepeatIntervalElapsed},
		{ReasonDoNotNotify, eventrecorder.NotificationReasonUnspecified},
		{ReasonAlertsUnmuted, eventrecorder.NotificationReasonUnspecified},
		{ReasonAllAlertsMuted, eventrecorder.NotificationReasonUnspecified},
		{ReasonStillMuted, eventrecorder.NotificationReasonUnspecified},
	}

	for _, test := range tests {
		t.Run(test.reason.String(), func(t *testing.T) {
			require.Equal(t, test.want, notifyReasonToEvent(test.reason))
		})
	}
}

// TestMutedAlertDetails asserts the events see the muted alerts in the same
// fixed order the notification log records them in, whether or not the muted
// alerts feature is enabled: the event recorder reads this slot either way.
func TestMutedAlertDetails(t *testing.T) {
	require.Nil(t, mutedAlertDetails(context.Background()), "no mute stage has run")

	alerts := make([]*alert.Alert, 0, 4)
	for _, name := range []string{"First", "Second", "Third", "Fourth"} {
		alerts = append(alerts, &alert.Alert{Alert: model.Alert{
			Labels: model.LabelSet{"alertname": model.LabelValue(name)},
		}})
	}

	byHash := func(a, b *alert.Alert) int { return cmp.Compare(hashAlert(a), hashAlert(b)) }

	got := mutedAlertDetails(recordMuted(context.Background(), alerts))
	require.ElementsMatch(t, alerts, got)
	require.True(t, slices.IsSortedFunc(got, byHash), "muted alerts come back in hash order")
}
