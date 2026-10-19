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
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/provider/mem"
)

func TestDispatcherStopBeforeStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		logger := promslog.NewNopLogger()
		reg := prometheus.NewRegistry()
		alerts, err := mem.NewAlerts(context.Background(), time.Hour, 0, nil, logger, eventrecorder.NopRecorder(), reg, nil)
		require.NoError(t, err)
		t.Cleanup(alerts.Close)
		require.NoError(t, alerts.Put(context.Background(), newAlert(model.LabelSet{"alertname": "A"})))

		recorder := &recordStage{alerts: make(map[string]map[model.Fingerprint]*alert.Alert)}
		timeout := func(d time.Duration) time.Duration { return d }
		route := NewRoute(&config.Route{Receiver: "team"}, nil)
		dispatcher := NewDispatcher(alerts, route, recorder, marker.NewGroupMarker(), timeout, testMaintenanceInterval, nil, logger, eventrecorder.NopRecorder(), NewDispatcherMetrics(false, reg, nil), nil)
		go dispatcher.Run(time.Now().Add(time.Hour))
		dispatcher.WaitForLoading()
		synctest.Wait()

		before := time.Now()
		dispatcher.Stop()
		require.Equal(t, before, time.Now(), "Stop must not wait for the scheduled start")
		require.Empty(t, recorder.Alerts())
	})
}
