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

package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/provider/mem"
)

// TestAlertsMetricGroupKeyLabels checks that, with group-key-in-metrics
// enabled, alertmanager_alerts carries both the opaque group_key and the
// human readable group_path for every aggregation group.
func TestAlertsMetricGroupKeyLabels(t *testing.T) {
	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	ff, err := featurecontrol.NewFlags(logger, featurecontrol.FeatureGroupKeyInMetrics)
	require.NoError(t, err)

	alerts, err := mem.NewAlerts(context.Background(), time.Hour, 0, nil, logger, eventrecorder.NopRecorder(), reg, nil)
	require.NoError(t, err)
	defer alerts.Close()

	route := &Route{
		RouteOpts: RouteOpts{
			Receiver:       "default",
			GroupBy:        map[model.LabelName]struct{}{"alertname": {}},
			GroupWait:      0,
			GroupInterval:  time.Hour,
			RepeatInterval: time.Hour,
		},
	}
	timeout := func(d time.Duration) time.Duration { return d }
	recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*alert.Alert)}
	dispatcher := NewDispatcher(alerts, route, recorder, marker.NewGroupMarker(), timeout, testMaintenanceInterval, nil, logger, eventrecorder.NopRecorder(), NewDispatcherMetrics(false, reg, ff), nil)
	go dispatcher.Run(time.Now())
	defer dispatcher.Stop()

	labels := model.LabelSet{"alertname": "TestAlert"}
	require.NoError(t, alerts.Put(context.Background(), newAlert(labels)))
	require.Eventually(t, func() bool { return len(recorder.Alerts()) == 1 }, 5*time.Second, 10*time.Millisecond)

	families, err := reg.Gather()
	require.NoError(t, err)
	var fam *dto.MetricFamily
	for _, f := range families {
		if f.GetName() == "alertmanager_alerts" {
			fam = f
		}
	}
	require.NotNil(t, fam, "alertmanager_alerts not found")
	require.NotEmpty(t, fam.GetMetric())

	wantPath := `{}:{alertname="TestAlert"}`
	wantKey := keyHash(wantPath, "default")
	for _, m := range fam.GetMetric() {
		got := map[string]string{}
		for _, lp := range m.GetLabel() {
			got[lp.GetName()] = lp.GetValue()
		}
		require.Contains(t, got, "state")
		require.Equal(t, wantKey, got["group_key"])
		require.Equal(t, wantPath, got["group_path"])
	}
}
