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
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// TestNewNotificationSequence covers how the sequence is derived from the log
// entry and this flush. Hashes 1, 2 and 3 stand for three alerts in one group.
func TestNewNotificationSequence(t *testing.T) {
	entry := func(firing, resolved, muted []uint64) *nflogpb.Entry {
		return &nflogpb.Entry{
			FiringAlerts:   firing,
			ResolvedAlerts: resolved,
			MutedAlerts:    muted,
			Timestamp:      timestamppb.New(utcNow()),
		}
	}

	tests := []struct {
		name   string
		entry  *nflogpb.Entry
		firing []uint64
		muted  []uint64
		reason NotifyReason
		want   NotificationSequence
	}{{
		// The reason alone settles the two closing cases.
		name:   "all alerts resolved closes the sequence",
		entry:  entry([]uint64{1}, nil, nil),
		reason: ReasonAllAlertsResolved,
		want:   SequenceClosedResolved,
	}, {
		name:   "all alerts muted closes the sequence",
		entry:  entry([]uint64{1}, nil, nil),
		firing: []uint64{1},
		muted:  []uint64{1},
		reason: ReasonAllAlertsMuted,
		want:   SequenceClosedMuted,
	}, {
		name:   "first notification opens the sequence",
		entry:  nil,
		firing: []uint64{1},
		reason: ReasonFirstNotification,
		want:   SequenceOpen,
	}, {
		name:   "an unmuted alert opens the sequence again",
		entry:  entry([]uint64{1}, nil, []uint64{1}),
		firing: []uint64{1},
		reason: ReasonAlertsUnmuted,
		want:   SequenceOpen,
	}, {
		// Notifying shows the firing alerts that are not muted. If that is
		// none of them, the receiver is left knowing of no firing alert.
		name:   "notifying about resolved alerts only leaves no sequence",
		entry:  entry(nil, []uint64{1}, nil),
		firing: nil,
		reason: ReasonNewResolvedAlerts,
		want:   SequenceNone,
	}, {
		// Without a notification the receiver knows only what the log says.
		name:   "no notification keeps the sequence the entry recorded",
		entry:  entry([]uint64{1}, nil, nil),
		firing: []uint64{1},
		reason: ReasonDoNotNotify,
		want:   SequenceOpen,
	}, {
		name:   "no notification and every logged alert was muted",
		entry:  entry([]uint64{1}, nil, []uint64{1}),
		firing: []uint64{1},
		muted:  []uint64{1},
		reason: ReasonDoNotNotify,
		want:   SequenceNone,
	}, {
		name:   "no notification and nothing was ever logged",
		entry:  nil,
		reason: ReasonDoNotNotify,
		want:   SequenceNone,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := mutedGroupState(test.firing, nil, test.muted)
			require.Equal(t, test.want, newNotificationSequence(test.entry, s, test.reason))
		})
	}
}

func TestNotificationSequenceString(t *testing.T) {
	require.Equal(t, "none", SequenceNone.String())
	require.Equal(t, "open", SequenceOpen.String())
	require.Equal(t, "closed, all alerts resolved", SequenceClosedResolved.String())
	require.Equal(t, "closed, all alerts muted", SequenceClosedMuted.String())
	require.Equal(t, "unknown", NotificationSequence(-1).String())
}

func TestNotifyReasonString(t *testing.T) {
	require.Equal(t, "first notification", ReasonFirstNotification.String())
	require.Equal(t, "new alerts added", ReasonNewAlertsInGroup.String())
	require.Equal(t, "some alerts resolved", ReasonNewResolvedAlerts.String())
	require.Equal(t, "all alerts resolved", ReasonAllAlertsResolved.String())
	require.Equal(t, "repeat interval elapsed", ReasonRepeatIntervalElapsed.String())
	require.Equal(t, "some alerts unmuted", ReasonAlertsUnmuted.String())
	require.Equal(t, "all alerts muted", ReasonAllAlertsMuted.String())
	require.Equal(t, "none", ReasonDoNotNotify.String())
	require.Equal(t, "unknown", NotifyReason(-1).String())
}

// TestNotifyReasonShouldNotify pins which reasons deliver a notification.
// ReasonAllAlertsMuted closes the sequence without one: telling the receiver
// its group went quiet is the per-receiver behaviour #5247 asks for, which
// does not exist yet.
func TestNotifyReasonShouldNotify(t *testing.T) {
	tests := []struct {
		reason NotifyReason
		want   bool
	}{
		{ReasonFirstNotification, true},
		{ReasonNewAlertsInGroup, true},
		{ReasonNewResolvedAlerts, true},
		{ReasonAllAlertsResolved, true},
		{ReasonRepeatIntervalElapsed, true},
		{ReasonAlertsUnmuted, true},
		{ReasonAllAlertsMuted, false},
		{ReasonDoNotNotify, false},
	}

	for _, test := range tests {
		t.Run(test.reason.String(), func(t *testing.T) {
			require.Equal(t, test.want, test.reason.shouldNotify())
		})
	}
}
