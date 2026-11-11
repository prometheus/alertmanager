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

package tracing

import (
	"time"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// This file holds everything callers record on a span after it has started:
// events, attributes, errors and status. Each helper takes plain values and
// builds attributes only once the span is known to be recording, so a call
// costs one method on the boxed noop span while tracing is disabled and
// nothing is formatted for spans the sampler dropped. Values that need
// formatting, such as fingerprints and timestamps, are passed raw so the
// formatting stays behind that check. Code outside this package never calls
// the span's own AddEvent, SetAttributes, RecordError or SetStatus.

// RecordError marks the span as failed with the error text as status
// description and records the error.
func RecordError(span trace.Span, err error) {
	if span.IsRecording() {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
}

// RecordErrorWithStatus marks the span as failed with message as status
// description and records the error.
func RecordErrorWithStatus(span trace.Span, message string, err error) {
	if span.IsRecording() {
		span.SetStatus(codes.Error, message)
		span.RecordError(err)
	}
}

// SetErrorStatus marks the span as failed with message as status description.
func SetErrorStatus(span trace.Span, message string) {
	if span.IsRecording() {
		span.SetStatus(codes.Error, message)
	}
}

// EventRouteMatched records that dispatch.Dispatcher.routeAlert matched a
// route and is handing the alert to its receiver.
func EventRouteMatched(span trace.Span, receiver string) {
	if span.IsRecording() {
		span.AddEvent("dispatching alert to route",
			trace.WithAttributes(attribute.String("alerting.route.receiver.name", receiver)),
		)
	}
}

// RecordAggregationGroupLimit records that dispatch.Dispatcher.groupAlert
// could not create an aggregation group because the limit was reached.
func RecordAggregationGroupLimit(span trace.Span, err error, groups, limit int) {
	if span.IsRecording() {
		span.SetStatus(codes.Error, "Failed to create aggregation group")
		span.RecordError(err,
			trace.WithAttributes(
				attribute.Int("alerting.aggregation_group.count", groups),
				attribute.Int("alerting.aggregation_group.limit", limit),
			),
		)
	}
}

// EventAggregationGroupCreated records that dispatch.Dispatcher.groupAlert
// created a new aggregation group.
func EventAggregationGroupCreated(span trace.Span, groupKey string, groups int) {
	if span.IsRecording() {
		span.AddEvent("new AggregationGroup created",
			trace.WithAttributes(
				attribute.String("alerting.aggregation_group.key", groupKey),
				attribute.Int("alerting.aggregation_group.count", groups),
			),
		)
	}
}

// EventImmediateFlush records that an alert was old enough for
// dispatch.Dispatcher.groupAlert to flush its new group right away.
func EventImmediateFlush(span trace.Span, startsAt time.Time) {
	if span.IsRecording() {
		span.AddEvent("Alert is old enough for immediate flush, resetting timer to zero",
			trace.WithAttributes(attribute.String("alerting.alert.StartsAt", startsAt.Format(time.RFC3339))),
		)
	}
}

// EventAggregationGroupWaiting records that a new aggregation group was not
// started because the dispatcher is still waiting to start.
func EventAggregationGroupWaiting(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("Not starting Aggregation Group, dispatcher is not running")
	}
}

// EventAggregationGroupStarted records that a new aggregation group was started.
func EventAggregationGroupStarted(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("Starting Aggregation Group")
	}
}

// SetGroupKey records the group key a notification stage is working on.
func SetGroupKey(span trace.Span, groupKey string) {
	if span.IsRecording() {
		span.SetAttributes(attribute.String("alerting.group.key", groupKey))
	}
}

// SetMuted records how many alerts notify.MuteStage.Exec muted and why.
func SetMuted(span trace.Span, muted, filtered int, reason string) {
	if span.IsRecording() {
		span.SetAttributes(
			attribute.Int("alerting.alerts.muted.count", muted),
			attribute.Int("alerting.alerts.filtered.count", filtered),
			attribute.String("alerting.suppressed.reason", reason),
		)
	}
}

// EventTimeMuted records that notify.TimeMuteStage.Exec muted all alerts.
func EventTimeMuted(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("notify.TimeMuteStage.Exec muted the alerts")
	}
}

