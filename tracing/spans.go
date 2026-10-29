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
	"context"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// This file holds one Start method per span that carries attributes. The
// methods take plain values and only format them into attributes once the
// span is known to be recording, so callers pay nothing when tracing is
// disabled or the span is not sampled. Events, attributes and errors recorded
// later in a span's lifetime have their own helpers in events.go.

// Alert is the subset of alert.Alert that alert-scoped spans record. It
// lets the helpers read the alert name only once the span is recording,
// without importing Alertmanager types into this package. Implementations
// must be pointers: converting a pointer to this interface does not
// allocate, converting a struct value would.
type Alert interface {
	Name() string
	Fingerprint() model.Fingerprint
}

// Span kind options are built once: constructing them per call allocates.
var (
	clientSpanOptions   = []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindClient)}
	producerSpanOptions = []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindProducer)}
)

// StartRouteAlert starts the span for dispatch.Dispatcher.routeAlert.
func (t Tracer) StartRouteAlert(ctx context.Context, a Alert) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "dispatch.Dispatcher.routeAlert")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
		)
	}
	return ctx, span
}

// StartGroupAlert starts the span for dispatch.Dispatcher.groupAlert.
func (t Tracer) StartGroupAlert(ctx context.Context, a Alert, receiver string) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "dispatch.Dispatcher.groupAlert")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
			attribute.String("alerting.route.receiver.name", receiver),
		)
	}
	return ctx, span
}

// StartAggregationGroupFlush starts the span for dispatch.AggregationGroup.flush.
func (t Tracer) StartAggregationGroupFlush(ctx context.Context, groupKey string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "dispatch.AggregationGroup.flush")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.aggregation_group.key", groupKey),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartAggregationGroupInsert starts the span for dispatch.AggregationGroup.insert.
func (t Tracer) StartAggregationGroupInsert(ctx context.Context, a Alert, groupKey string) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "dispatch.AggregationGroup.insert")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
			attribute.String("alerting.aggregation_group.key", groupKey),
		)
	}
	return ctx, span
}

// StartIntegrationNotify starts the client span for notify.Integration.Notify.
func (t Tracer) StartIntegrationNotify(ctx context.Context, integration string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.Integration.Notify", clientSpanOptions...)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.notify.integration.name", integration),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartRoutingStage starts the span for notify.RoutingStage.Exec.
func (t Tracer) StartRoutingStage(ctx context.Context, receiver string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.RoutingStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.notify.receiver.name", receiver),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartMuteStage starts the span for notify.MuteStage.Exec.
func (t Tracer) StartMuteStage(ctx context.Context, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.MuteStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(attribute.Int("alerting.alerts.count", alerts))
	}
	return ctx, span
}

// StartTimeMuteStage starts the span for notify.TimeMuteStage.Exec.
func (t Tracer) StartTimeMuteStage(ctx context.Context, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.TimeMuteStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(attribute.Int("alerting.alerts.count", alerts))
	}
	return ctx, span
}

// StartTimeActiveStage starts the span for notify.TimeActiveStage.Exec.
func (t Tracer) StartTimeActiveStage(ctx context.Context, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.TimeActiveStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(attribute.Int("alerting.alerts.count", alerts))
	}
	return ctx, span
}

// StartDedupStage starts the span for notify.DedupStage.Exec.
func (t Tracer) StartDedupStage(ctx context.Context, groupKey string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.DedupStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.group.key", groupKey),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartRetryStage starts the span for notify.RetryStage.Exec.
func (t Tracer) StartRetryStage(ctx context.Context, groupName, integration string, labelValues []string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.RetryStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.group.name", groupName),
			attribute.String("alerting.integration.name", integration),
			attribute.StringSlice("alerting.label.values", labelValues),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartSetNotifiesStage starts the span for notify.SetNotifiesStage.Exec.
func (t Tracer) StartSetNotifiesStage(ctx context.Context, groupKey string, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "notify.SetNotifiesStage.Exec")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.group.key", groupKey),
			attribute.Int("alerting.alerts.count", alerts),
		)
	}
	return ctx, span
}

// StartInhibitorProcessAlert starts the span for inhibit.Inhibitor.processAlert.
func (t Tracer) StartInhibitorProcessAlert(ctx context.Context, a Alert) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "inhibit.Inhibitor.processAlert")
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("alerting.alert.name", a.Name()),
			attribute.String("alerting.alert.fingerprint", a.Fingerprint().String()),
		)
	}
	return ctx, span
}

// StartInhibitorMutes starts the span for inhibit.Inhibitor.Mutes.
func (t Tracer) StartInhibitorMutes(ctx context.Context, fp model.Fingerprint) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "inhibit.Inhibitor.Mutes")
	if span.IsRecording() {
		span.SetAttributes(attribute.String("alerting.alert.fingerprint", fp.String()))
	}
	return ctx, span
}

// StartSilencerMutes starts the span for silence.Silencer.Mutes.
func (t Tracer) StartSilencerMutes(ctx context.Context, fp model.Fingerprint) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "silence.Silencer.Mutes")
	if span.IsRecording() {
		span.SetAttributes(attribute.String("alerting.alert.fingerprint", fp.String()))
	}
	return ctx, span
}

// StartSilencesExpire starts the span for silences.Expire.
func (t Tracer) StartSilencesExpire(ctx context.Context, id string) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "silences.Expire")
	if span.IsRecording() {
		span.SetAttributes(attribute.String("alerting.silence.id", id))
	}
	return ctx, span
}

// StartAlertsPut starts the producer span for provider.mem.Put.
func (t Tracer) StartAlertsPut(ctx context.Context, alerts int) (context.Context, trace.Span) {
	ctx, span := t.Start(ctx, "provider.mem.Put", producerSpanOptions...)
	if span.IsRecording() {
		span.SetAttributes(attribute.Int("alerting.alerts.count", alerts))
	}
	return ctx, span
}
