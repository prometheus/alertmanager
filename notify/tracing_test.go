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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/tracing"
)

func integrationTracingVerdicts() []struct {
	name    string
	verdict NotifyVerdict
} {
	return []struct {
		name    string
		verdict NotifyVerdict
	}{
		{name: "success", verdict: Success()},
		{name: "failure", verdict: Retry(0, errors.New("request failed"), RequestErrorReason)},
	}
}

func tracingTestIntegration(verdict NotifyVerdict) Integration {
	n := notifierFunc(func(context.Context, ...*alert.Alert) NotifyVerdict {
		return verdict
	})
	return NewIntegration(n, sendResolved(true), "test", 0, "receiver")
}

func TestIntegrationTracingDisabledAllocations(t *testing.T) {
	// AllocsPerRun changes GOMAXPROCS, so keep allocation tests sequential.
	require.False(t, tracing.Enabled())
	for _, tc := range integrationTracingVerdicts() {
		t.Run(tc.name, func(t *testing.T) {
			integration := tracingTestIntegration(tc.verdict)
			ctx := context.Background()
			require.Equal(t, tc.verdict, integration.Notify(ctx))
			allocs := testing.AllocsPerRun(100, func() {
				integration.Notify(ctx)
			})
			require.Zero(t, allocs)
		})
	}
}

func BenchmarkIntegrationTracingDisabled(b *testing.B) {
	require.False(b, tracing.Enabled())
	for _, tc := range integrationTracingVerdicts() {
		b.Run(tc.name, func(b *testing.B) {
			integration := tracingTestIntegration(tc.verdict)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				integration.Notify(ctx)
			}
		})
	}
}
