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

package mem

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/tracing"
)

func newTracingTestAlerts(t testing.TB) *Alerts {
	t.Helper()
	require.False(t, tracing.Enabled())
	alerts, err := NewAlerts(context.Background(), time.Hour, 0, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	require.NoError(t, err)
	t.Cleanup(alerts.Close)
	return alerts
}

func TestPutTracingDisabledAllocations(t *testing.T) {
	// AllocsPerRun changes GOMAXPROCS, so keep allocation tests sequential.
	alerts := newTracingTestAlerts(t)
	ctx := context.Background()
	require.NoError(t, alerts.Put(ctx))
	// An empty batch exercises the producer span without alert storage costs.
	allocs := testing.AllocsPerRun(100, func() {
		_ = alerts.Put(ctx)
	})
	require.Zero(t, allocs)
}

func BenchmarkPutTracingDisabled(b *testing.B) {
	alerts := newTracingTestAlerts(b)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_ = alerts.Put(ctx)
	}
}
