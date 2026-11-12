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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// countingAlert records how often the span helpers read the alert name.
type countingAlert struct {
	nameCalls int
}

func (a *countingAlert) Name() string {
	a.nameCalls++
	return "TestAlert"
}

func (a *countingAlert) Fingerprint() model.Fingerprint { return 42 }

func alertSpanStarts(tracer Tracer, a *countingAlert) map[string]func(context.Context) trace.Span {
	return map[string]func(context.Context) trace.Span{
		"StartRouteAlert": func(ctx context.Context) trace.Span {
			_, span := tracer.StartRouteAlert(ctx, a)
			return span
		},
		"StartGroupAlert": func(ctx context.Context) trace.Span {
			_, span := tracer.StartGroupAlert(ctx, a, "receiver")
			return span
		},
		"StartAggregationGroupInsert": func(ctx context.Context) trace.Span {
			_, span := tracer.StartAggregationGroupInsert(ctx, a, "{}:{}")
			return span
		},
		"StartInhibitorProcessAlert": func(ctx context.Context) trace.Span {
			_, span := tracer.StartInhibitorProcessAlert(ctx, a)
			return span
		},
	}
}

// TestAlertSpansDisabledAreLazy checks that the alert-scoped span helpers
// neither allocate nor touch the alert while tracing is disabled.
func TestAlertSpansDisabledAreLazy(t *testing.T) {
	// AllocsPerRun changes GOMAXPROCS, so keep allocation tests sequential.
	tracingEnabled.Store(false)
	t.Cleanup(func() { tracingEnabled.Store(false) })

	tracer := NewTracer("test")
	ctx := context.Background()
	for name := range alertSpanStarts(tracer, nil) {
		t.Run(name, func(t *testing.T) {
			a := &countingAlert{}
			start := alertSpanStarts(tracer, a)[name]
			allocs := testing.AllocsPerRun(100, func() {
				start(ctx).End()
			})
			require.Zero(t, allocs)
			require.Zero(t, a.nameCalls)
		})
	}
}
