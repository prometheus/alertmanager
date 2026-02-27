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

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/types"
)

// A Muter determines whether a given label set is muted. Implementers that
// maintain an underlying AlertMarker are expected to update it during a call of
// Mutes.
type Muter interface {
	Mutes(ctx context.Context, lset model.LabelSet) bool
}

// A MuteFunc is a function that implements the Muter interface.
type MuteFunc func(ctx context.Context, lset model.LabelSet) bool

// Mutes implements the Muter interface.
func (f MuteFunc) Mutes(ctx context.Context, lset model.LabelSet) bool { return f(ctx, lset) }

// MuteStage filters alerts through a Muter.
type MuteStage struct {
	muter   Muter
	metrics *Metrics
}

// NewMuteStage return a new MuteStage.
func NewMuteStage(m Muter, metrics *Metrics) *MuteStage {
	return &MuteStage{muter: m, metrics: metrics}
}

// Exec implements the Stage interface.
func (n *MuteStage) Exec(ctx context.Context, logger *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	ctx, span := tracer.Start(ctx, "notify.MuteStage.Exec",
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	var (
		filtered []*types.Alert
		muted    []*types.Alert
	)
	for _, a := range alerts {
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if muted.
		if n.muter.Mutes(ctx, a.Labels) {
			muted = append(muted, a)
		} else {
			filtered = append(filtered, a)
		}
		// TODO(fabxc): increment muted alerts counter if muted.
	}
	if len(muted) > 0 {

		var reason string
		switch n.muter.(type) {
		case *silence.Silencer:
			reason = SuppressedReasonSilence
		case *inhibit.Inhibitor:
			reason = SuppressedReasonInhibition
		default:
		}
		span.SetAttributes(
			attribute.Int("alerting.alerts.muted.count", len(muted)),
			attribute.Int("alerting.alerts.filtered.count", len(filtered)),
			attribute.String("alerting.suppressed.reason", reason),
		)
		n.metrics.numNotificationSuppressedTotal.WithLabelValues(reason).Add(float64(len(muted)))
		logger.Debug("Notifications will not be sent for muted alerts", "alerts", fmt.Sprintf("%v", muted), "reason", reason)
	}

	return ctx, filtered, nil
}

// A TimeMuter determines if the time is muted by one or more active or mute
// time intervals. If the time is muted, it returns true and the names of the
// time intervals that muted it. Otherwise, it returns false and a nil slice.
type TimeMuter interface {
	Mutes(timeIntervalNames []string, now time.Time) (bool, []string, error)
}

type timeStage struct {
	muter   TimeMuter
	marker  types.GroupMarker
	metrics *Metrics
}

type TimeMuteStage timeStage

func NewTimeMuteStage(muter TimeMuter, marker types.GroupMarker, metrics *Metrics) *TimeMuteStage {
	return &TimeMuteStage{muter, marker, metrics}
}

// Exec implements the stage interface for TimeMuteStage.
// TimeMuteStage is responsible for muting alerts whose route is not in an active time.
func (tms TimeMuteStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	ctx, span := tracer.Start(ctx, "notify.TimeMuteStage.Exec",
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	routeID, ok := RouteID(ctx)
	if !ok {
		err := errors.New("route ID missing")
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return ctx, nil, err
	}
	span.SetAttributes(attribute.String("alerting.route.id", routeID))

	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, errors.New("group key missing")
	}
	span.SetAttributes(attribute.String("alerting.group.key", gkey))

	muteTimeIntervalNames, ok := MuteTimeIntervalNames(ctx)
	if !ok {
		return ctx, alerts, nil
	}
	now, ok := Now(ctx)
	if !ok {
		return ctx, alerts, errors.New("missing now timestamp")
	}

	// Skip this stage if there are no mute timings.
	if len(muteTimeIntervalNames) == 0 {
		return ctx, alerts, nil
	}

	muted, mutedBy, err := tms.muter.Mutes(muteTimeIntervalNames, now)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return ctx, alerts, err
	}
	// If muted is false then mutedBy is nil and the muted marker is removed.
	tms.marker.SetMuted(routeID, gkey, mutedBy)

	// If the current time is inside a mute time, all alerts are removed from the pipeline.
	if muted {
		tms.metrics.numNotificationSuppressedTotal.WithLabelValues(SuppressedReasonMuteTimeInterval).Add(float64(len(alerts)))
		l.Debug("Notifications not sent, route is within mute time", "alerts", len(alerts))
		span.AddEvent("notify.TimeMuteStage.Exec muted the alerts")
		return ctx, nil, nil
	}

	return ctx, alerts, nil
}

type TimeActiveStage timeStage

func NewTimeActiveStage(muter TimeMuter, marker types.GroupMarker, metrics *Metrics) *TimeActiveStage {
	return &TimeActiveStage{muter, marker, metrics}
}

// Exec implements the stage interface for TimeActiveStage.
// TimeActiveStage is responsible for muting alerts whose route is not in an active time.
func (tas TimeActiveStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	routeID, ok := RouteID(ctx)
	if !ok {
		return ctx, nil, errors.New("route ID missing")
	}

	ctx, span := tracer.Start(ctx, "notify.TimeActiveStage.Exec",
		trace.WithAttributes(attribute.String("alerting.route.id", routeID)),
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, errors.New("group key missing")
	}

	activeTimeIntervalNames, ok := ActiveTimeIntervalNames(ctx)
	if !ok {
		return ctx, alerts, nil
	}

	// if we don't have active time intervals at all it is always active.
	if len(activeTimeIntervalNames) == 0 {
		return ctx, alerts, nil
	}

	now, ok := Now(ctx)
	if !ok {
		return ctx, alerts, errors.New("missing now timestamp")
	}

	active, _, err := tas.muter.Mutes(activeTimeIntervalNames, now)
	if err != nil {
		return ctx, alerts, err
	}

	var mutedBy []string
	if !active {
		// If the group is muted, then it must be muted by all active time intervals.
		// Otherwise, the group must be in at least one active time interval for it
		// to be active.
		mutedBy = activeTimeIntervalNames
	}
	tas.marker.SetMuted(routeID, gkey, mutedBy)

	// If the current time is not inside an active time, all alerts are removed from the pipeline
	if !active {
		span.AddEvent("notify.TimeActiveStage.Exec not active, removing all alerts")
		tas.metrics.numNotificationSuppressedTotal.WithLabelValues(SuppressedReasonActiveTimeInterval).Add(float64(len(alerts)))
		l.Debug("Notifications not sent, route is not within active time", "alerts", len(alerts))
		return ctx, nil, nil
	}

	return ctx, alerts, nil
}
