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

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
)

// RetryStage notifies via passed integration with exponential backoff until it
// succeeds. It aborts if the context is canceled or timed out.
type RetryStage struct {
	integration Integration
	groupName   string
	metrics     *Metrics
	labelValues []string
	recorder    eventrecorder.Recorder
}

// NewRetryStage returns a new instance of a RetryStage.
func NewRetryStage(i Integration, groupName string, metrics *Metrics, recorder eventrecorder.Recorder) *RetryStage {
	labelValues := []string{i.Name()}

	if metrics.ff.EnableReceiverNamesInMetrics() {
		labelValues = append(labelValues, i.receiverName)
	}

	return &RetryStage{
		integration: i,
		groupName:   groupName,
		metrics:     metrics,
		labelValues: labelValues,
		recorder:    recorder,
	}
}

func (r RetryStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	// Every alert in the group was muted, so there is nothing left to deliver.
	// An integration whose mute_action asks for the close is told the group is
	// over anyway; every other one is told nothing. Later stages run either way,
	// to record the group's state.
	closing := len(alerts) == 0
	if closing {
		resolved := r.mutedGroupAsResolved(ctx)
		if len(resolved) == 0 {
			return ctx, alerts, nil
		}
		alerts = resolved
	}

	r.metrics.numNotifications.WithLabelValues(r.labelValues...).Inc()

	ctx, span := tracer.Start(ctx, "notify.RetryStage.Exec",
		trace.WithAttributes(attribute.String("alerting.group.name", r.groupName)),
		trace.WithAttributes(attribute.String("alerting.integration.name", r.integration.name)),
		trace.WithAttributes(attribute.StringSlice("alerting.label.values", r.labelValues)),
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	ctx, alerts, failureReason, err := r.exec(ctx, l, closing, alerts...)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)

		r.metrics.numTotalFailedNotifications.WithLabelValues(append(r.labelValues, failureReason.String())...).Inc()
	}
	return ctx, alerts, err
}

// mutedGroupAsResolved returns the alerts of a group that muting has emptied, as
// resolved, for an integration whose mute_action asks to be told the group is
// over. It returns nothing on any flush but the one that closes the group: the
// notification sequence is closed by the flush that ends it and reports itself
// closed only then.
//
// Every muted alert is returned either way. Which close counts is what separates
// the two actions, and the two closes differ only in whether the group can still
// hold an alert that needs an end invented for it.
//
// Whether the receiver was ever notified about the group is not checked here.
func (r RetryStage) mutedGroupAsResolved(ctx context.Context) []*alert.Alert {
	seq, ok := NotificationSequenceFor(ctx)
	if !ok {
		return nil
	}

	switch seq {
	case SequenceClosedResolved:
		if !r.integration.SendsResolvedWhenMuted() {
			return nil
		}
		// The group closed because every alert in it resolved. The dedup stage
		// counts muted alerts among the firing ones, so reaching this close means
		// none is still firing and each already carries its own end.
		return mutedAlertDetails(ctx)

	case SequenceClosedMuted:
		if !r.integration.TreatsMuteAsResolved() {
			return nil
		}
		// The group closed because muting hid it while it was still firing, so it
		// can hold a mix: alerts that resolved out of sight keep their own end,
		// and those still firing are given the time the group went quiet.
		return endedAt(mutedAlertDetails(ctx), utcNow())

	default:
		return nil
	}
}

// endedAt returns alerts, with any that has not resolved replaced by a copy
// ending at end. The originals are left alone: they are still firing in the
// group, and only this integration is being told the group is over.
func endedAt(alerts []*alert.Alert, end time.Time) []*alert.Alert {
	out := make([]*alert.Alert, 0, len(alerts))
	for _, a := range alerts {
		if !a.Resolved() {
			ended := *a
			ended.EndsAt = end
			a = &ended
		}
		out = append(out, a)
	}
	return out
}

