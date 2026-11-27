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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/tracing"
)

// DedupStage filters alerts.
// Filtering happens based on a notification log.
type DedupStage struct {
	rs    ResolvedSender
	nflog NotificationLog
	recv  *nflogpb.Receiver

	now  func() time.Time
	hash func(*alert.Alert) uint64
}

// NewDedupStage wraps a DedupStage that runs against the given notification log.
// The stage decides what to notify from the whole group, muted alerts included,
// and records the group's state in the notification log.
func NewDedupStage(rs ResolvedSender, l NotificationLog, recv *nflogpb.Receiver) *DedupStage {
	return &DedupStage{
		rs:    rs,
		nflog: l,
		recv:  recv,
		now:   utcNow,
		hash:  hashAlert,
	}
}

// groupState is the state of an alert group at a flush. Muting decides whether
// the receiver is shown an alert, not whether it is firing, so a muted alert is
// in firing or resolved as well as in muted.
type groupState struct {
	firing   []uint64
	resolved []uint64

	firingSet   map[uint64]struct{}
	resolvedSet map[uint64]struct{}
	mutedSet    map[uint64]struct{}
}

// visibleFiring returns the firing alerts the receiver is shown.
func (s groupState) visibleFiring() map[uint64]struct{} {
	return withoutMuted(s.firingSet, s.mutedSet)
}

// visibleResolved returns the resolved alerts the receiver is shown.
func (s groupState) visibleResolved() map[uint64]struct{} {
	return withoutMuted(s.resolvedSet, s.mutedSet)
}

// withoutMuted returns the members of set that are not muted.
func withoutMuted(set, muted map[uint64]struct{}) map[uint64]struct{} {
	if len(muted) == 0 {
		return set
	}

	visible := make(map[uint64]struct{}, len(set))
	for hash := range set {
		if _, ok := muted[hash]; !ok {
			visible[hash] = struct{}{}
		}
	}
	return visible
}

// newGroupState partitions the group this flush is about. The alerts a mute
// stage removed are partitioned too, so that the notification log records the
// whole group.
func (n *DedupStage) newGroupState(ctx context.Context, alerts []*alert.Alert) groupState {
	firing, resolved, firingSet, resolvedSet := partitionAlertsByState(alerts, n.hash)
	s := groupState{
		firing:      firing,
		resolved:    resolved,
		firingSet:   firingSet,
		resolvedSet: resolvedSet,
	}

	// In hash order, so that an unchanged group produces the same entry.
	hashes, muted := sortedMutedAlerts(ctx)
	s.mutedSet = make(map[uint64]struct{}, len(hashes))
	for _, hash := range hashes {
		s.mutedSet[hash] = struct{}{}
		if muted[hash].Resolved() {
			s.resolved = append(s.resolved, hash)
			s.resolvedSet[hash] = struct{}{}
		} else {
			s.firing = append(s.firing, hash)
			s.firingSet[hash] = struct{}{}
		}
	}
	return s
}

