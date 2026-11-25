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

package inhibit

import (
	"context"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/tracing"
)

func TestMutesTracingDisabledAllocations(t *testing.T) {
	// AllocsPerRun changes GOMAXPROCS, so keep allocation tests sequential.
	require.False(t, tracing.Enabled())
	var ih Inhibitor
	ctx := context.Background()
	lset := labelset.FromModel(model.LabelSet{"alertname": "TestAlert"})
	require.False(t, ih.Mutes(ctx, lset))
	allocs := testing.AllocsPerRun(100, func() {
		ih.Mutes(ctx, lset)
	})
	require.Zero(t, allocs)
}

func BenchmarkMutesTracingDisabled(b *testing.B) {
	require.False(b, tracing.Enabled())
	var ih Inhibitor
	ctx := context.Background()
	lset := labelset.FromModel(model.LabelSet{"alertname": "TestAlert"})
	b.ReportAllocs()
	for b.Loop() {
		ih.Mutes(ctx, lset)
	}
}