// EventTimeNotActive records that notify.TimeActiveStage.Exec dropped all
// alerts because the route is outside its active times.
func EventTimeNotActive(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("notify.TimeActiveStage.Exec not active, removing all alerts")
	}
}

// EventDedupNeedsUpdate records that notify.DedupStage.Exec decided the
// notification log needs an update.
func EventDedupNeedsUpdate(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("notify.DedupStage.Exec nflog needs update")
	}
}

// SetNotifyRecoverable records whether a failed notify.Integration.Notify
// may be retried.
func SetNotifyRecoverable(span trace.Span, recoverable bool) {
	if span.IsRecording() {
		span.SetAttributes(attribute.Bool("alerting.notify.error.recoverable", recoverable))
	}
}

// RecordNotifyFailure records a failed notify.Integration.Notify.
func RecordNotifyFailure(span trace.Span, err error, reason, receiver, integration string, index int) {
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.notify.error.reason", reason),
			attribute.String("alerting.notify.receiver.name", receiver),
			attribute.String("alerting.notify.integration.name", integration),
			attribute.Int("alerting.notify.integration.index", index),
		)
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
}

// SetNotifiesCounts records the alert counts notify.SetNotifiesStage.Exec
// writes to the notification log.
func SetNotifiesCounts(span trace.Span, firing, resolved, muted int) {
	if span.IsRecording() {
		span.SetAttributes(
			attribute.Int("alerting.alerts.firing.count", firing),
			attribute.Int("alerting.alerts.resolved.count", resolved),
			attribute.Int("alerting.alerts.muted.count", muted),
		)
	}
}

// EventInhibitRuleSourceMatched records that inhibit.Inhibitor.processAlert
// found the alert to be a source for the named rule.
func EventInhibitRuleSourceMatched(span trace.Span, rule string) {
	if span.IsRecording() {
		attr := attribute.String("alerting.inhibit_rule.name", rule)
		span.AddEvent("alert matched rule source", trace.WithAttributes(attr))
		span.SetAttributes(attr)
	}
}

// EventInhibitRuleTargetMatched records that inhibit.Inhibitor.Mutes found
// the alert to be a target of the named rule.
func EventInhibitRuleTargetMatched(span trace.Span, rule string) {
	if span.IsRecording() {
		span.AddEvent("alert matched rule target",
			trace.WithAttributes(attribute.String("alerting.inhibit_rule.name", rule)),
		)
	}
}

// EventInhibited records that inhibit.Inhibitor.Mutes found a source alert
// that inhibits the alert.
func EventInhibited(span trace.Span, source model.Fingerprint) {
	if span.IsRecording() {
		span.AddEvent("alert inhibited",
			trace.WithAttributes(attribute.String("alerting.inhibit_rule.source.fingerprint", source.String())),
		)
	}
}

// EventNotInhibited records that inhibit.Inhibitor.Mutes found no inhibiting
// source alert.
func EventNotInhibited(span trace.Span) {
	if span.IsRecording() {
		span.AddEvent("alert not inhibited")
	}
}

// EventSilencesCached records that silence.Silencer.Mutes answered from its
// cache without querying silences.
func EventSilencesCached(span trace.Span, cached int) {
	if span.IsRecording() {
		span.AddEvent("No new silences to match since last check",
			trace.WithAttributes(attribute.Int("alerting.silences.cache.count", cached)),
		)
	}
}

// EventNoSilences records that silence.Silencer.Mutes found no active or
// pending silence for the alert.
func EventNoSilences(span trace.Span, total int) {
	if span.IsRecording() {
		span.AddEvent("No silences to match",
			trace.WithAttributes(attribute.Int("alerting.silences.count", total)),
		)
	}
}

// EventSilencerResult records whether silence.Silencer.Mutes muted the alert
// and how many silences it weighed.
func EventSilencerResult(span trace.Span, mutes bool, active, total int) {
	if span.IsRecording() {
		message := "Silencer does not mute alert"
		if mutes {
			message = "Silencer mutes alert"
		}
		span.AddEvent(message,
			trace.WithAttributes(
				attribute.Int("alerting.silences.active.count", active),
				attribute.Int("alerting.silences.pending.count", total-active),
				attribute.Int("alerting.silences.total.count", total),
			),
		)
	}
}