// needsUpdate decides whether the group needs a notification. Each question is
// asked against the set that answers it: whether the group is over is decided
// by every alert in it, muted or not, and whether the receiver already knows
// about an alert by the alerts it was shown.
func (n *DedupStage) needsUpdate(entry *nflogpb.Entry, s groupState, repeat time.Duration, now time.Time) NotifyReason {
	visibleFiring := s.visibleFiring()

	// If we haven't notified about the alert group before, notify right away
	// unless there is nothing to show the receiver.
	if entry == nil {
		if len(visibleFiring) > 0 {
			return ReasonFirstNotification
		}
		return ReasonDoNotNotify
	}

	// What the receiver was shown at the last notification. Muted alerts were
	// recorded but not delivered, so as far as it is concerned they never fired.
	notifiedFiring := entry.NotifiedFiringAlerts()

	// Is the group over? Nothing in it is firing, muted included.
	if len(s.firingSet) == 0 {
		// Should we notify? The log has to say the group was firing,
		// and either the receiver saw some of it, or none of the group is
		// visible so no other notification will report the end.
		if len(entry.FiringAlerts) > 0 &&
			(len(notifiedFiring) > 0 || len(s.visibleResolved()) == 0) {
			return ReasonAllAlertsResolved
		}
		return ReasonDoNotNotify
	}

	// Alerts the receiver has not been shown.
	if !nflogpb.IsSubset(notifiedFiring, visibleFiring) {
		// Nothing was shown last time, so this opens a new sequence.
		if len(notifiedFiring) == 0 {
			return ReasonFirstNotification
		}
		// Every visible alert was already in the group, so it became visible
		// because a mute ended, not because it started firing.
		if nflogpb.IsSubset(entry.FiringAlertSet(), visibleFiring) {
			return ReasonAlertsUnmuted
		}
		return ReasonNewAlertsInGroup
	}

	if n.rs.SendResolved() && !nflogpb.IsSubset(entry.NotifiedResolvedAlerts(), s.visibleResolved()) {
		return ReasonNewResolvedAlerts
	}

	isRepeatIntervalElapsed := entry.Timestamp.AsTime().Before(now.Add(-repeat))

	// The group is still firing but none of it can be shown, and there is
	// nothing else to say, so the sequence closes as muted, not as resolved.
	if len(visibleFiring) == 0 {
		if len(notifiedFiring) > 0 {
			return ReasonAllAlertsMuted
		}
		// The sequence is already closed. Nothing is delivered while the group
		// stays muted, so without a rewrite the entry expires and the group
		// comes back as one the receiver has never been told about.
		if isRepeatIntervalElapsed {
			return ReasonStillMuted
		}
		return ReasonDoNotNotify
	}

	// Nothing changed, only notify if the repeat interval has passed.
	if isRepeatIntervalElapsed {
		return ReasonRepeatIntervalElapsed
	}
	return ReasonDoNotNotify
}

// partitionAlertsByState separates alerts into firing and resolved, returning both slices and sets.
func partitionAlertsByState(alerts []*alert.Alert, hashFn func(*alert.Alert) uint64) (firing, resolved []uint64, firingSet, resolvedSet map[uint64]struct{}) {
	firingSet = make(map[uint64]struct{}, len(alerts))
	resolvedSet = make(map[uint64]struct{}, len(alerts))
	firing = make([]uint64, 0, len(alerts))
	resolved = make([]uint64, 0, len(alerts))

	for _, a := range alerts {
		hash := hashFn(a)
		if a.Resolved() {
			resolved = append(resolved, hash)
			resolvedSet[hash] = struct{}{}
		} else {
			firing = append(firing, hash)
			firingSet[hash] = struct{}{}
		}
	}
	return firing, resolved, firingSet, resolvedSet
}

// Exec implements the Stage interface.
func (n *DedupStage) Exec(ctx context.Context, _ *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, errors.New("group key missing")
	}

	ctx, span := tracer.StartDedupStage(ctx, gkey, len(alerts))
	defer span.End()

	repeatInterval, ok := RepeatInterval(ctx)
	if !ok {
		return ctx, nil, errors.New("repeat interval missing")
	}

	state := n.newGroupState(ctx, alerts)

	ctx = WithFiringAlerts(ctx, state.firing)
	ctx = WithResolvedAlerts(ctx, state.resolved)

	entries, err := n.nflog.Query(nflog.QGroupKey(gkey), nflog.QReceiver(n.recv))
	if err != nil && !errors.Is(err, nflog.ErrNotFound) {
		return ctx, nil, err
	}

	var entry *nflogpb.Entry
	switch len(entries) {
	case 0:
	case 1:
		entry = entries[0]
	default:
		return ctx, nil, fmt.Errorf("unexpected entry result size %d", len(entries))
	}

	now := n.now()
	if ctxNow, ok := Now(ctx); ok {
		now = ctxNow
	}
	updateReason := n.needsUpdate(entry, state, repeatInterval, now)
	ctx = WithNotificationSequence(ctx, newNotificationSequence(entry, state, updateReason))
	ctx = WithNotificationReason(ctx, updateReason)

	if updateReason == ReasonFirstNotification {
		ctx = WithNflogStore(ctx, nflog.NewStore(nil))
	} else {
		ctx = WithNflogStore(ctx, nflog.NewStore(entry))
	}

	// When ReasonStillMuted, nflog is updated, but there are no alerts to deliver.
	if updateReason == ReasonStillMuted {
		span.AddEvent("notify.DedupStage.Exec nflog needs update, nothing to deliver")
		return ctx, nil, nil
	}

	if updateReason.shouldNotify() {
		tracing.EventDedupNeedsUpdate(span)
		return ctx, alerts, nil
	}
	return ctx, nil, nil
}
