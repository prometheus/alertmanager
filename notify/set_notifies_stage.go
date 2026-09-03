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
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// SetNotifiesStage sets the notification information about passed alerts. The
// passed alerts should have already been sent to the receivers.
type SetNotifiesStage struct {
	nflog NotificationLog
	recv  *nflogpb.Receiver
	ff    featurecontrol.Flagger
}

// NewSetNotifiesStage returns a new instance of a SetNotifiesStage.
func NewSetNotifiesStage(l NotificationLog, recv *nflogpb.Receiver, ff featurecontrol.Flagger) *SetNotifiesStage {
	return &SetNotifiesStage{
		nflog: l,
		recv:  recv,
		ff:    ff,
	}
}

// mutedAlerts returns the hashes of the alerts a mute stage removed from the
// pipeline, sorted so that the entry written to the notification log is stable
// across flushes and across peers.
func (n SetNotifiesStage) mutedAlerts(ctx context.Context) []uint64 {
	if !n.ff.EnableMutedAlertsInNflog() {
		return nil
	}

	muted, ok := MutedAlerts(ctx)
	if !ok || len(muted) == 0 {
		return nil
	}

	hashes := make([]uint64, 0, len(muted))
	for hash := range muted {
		hashes = append(hashes, hash)
	}
	slices.Sort(hashes)

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