func (r RetryStage) exec(ctx context.Context, l *slog.Logger, closing bool, alerts ...*alert.Alert) (context.Context, []*alert.Alert, Reason, error) {
	var sent alert.AlertSlice

	// If we shouldn't send notifications for resolved alerts, and that leaves
	// nothing to send, report them all as successfully notified (we still want
	// the notification log to log them for the next run of DedupStage). What is
	// left is decided by the alerts in hand rather than by the firing alerts in
	// the context, which cover the whole group: with the muted alerts feature
	// those include alerts a mute stage removed from this pipeline.
	// The close of a muted group is delivered whole: the integration asked for it
	// with mute_action, so the send_resolved filter does not apply.
	if !closing && !r.integration.SendResolved() {
		if _, ok := FiringAlerts(ctx); !ok {
			return ctx, nil, DefaultReason, errors.New("firing alerts missing")
		}
		for _, a := range alerts {
			if a.Status() != model.AlertResolved {
				sent = append(sent, a)
			}
		}
		if len(sent) == 0 {
			return ctx, alerts, DefaultReason, nil
		}
	} else {
		sent = alerts
	}

	// backoff/v5's ExponentialBackOff never returns Stop from NextBackOff, so
	// we retry indefinitely until the context is canceled.
	b := backoff.NewExponentialBackOff()
	// Redundant for a freshly constructed ExponentialBackOff, but the BackOff
	// docs require Reset before first use.
	b.Reset()

	// Zero delay so the first attempt happens immediately.
	tick := time.NewTimer(0)
	defer tick.Stop()

	var (
		i       = 0
		iErr    error
		iReason Reason
	)

	l = l.With("receiver", r.groupName, "integration", r.integration.String())
	if groupKey, ok := GroupKey(ctx); ok {
		l = l.With("aggrGroup", groupKey)
	}

	for {

		// Always check the context first to not notify again.
		select {
		case <-ctx.Done():
			if iErr == nil {
				iErr = ctx.Err()
				if errors.Is(iErr, context.Canceled) {
					iReason = ContextCanceledReason
				} else if errors.Is(iErr, context.DeadlineExceeded) {
					iReason = ContextDeadlineExceededReason
				}
			}

			if iErr != nil {
				return ctx, nil, iReason, fmt.Errorf("%s/%s: notify retry canceled after %d attempts: %w", r.groupName, r.integration.String(), i, iErr)
			}
			return ctx, nil, DefaultReason, nil
		default:
		}

		select {
		case <-tick.C:
			now := time.Now()
			verdict := r.integration.Notify(ctx, sent...)
			i++
			dur := time.Since(now)
			// Back off from the end of the attempt, not from its start, so a
			// slow failure doesn't eat into the wait.
			tick.Reset(max(b.NextBackOff(), verdict.Delay()))
			r.metrics.notificationLatencySeconds.WithLabelValues(r.labelValues...).Observe(dur.Seconds())
			r.metrics.numNotificationRequestsTotal.WithLabelValues(r.labelValues...).Inc()
			if err := verdict.Err(); err != nil {
				r.metrics.numNotificationRequestsFailedTotal.WithLabelValues(r.labelValues...).Inc()
				if !verdict.ShouldRetry() {
					return ctx, alerts, verdict.Reason(), fmt.Errorf("%s/%s: notify retry canceled due to unrecoverable error after %d attempts: %w", r.groupName, r.integration.String(), i, err)
				}
				if ctx.Err() == nil {
					if iErr == nil || err.Error() != iErr.Error() {
						// Log the error if the context isn't done and the error isn't the same as before.
						l.Warn("Notify attempt failed, will retry later", "attempts", i, "err", err)
					}
					// Save this error to be able to return the last seen error by an
					// integration upon context timeout.
					iErr, iReason = err, verdict.Reason()
				}
			} else {
				l := l.With(
					"attempts", i,
					"duration", dur,
					"numAlerts", len(sent),
				)
				if i <= 1 {
					l.Debug("Notify success", "alerts", sent)
				} else {
					l.Info("Notify success")
				}

				r.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
					return newNotificationEvent(ctx, alerts, r.integration)
				})
				return ctx, alerts, DefaultReason, nil
			}
		case <-ctx.Done():
		}
	}
}
