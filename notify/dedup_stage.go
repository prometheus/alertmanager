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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// DedupStage filters alerts.
// Filtering happens based on a notification log.
type DedupStage struct {
	rs    ResolvedSender
	nflog NotificationLog
	recv  *nflogpb.Receiver

	// mutedAware is the muted-alerts-in-nflog feature, resolved once by the
	// pipeline builder. See newMultiStage.
	mutedAware bool

	now  func() time.Time
	hash func(*alert.Alert) uint64
}

// NewDedupStage wraps a DedupStage that runs against the given notification log.
// When mutedAware is set the stage decides what to notify from the whole group,
// muted alerts included, and records the group's state in the notification log.
func NewDedupStage(rs ResolvedSender, l NotificationLog, recv *nflogpb.Receiver, mutedAware bool) *DedupStage {
	return &DedupStage{
		rs:         rs,
		nflog:      l,
		recv:       recv,
		mutedAware: mutedAware,
		now:        utcNow,
		hash:       hashAlert,
	}
}

func (n *DedupStage) needsUpdate(entry *nflogpb.Entry, firing, resolved map[uint64]struct{}, repeat time.Duration, now time.Time) NotifyReason {
	// If we haven't notified about the alert group before, notify right away
	// unless we only have resolved alerts.
	if entry == nil {
		if len(firing) > 0 {
			return ReasonFirstNotification
		}
		return ReasonDoNotNotify
	}

	// new alerts in the group
	if !entry.IsFiringSubset(firing) {
		// If the previous entry has no firing alerts, it was a resolution and we
		// should treat this as the first notification for the group.
		if len(entry.FiringAlerts) == 0 {
			return ReasonFirstNotification
		}
		return ReasonNewAlertsInGroup
	}

	// Notify about all alerts being resolved.
	// This is done irrespective of the send_resolved flag to make sure that
	// the firing alerts are cleared from the notification log.
	if len(firing) == 0 {
		// If the current alert group and last notification contain no firing
		// alert, it means that some alerts have been fired and resolved during the
		// last interval. In this case, there is no need to notify the receiver
		// since it doesn't know about them.
		if len(entry.FiringAlerts) > 0 {
			return ReasonAllAlertsResolved
		}
		return ReasonDoNotNotify
	}

	if n.rs.SendResolved() && !entry.IsResolvedSubset(resolved) {
		return ReasonNewResolvedAlerts
	}

	// Nothing changed, only notify if the repeat interval has passed.
	isRepeatIntervalElapsed := entry.Timestamp.AsTime().Before(now.Add(-repeat))
	if isRepeatIntervalElapsed {
		return ReasonRepeatIntervalElapsed
	}
	return ReasonDoNotNotify
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

// newGroupState partitions the group this flush is about. With the feature
// enabled the alerts a mute stage removed are partitioned too, so that the
// notification log records the whole group.
func (n *DedupStage) newGroupState(ctx context.Context, alerts []*alert.Alert) groupState {
	firing, resolved, firingSet, resolvedSet := partitionAlertsByState(alerts, n.hash)
	s := groupState{
		firing:      firing,
		resolved:    resolved,
		firingSet:   firingSet,
		resolvedSet: resolvedSet,
	}

	if !n.mutedAware {
		return s
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

// needsUpdateMuteAware asks the same questions as needsUpdate, but each against
// the set that answers it: whether the group is over is decided by every alert
// in it, muted or not, and whether the receiver already knows about an alert by
// the alerts it was shown.
func (n *DedupStage) needsUpdateMuteAware(entry *nflogpb.Entry, s groupState, repeat time.Duration, now time.Time) NotifyReason {
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

	// Notify about all alerts being resolved. The whole group decides this: an
	// alert that is firing but muted still keeps the group from resolving.
	if len(s.firingSet) == 0 {
		// The receiver cannot be told about alerts it was never shown.
		if len(notifiedFiring) == 0 {
			return ReasonDoNotNotify
		}
		// Every alert that ended the group is muted, so there
		// is nothing to send and nothing worth recording.
		if len(s.visibleResolved()) == 0 {
			return ReasonDoNotNotify
		}
		return ReasonAllAlertsResolved
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

	// The group is still firing but none of it can be shown, and there is
	// nothing else to say, so the sequence closes as muted, not as resolved.
	if len(visibleFiring) == 0 {
		if len(notifiedFiring) > 0 {
			return ReasonAllAlertsMuted
		}
		return ReasonDoNotNotify
	}

	// Nothing changed, only notify if the repeat interval has passed.
	isRepeatIntervalElapsed := entry.Timestamp.AsTime().Before(now.Add(-repeat))
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

	ctx, span := tracer.Start(ctx, "notify.DedupStage.Exec",
		trace.WithAttributes(attribute.String("alerting.group.key", gkey)),
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
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
	var updateReason NotifyReason
	if n.mutedAware {
		updateReason = n.needsUpdateMuteAware(entry, state, repeatInterval, now)
		ctx = WithNotificationSequence(ctx, newNotificationSequence(entry, state, updateReason))
	} else {
		updateReason = n.needsUpdate(entry, state.firingSet, state.resolvedSet, repeatInterval, now)
	}
	ctx = WithNotificationReason(ctx, updateReason)

	if updateReason == ReasonFirstNotification {
		ctx = WithNflogStore(ctx, nflog.NewStore(nil))
	} else {
		ctx = WithNflogStore(ctx, nflog.NewStore(entry))
	}

	if updateReason.shouldNotify() {
		span.AddEvent("notify.DedupStage.Exec nflog needs update")
		return ctx, alerts, nil
	}
	return ctx, nil, nil
}
