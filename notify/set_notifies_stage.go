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
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// SetNotifiesStage sets the notification information about passed alerts. The
// passed alerts should have already been sent to the receivers.
type SetNotifiesStage struct {
	nflog NotificationLog
	recv  *nflogpb.Receiver

	// mutedAware is the muted-alerts-in-nflog feature, resolved once by the
	// pipeline builder. See newMultiStage.
	mutedAware bool
}

// NewSetNotifiesStage returns a new instance of a SetNotifiesStage.
// When mutedAware is set the log entry also records the alerts a mute stage
// removed from the pipeline.
func NewSetNotifiesStage(l NotificationLog, recv *nflogpb.Receiver, mutedAware bool) *SetNotifiesStage {
	return &SetNotifiesStage{
		nflog:      l,
		recv:       recv,
		mutedAware: mutedAware,
	}
}

// mutedAlerts returns the hashes of the alerts a mute stage removed from the
// pipeline, for the log entry to record.
func (n SetNotifiesStage) mutedAlerts(ctx context.Context) []uint64 {
	if !n.mutedAware {
		return nil
	}

	hashes, _ := sortedMutedAlerts(ctx)
	return hashes
}

// Exec implements the Stage interface.
func (n SetNotifiesStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, errors.New("group key missing")
	}

	ctx, span := tracer.Start(ctx, "notify.SetNotifiesStage.Exec",
		trace.WithAttributes(attribute.String("alerting.group.key", gkey)),
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// With the feature enabled this stage is reached even when the group was
	// emptied and nothing was delivered. The entry records the group as the
	// receiver was last shown it, and rewriting it would refresh its timestamp
	// and defer the repeat interval forever.
	if reason, ok := NotificationReason(ctx); ok && !reason.shouldNotify() {
		span.AddEvent("notify.SetNotifiesStage.Exec nothing was notified, log entry left unchanged")
		return ctx, alerts, nil
	}

	firing, ok := FiringAlerts(ctx)
	if !ok {
		return ctx, nil, errors.New("firing alerts missing")
	}

	resolved, ok := ResolvedAlerts(ctx)
	if !ok {
		return ctx, nil, errors.New("resolved alerts missing")
	}

	repeat, ok := RepeatInterval(ctx)
	if !ok {
		return ctx, nil, errors.New("repeat interval missing")
	}
	expiry := 2 * repeat

	muted := n.mutedAlerts(ctx)

	span.SetAttributes(
		attribute.Int("alerting.alerts.firing.count", len(firing)),
		attribute.Int("alerting.alerts.resolved.count", len(resolved)),
		attribute.Int("alerting.alerts.muted.count", len(muted)),
	)

	// Extract receiver data from context if present (it's ok for it to be nil).
	store, _ := NflogStore(ctx)
	return ctx, alerts, n.nflog.Log(n.recv, gkey, firing, resolved, muted, store, expiry)
}
