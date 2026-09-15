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

	now  func() time.Time
	hash func(*alert.Alert) uint64
}

// NewDedupStage wraps a DedupStage that runs against the given notification log.
func NewDedupStage(rs ResolvedSender, l NotificationLog, recv *nflogpb.Receiver) *DedupStage {
	return &DedupStage{
		rs:    rs,
		nflog: l,
		recv:  recv,
		now:   utcNow,
		hash:  hashAlert,
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

	firing, resolved, firingSet, resolvedSet := partitionAlertsByState(alerts, n.hash)

	ctx = WithFiringAlerts(ctx, firing)
	ctx = WithResolvedAlerts(ctx, resolved)

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
	updateReason := n.needsUpdate(entry, firingSet, resolvedSet, repeatInterval, now)
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
