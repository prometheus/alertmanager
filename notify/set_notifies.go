package notify

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SetNotifiesStage sets the notification information about passed alerts. The
// passed alerts should have already been sent to the receivers.
type SetNotifiesStage struct {
	nflog NotificationLog
	recv  *nflogpb.Receiver
}

// NewSetNotifiesStage returns a new instance of a SetNotifiesStage.
func NewSetNotifiesStage(l NotificationLog, recv *nflogpb.Receiver) *SetNotifiesStage {
	return &SetNotifiesStage{
		nflog: l,
		recv:  recv,
	}
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

	span.SetAttributes(
		attribute.Int("alerting.alerts.firing.count", len(firing)),
		attribute.Int("alerting.alerts.resolved.count", len(resolved)),
	)

	// Extract receiver data from context if present (it's ok for it to be nil).
	store, _ := NflogStore(ctx)
	return ctx, alerts, n.nflog.Log(n.recv, gkey, firing, resolved, store, expiry)
}
