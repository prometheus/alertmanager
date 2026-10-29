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
	"errors"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var errRecorded = errors.New("recorded error")

type spanEvent struct {
	name  string
	attrs []attribute.KeyValue
}

// exceptionEvent is what the SDK records for RecordError: the caller's
// attributes followed by the exception type and message.
func exceptionEvent(attrs ...attribute.KeyValue) spanEvent {
	return spanEvent{"exception", append(attrs,
		attribute.String("exception.type", "*errors.errorString"),
		attribute.String("exception.message", errRecorded.Error()),
	)}
}

type spanRecordCase struct {
	name        string
	record      func(trace.Span)
	events      []spanEvent
	attrs       []attribute.KeyValue
	status      codes.Code
	description string
}

func spanRecordCases() []spanRecordCase {
	startsAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	return []spanRecordCase{
		{
			name:        "RecordError",
			record:      func(span trace.Span) { RecordError(span, errRecorded) },
			events:      []spanEvent{exceptionEvent()},
			status:      codes.Error,
			description: errRecorded.Error(),
		}, {
			name:        "RecordErrorWithStatus",
			record:      func(span trace.Span) { RecordErrorWithStatus(span, "failed", errRecorded) },
			events:      []spanEvent{exceptionEvent()},
			status:      codes.Error,
			description: "failed",
		}, {
			name:        "SetErrorStatus",
			record:      func(span trace.Span) { SetErrorStatus(span, "failed") },
			status:      codes.Error,
			description: "failed",
		}, {
			name:   "EventRouteMatched",
			record: func(span trace.Span) { EventRouteMatched(span, "team") },
			events: []spanEvent{{"dispatching alert to route", []attribute.KeyValue{
				attribute.String("alerting.route.receiver.name", "team"),
			}}},
		}, {
			name:   "RecordAggregationGroupLimit",
			record: func(span trace.Span) { RecordAggregationGroupLimit(span, errRecorded, 10, 5) },
			events: []spanEvent{exceptionEvent(
				attribute.Int("alerting.aggregation_group.count", 10),
				attribute.Int("alerting.aggregation_group.limit", 5),
			)},
			status:      codes.Error,
			description: "Failed to create aggregation group",
		}, {
			name:   "EventAggregationGroupCreated",
			record: func(span trace.Span) { EventAggregationGroupCreated(span, "{}:{}", 3) },
			events: []spanEvent{{"new AggregationGroup created", []attribute.KeyValue{
				attribute.String("alerting.aggregation_group.key", "{}:{}"),
				attribute.Int("alerting.aggregation_group.count", 3),
			}}},
		}, {
			name:   "EventImmediateFlush",
			record: func(span trace.Span) { EventImmediateFlush(span, startsAt) },
			events: []spanEvent{{"Alert is old enough for immediate flush, resetting timer to zero", []attribute.KeyValue{
				attribute.String("alerting.alert.StartsAt", "2026-10-27T12:00:00Z"),
			}}},
		}, {
			name:   "EventAggregationGroupWaiting",
			record: EventAggregationGroupWaiting,
			events: []spanEvent{{name: "Not starting Aggregation Group, dispatcher is not running"}},
		}, {
			name:   "EventAggregationGroupStarted",
			record: EventAggregationGroupStarted,
			events: []spanEvent{{name: "Starting Aggregation Group"}},
		}, {
			name:   "SetGroupKey",
			record: func(span trace.Span) { SetGroupKey(span, "{}:{}") },
			attrs:  []attribute.KeyValue{attribute.String("alerting.group.key", "{}:{}")},
		}, {
			name:   "SetMuted",
			record: func(span trace.Span) { SetMuted(span, 2, 3, "silence") },
			attrs: []attribute.KeyValue{
				attribute.Int("alerting.alerts.muted.count", 2),
				attribute.Int("alerting.alerts.filtered.count", 3),
				attribute.String("alerting.suppressed.reason", "silence"),
			},
		}, {
			name:   "EventTimeMuted",
			record: EventTimeMuted,
			events: []spanEvent{{name: "notify.TimeMuteStage.Exec muted the alerts"}},
		}, {
			name:   "EventTimeNotActive",
			record: EventTimeNotActive,
			events: []spanEvent{{name: "notify.TimeActiveStage.Exec not active, removing all alerts"}},
		}, {
			name:   "EventDedupNeedsUpdate",
			record: EventDedupNeedsUpdate,
			events: []spanEvent{{name: "notify.DedupStage.Exec nflog needs update"}},
		}, {
			name:   "SetNotifyRecoverable",
			record: func(span trace.Span) { SetNotifyRecoverable(span, true) },
			attrs:  []attribute.KeyValue{attribute.Bool("alerting.notify.error.recoverable", true)},
		}, {
			name:   "RecordNotifyFailure",
			record: func(span trace.Span) { RecordNotifyFailure(span, errRecorded, "clientError", "team", "slack", 1) },
			attrs: []attribute.KeyValue{
				attribute.String("alerting.notify.error.reason", "clientError"),
				attribute.String("alerting.notify.receiver.name", "team"),
				attribute.String("alerting.notify.integration.name", "slack"),
				attribute.Int("alerting.notify.integration.index", 1),
			},
			events:      []spanEvent{exceptionEvent()},
			status:      codes.Error,
			description: errRecorded.Error(),
		}, {
			name:   "SetNotifiesCounts",
			record: func(span trace.Span) { SetNotifiesCounts(span, 1, 2, 3) },
			attrs: []attribute.KeyValue{
				attribute.Int("alerting.alerts.firing.count", 1),
				attribute.Int("alerting.alerts.resolved.count", 2),
				attribute.Int("alerting.alerts.muted.count", 3),
			},
		}, {
			name:   "EventInhibitRuleSourceMatched",
			record: func(span trace.Span) { EventInhibitRuleSourceMatched(span, "rule") },
			events: []spanEvent{{"alert matched rule source", []attribute.KeyValue{
				attribute.String("alerting.inhibit_rule.name", "rule"),
			}}},
			attrs: []attribute.KeyValue{attribute.String("alerting.inhibit_rule.name", "rule")},
		}, {
			name:   "EventInhibitRuleTargetMatched",
			record: func(span trace.Span) { EventInhibitRuleTargetMatched(span, "rule") },
			events: []spanEvent{{"alert matched rule target", []attribute.KeyValue{
				attribute.String("alerting.inhibit_rule.name", "rule"),
			}}},
		}, {
			name:   "EventInhibited",
			record: func(span trace.Span) { EventInhibited(span, model.Fingerprint(42)) },
			events: []spanEvent{{"alert inhibited", []attribute.KeyValue{
				attribute.String("alerting.inhibit_rule.source.fingerprint", "000000000000002a"),
			}}},
		}, {
			name:   "EventNotInhibited",
			record: EventNotInhibited,
			events: []spanEvent{{name: "alert not inhibited"}},
		}, {
			name:   "EventSilencesCached",
			record: func(span trace.Span) { EventSilencesCached(span, 0) },
			events: []spanEvent{{"No new silences to match since last check", []attribute.KeyValue{
				attribute.Int("alerting.silences.cache.count", 0),
			}}},
		}, {
			name:   "EventNoSilences",
			record: func(span trace.Span) { EventNoSilences(span, 0) },
			events: []spanEvent{{"No silences to match", []attribute.KeyValue{
				attribute.Int("alerting.silences.count", 0),
			}}},
		}, {
			name:   "EventSilencerResult muted",
			record: func(span trace.Span) { EventSilencerResult(span, true, 1, 3) },
			events: []spanEvent{{"Silencer mutes alert", []attribute.KeyValue{
				attribute.Int("alerting.silences.active.count", 1),
				attribute.Int("alerting.silences.pending.count", 2),
				attribute.Int("alerting.silences.total.count", 3),
			}}},
		}, {
			name:   "EventSilencerResult not muted",
			record: func(span trace.Span) { EventSilencerResult(span, false, 0, 2) },
			events: []spanEvent{{"Silencer does not mute alert", []attribute.KeyValue{
				attribute.Int("alerting.silences.active.count", 0),
				attribute.Int("alerting.silences.pending.count", 2),
				attribute.Int("alerting.silences.total.count", 2),
			}}},
		},
	}
}

