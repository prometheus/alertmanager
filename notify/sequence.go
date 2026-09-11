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
// notifications one receiver has been sent about it. A sequence opens with the
// first notification about a group and closes when the receiver is told the
// group is over, either because every alert in it resolved or because every
// alert in it was muted.
//
// It is derived from the notification log entry and the state of the group at
// the current flush, and is never stored: the notification log entry remains
// the only record. Integrations read it to tell a group that has gone quiet
// because it was muted apart from one that has gone quiet because it resolved,
// which they cannot do from the alerts they are handed.
type NotificationSequence int

const (
	// SequenceNone means the receiver has not been told about any firing alert
	// in this group that it has not also been told is over. Either the group
	// has never been notified about, or its last sequence has closed.
	SequenceNone NotificationSequence = iota
	// SequenceOpen means the receiver has been told about firing alerts it has
	// not been told are over.
	SequenceOpen
	// SequenceClosedResolved means this notification closes the sequence
	// because every alert in the group has resolved.
	SequenceClosedResolved
	// SequenceClosedMuted means this notification closes the sequence because
	// the group is still firing but every alert in it is muted, so the
	// receiver can no longer be shown any of it.
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

// newNotificationSequence derives where the group stands once the
// notification this flush decided on, if any, has been sent.
func newNotificationSequence(entry *nflogpb.Entry, s groupState, reason NotifyReason) NotificationSequence {
	switch reason {
	case ReasonAllAlertsResolved:
		return SequenceClosedResolved
	case ReasonAllAlertsMuted:
		return SequenceClosedMuted
	}

	// A notification shows the receiver the alerts that are firing and not
	// muted. Without one, the receiver knows what the notification log says it
	// was last told.
	notified := s.visibleFiring()
	if !reason.shouldNotify() {
		if entry == nil {
			return SequenceNone
		}
		notified = entry.NotifiedFiringAlerts()
	}

	if len(notified) > 0 {
		return SequenceOpen
	}
	return SequenceNone
}
