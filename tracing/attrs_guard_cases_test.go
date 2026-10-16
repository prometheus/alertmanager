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
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTracingAttributeGuard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		source     string
		importName string
		want       string
	}{
		{
			name:   "bare start",
			source: `func f() { tracer.Start(ctx, "span") }`,
		}, {
			name:   "attributes passed to start",
			source: `func f() { tracer.Start(ctx, "span", trace.WithAttributes(attr)) }`,
			want:   "WithAttributes",
		}, {
			name:   "renamed tracer",
			source: `func f() { other.Start(ctx, "span", trace.WithAttributes(attr)) }`,
			want:   "WithAttributes",
		}, {
			name:   "tracer field",
			source: `func f() { service.tracer.Start(ctx, "span", trace.WithAttributes(attr)) }`,
			want:   "WithAttributes",
		}, {
			name:   "option variable",
			source: `func f() { option := trace.WithAttributes(attr); tracer.Start(ctx, "span", option) }`,
			want:   "WithAttributes",
		}, {
			name: "option slice",
			source: `var options = []trace.SpanStartOption{trace.WithAttributes(attr)}
				func f() { tracer.Start(ctx, "span", options...) }`,
			want: "WithAttributes",
		}, {
			name: "aliased constructor",
			source: `var attrs = trace.WithAttributes
				func f() { tracer.Start(ctx, "span", attrs(attr)) }`,
			want: "WithAttributes",
		}, {
			name:       "aliased import",
			importName: "oteltrace",
			source:     `func f() { tracer.Start(ctx, "span", oteltrace.WithAttributes(attr)) }`,
			want:       "WithAttributes",
		}, {
			name:       "dot import",
			importName: ".",
			source:     `func f() { tracer.Start(ctx, "span", WithAttributes(attr)) }`,
			want:       "WithAttributes",
		}, {
			name:   "unrelated attributes",
			source: `func f() { other.Start(ctx, resource.WithAttributes(attr)) }`,
		}, {
			name:   "guarded span attributes",
			source: `func f() { if span.IsRecording() { span.SetAttributes(attr) } }`,
		}, {
			name:   "cached recording check",
			source: `func f() { recording := span.IsRecording(); if recording { span.SetAttributes(attr) } }`,
		}, {
			name:   "declared recording check",
			source: `func f() { var recording = span.IsRecording(); if recording { span.SetAttributes(attr) } }`,
		}, {
			name:   "recording check in if initializer",
			source: `func f() { if recording := span.IsRecording(); recording { span.SetAttributes(attr) } }`,
		}, {
			name:   "nested recording guard",
			source: `func f() { recording := span.IsRecording(); if recording { if ready { span.SetAttributes(attr) } } }`,
		}, {
			name:   "field receiver",
			source: `func f() { if service.span.IsRecording() { service.span.SetAttributes(attr) } }`,
		}, {
			name:   "conjunction",
			source: `func f() { if ready && span.IsRecording() { span.SetAttributes(attr) } }`,
		}, {
			name:   "unguarded attributes",
			source: `func f() { span.SetAttributes(attr) }`,
			want:   "recording guard",
		}, {
			name:   "other span recording",
			source: `func f() { if other.IsRecording() { span.SetAttributes(attr) } }`,
			want:   "recording guard",
		}, {
			name:   "negative check",
			source: `func f() { if !span.IsRecording() { span.SetAttributes(attr) } }`,
			want:   "recording guard",
		}, {
			name:   "else branch",
			source: `func f() { if span.IsRecording() {} else { span.SetAttributes(attr) } }`,
			want:   "recording guard",
		}, {
			name:   "disjunction",
			source: `func f() { if ready || span.IsRecording() { span.SetAttributes(attr) } }`,
			want:   "recording guard",
		}, {
			name:   "overwritten recording check",
			source: `func f() { recording := span.IsRecording(); recording = true; if recording { span.SetAttributes(attr) } }`,
			want:   "recording guard",
		}, {
			name:   "shadowed recording check",
			source: `func f() { recording := span.IsRecording(); if ready { recording := true; if recording { span.SetAttributes(attr) } } }`,
			want:   "recording guard",
		}, {
			name:   "guarded event attributes",
			source: `func f() { if span.IsRecording() { span.AddEvent("event", trace.WithAttributes(attr)) } }`,
		}, {
			name:   "guarded error attributes",
			source: `func f() { if span.IsRecording() { span.RecordError(err, trace.WithAttributes(attr)) } }`,
		}, {
			name:   "unguarded event attributes",
			source: `func f() { span.AddEvent("event", trace.WithAttributes(attr)) }`,
			want:   "recording guard",
		}, {
			name:   "event without attributes",
			source: `func f() { span.AddEvent("event") }`,
		}, {
			name: "cached span kind slice",
			source: `var options = []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindClient)}
				func f() { tracer.Start(ctx, "span", options...) }`,
		}, {
			name:   "inline span kind",
			source: `func f() { tracer.Start(ctx, "span", trace.WithSpanKind(trace.SpanKindClient)) }`,
			want:   "span kind options",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name := tc.importName
			if name == "" {
				name = "trace"
			}
			source := fmt.Sprintf("package example\nimport %s \"go.opentelemetry.io/otel/trace\"\n%s", name, tc.source)
			f, err := parser.ParseFile(token.NewFileSet(), "example.go", source, parser.SkipObjectResolution)
			require.NoError(t, err)
			violations := tracingAttributeViolations(f)
			if tc.want == "" {
				require.Empty(t, violations)
			} else {
				require.Len(t, violations, 1)
				require.Contains(t, violations[0].message, tc.want)
			}
		})
	}
}