// TestSpanRecordHelpers checks what each helper records on a sampled span.
func TestSpanRecordHelpers(t *testing.T) {
	t.Parallel()

	for _, tc := range spanRecordCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

			_, span := provider.Tracer("test").Start(context.Background(), "test")
			tc.record(span)
			span.End()

			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			got := spans[0]
			require.Equal(t, tc.status, got.Status.Code)
			require.Equal(t, tc.description, got.Status.Description)
			require.ElementsMatch(t, tc.attrs, got.Attributes)
			require.Len(t, got.Events, len(tc.events))
			for i, want := range tc.events {
				require.Equal(t, want.name, got.Events[i].Name)
				require.ElementsMatch(t, want.attrs, got.Events[i].Attributes)
			}
		})
	}
}

// TestSpanRecordHelpersNotRecordingAreFree checks that the helpers do not
// allocate on the boxed noop span used while tracing is disabled, nor on a
// span the sampler dropped while tracing is enabled.
func TestSpanRecordHelpersNotRecordingAreFree(t *testing.T) {
	// AllocsPerRun changes GOMAXPROCS, so keep allocation tests sequential.
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	_, unsampled := provider.Tracer("test").Start(context.Background(), "test")
	t.Cleanup(func() { unsampled.End() })
	require.False(t, unsampled.IsRecording())

	spans := map[string]trace.Span{"noop": noopSpan, "unsampled": unsampled}
	for _, tc := range spanRecordCases() {
		for spanName, span := range spans {
			t.Run(tc.name+"/"+spanName, func(t *testing.T) {
				allocs := testing.AllocsPerRun(100, func() { tc.record(span) })
				require.Zero(t, allocs)
			})
		}
	}
}
