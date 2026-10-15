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

import "github.com/prometheus/alertmanager/nflog/nflogpb"

// A NotificationSequence is where an alert group stands in the run of
// notifications one receiver has been sent about it. It opens with the first
// notification and closes when the receiver is told the group is over, either
// because every alert resolved or because every alert was muted.
//
// It is derived from the notification log entry and the current flush, never
// stored. Integrations read it to tell a group that went quiet because it was
// muted from one that went quiet because it resolved.
type NotificationSequence int

const (
	// SequenceNone means the receiver knows of no firing alert in this group:
	// either it was never notified, or its last sequence has closed.
	SequenceNone NotificationSequence = iota
	// SequenceOpen means the receiver was shown firing alerts it has not been
	// told are over.
	SequenceOpen
	// SequenceClosedResolved means this notification closes the sequence,
	// because every alert in the group resolved.
	SequenceClosedResolved
	// SequenceClosedMuted means this notification closes the sequence because
	// the group is still firing but every alert in it is muted.
	SequenceClosedMuted
)

func (s NotificationSequence) String() string {
	switch s {
	case SequenceNone:
		return "none"
	case SequenceOpen:
		return "open"
	case SequenceClosedResolved:
		return "closed, all alerts resolved"
	case SequenceClosedMuted:
		return "closed, all alerts muted"
	default:
		return "unknown"
	}
}

// newNotificationSequence derives where the group stands once this flush's
// notification, if any, has been sent.
func newNotificationSequence(entry *nflogpb.Entry, s groupState, reason NotifyReason) NotificationSequence {
	switch reason {
	case ReasonAllAlertsResolved:
		return SequenceClosedResolved
	case ReasonAllAlertsMuted:
		return SequenceClosedMuted
	}

	// A notification shows the firing alerts that are not muted. Without one,
	// the receiver still knows only what the log says it was last shown.
	var notified map[uint64]struct{}
	switch {
	case reason.shouldNotify():
		notified = s.visibleFiring()
	case entry == nil:
		return SequenceNone
	default:
		notified = entry.NotifiedFiringAlerts()
	}

	if len(notified) > 0 {
		return SequenceOpen
	}
	return SequenceNone
}
