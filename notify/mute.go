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
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/silence"
)

const (
	SuppressedReasonSilence            = "silence"
	SuppressedReasonInhibition         = "inhibition"
	SuppressedReasonMuteTimeInterval   = "mute_time_interval"
	SuppressedReasonActiveTimeInterval = "active_time_interval"
)

// A Muter determines whether a given label set is muted. Implementers that
// maintain an underlying AlertMarker are expected to update it during a call of
// Mutes. The label set carries the fingerprint of the alert it belongs to when
// known, so implementers should use lset.Fingerprint() rather than rehashing.
type Muter interface {
	Mutes(ctx context.Context, lset labelset.LabelSet) bool
}

// A MuteFunc is a function that implements the Muter interface.
type MuteFunc func(ctx context.Context, lset labelset.LabelSet) bool

// Mutes implements the Muter interface.
func (f MuteFunc) Mutes(ctx context.Context, lset labelset.LabelSet) bool { return f(ctx, lset) }

// recordMuted adds the hashes of the given alerts to the set of muted alert
// hashes in the context.
func recordMuted(ctx context.Context, muted []*alert.Alert) context.Context {
	hashes, _ := MutedAlerts(ctx)
	if hashes == nil {
		hashes = make(map[uint64]struct{}, len(muted))
	}
	for _, a := range muted {
		hashes[hashAlert(a)] = struct{}{}
	}
	ctx = WithMutedAlerts(ctx, hashes)
	return withMutedAlertDetails(ctx, muted)
}

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
func (n *MuteStage) Exec(ctx context.Context, logger *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	ctx, span := tracer.Start(ctx, "notify.MuteStage.Exec",
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	var (
		filtered []*alert.Alert
		muted    []*alert.Alert
	)
	for _, a := range alerts {
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if muted.
		if n.muter.Mutes(ctx, a.LabelSet()) {
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

		ctx = recordMuted(ctx, muted)
	}

	return ctx, filtered, nil
}

// A TimeMuter determines if the time is muted by one or more active or mute
// time intervals. If the time is muted, it returns true and the names of the
// time intervals that muted it. Otherwise, it returns false and a nil slice.
type TimeMuter interface {
	Mutes(timeIntervalNames []string, now time.Time) (bool, []string, error)
}

// mutedByActiveTimeIntervals returns the names of the active time intervals
// that mute a group at time now. A group with active time intervals is muted
// when now is outside all of them, in which case it is muted by every active
// time interval. A group without active time intervals is never muted by them.
func mutedByActiveTimeIntervals(muter TimeMuter, activeTimeIntervalNames []string, now time.Time) ([]string, error) {
	if len(activeTimeIntervalNames) == 0 {
		return nil, nil
	}
	active, _, err := muter.Mutes(activeTimeIntervalNames, now)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, nil
	}
	return activeTimeIntervalNames, nil
}

// mutedByMuteTimeIntervals returns the names of the mute time intervals that
// mute a group at time now, which are the ones that contain now. A group
// without mute time intervals is never muted by them.
func mutedByMuteTimeIntervals(muter TimeMuter, muteTimeIntervalNames []string, now time.Time) ([]string, error) {
	if len(muteTimeIntervalNames) == 0 {
		return nil, nil
	}
	_, mutedBy, err := muter.Mutes(muteTimeIntervalNames, now)
	if err != nil {
		return nil, err
	}
	return mutedBy, nil
}

// MutedByTimeIntervals returns the names of the time intervals that mute a
// group with the given active and mute time intervals at time now. The group
// is muted if the returned slice is not empty. It computes the same result
// the TimeActiveStage and TimeMuteStage would produce for a flush at that
// time: active time intervals are checked first and, only if the group is in
// an active time, mute time intervals are checked next. The names must be
// resolvable by muter, so both must come from the same configuration.
func MutedByTimeIntervals(muter TimeMuter, activeTimeIntervalNames, muteTimeIntervalNames []string, now time.Time) ([]string, error) {
	mutedBy, err := mutedByActiveTimeIntervals(muter, activeTimeIntervalNames, now)
	if err != nil || len(mutedBy) > 0 {
		return mutedBy, err
	}
	return mutedByMuteTimeIntervals(muter, muteTimeIntervalNames, now)
}

type timeStage struct {
	muter   TimeMuter
	metrics *Metrics
}

type TimeMuteStage timeStage

func NewTimeMuteStage(muter TimeMuter, metrics *Metrics) *TimeMuteStage {
	return &TimeMuteStage{muter, metrics}
}

// Exec implements the stage interface for TimeMuteStage.
// TimeMuteStage is responsible for muting alerts whose route is not in an active time.
func (tms TimeMuteStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	ctx, span := tracer.Start(ctx, "notify.TimeMuteStage.Exec",
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

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

	mutedBy, err := mutedByMuteTimeIntervals(tms.muter, muteTimeIntervalNames, now)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return ctx, alerts, err
	}

	// If the current time is inside a mute time, all alerts are removed from the pipeline.
	if len(mutedBy) > 0 {
		tms.metrics.numNotificationSuppressedTotal.WithLabelValues(SuppressedReasonMuteTimeInterval).Add(float64(len(alerts)))
		l.Debug("Notifications not sent, route is within mute time", "alerts", len(alerts))
		span.AddEvent("notify.TimeMuteStage.Exec muted the alerts")
		return recordMuted(ctx, alerts), nil, nil
	}

	return ctx, alerts, nil
}

type TimeActiveStage timeStage

func NewTimeActiveStage(muter TimeMuter, metrics *Metrics) *TimeActiveStage {
	return &TimeActiveStage{muter, metrics}
}

// Exec implements the stage interface for TimeActiveStage.
// TimeActiveStage is responsible for muting alerts whose route is not in an active time.
func (tas TimeActiveStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	ctx, span := tracer.Start(ctx, "notify.TimeActiveStage.Exec",
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, errors.New("group key missing")
	}
	span.SetAttributes(attribute.String("alerting.group.key", gkey))

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

	mutedBy, err := mutedByActiveTimeIntervals(tas.muter, activeTimeIntervalNames, now)
	if err != nil {
		return ctx, alerts, err
	}

	// If the current time is not inside an active time, all alerts are removed from the pipeline
	if len(mutedBy) > 0 {
		span.AddEvent("notify.TimeActiveStage.Exec not active, removing all alerts")
		tas.metrics.numNotificationSuppressedTotal.WithLabelValues(SuppressedReasonActiveTimeInterval).Add(float64(len(alerts)))
		l.Debug("Notifications not sent, route is not within active time", "alerts", len(alerts))
		return recordMuted(ctx, alerts), nil, nil
	}

	return ctx, alerts, nil
}
