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

package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/api"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/tracing"
)

// newTestReloader builds a reloader backed by real (but local, cluster-
// disabled) collaborators, mirroring how setup wires it. It is the
// minimum needed to exercise reload/stop in isolation from HTTP.
func newTestReloader(t *testing.T) *reloader {
	t.Helper()

	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	ff, err := featurecontrol.NewFlags(logger, "")
	require.NoError(t, err)

	m := newMetrics(reg)
	rec := eventrecorder.NopRecorder()
	dir := t.TempDir()

	alerts, err := mem.NewAlerts(context.Background(), 30*time.Minute, 0, nil, logger, rec, reg, ff)
	require.NoError(t, err)
	t.Cleanup(alerts.Close)

	silences, err := silence.New(silence.Options{
		SnapshotFile: filepath.Join(dir, "silences"),
		Logger:       logger,
		Metrics:      reg,
	})
	require.NoError(t, err)
	silencer := silence.NewSilencer(silences, logger, rec)

	nflogger, err := nflog.New(nflog.Options{
		SnapshotFile: filepath.Join(dir, "nflog"),
		Logger:       logger,
		Metrics:      reg,
	})
	require.NoError(t, err)

	// The reloader owns the dispatcher/inhibitor; the API's GroupFunc
	// reads them through r, which is assigned just below (mirroring setup).
	var r *reloader

	apih, err := api.New(api.Options{
		Alerts:          alerts,
		Silences:        silences,
		Logger:          logger,
		Registry:        reg,
		RequestDuration: m.requestDuration,
		GroupFunc: func(ctx context.Context, rf func(*dispatch.Route) bool, af func(*alert.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error) {
			return r.groups(ctx, rf, af)
		},
	})
	require.NoError(t, err)

	extURL, err := url.Parse("http://localhost:9093")
	require.NoError(t, err)

	r = &reloader{
		logger:                      logger,
		alerts:                      alerts,
		silencer:                    silencer,
		notificationLog:             nflogger,
		eventRecorder:               rec,
		apih:                        apih,
		tracingMgr:                  tracing.NewManager(logger),
		pipelineBuilder:             notify.NewPipelineBuilder(reg, ff, rec),
		dispatcherMetrics:           dispatch.NewDispatcherMetrics(false, reg, ff),
		metrics:                     m,
		peer:                        nil,
		waitFunc:                    func() time.Duration { return 0 },
		timeoutFunc:                 func(d time.Duration) time.Duration { return d },
		externalURL:                 extURL,
		startTime:                   time.Now(),
		dispatchStartDelay:          0,
		dispatchMaintenanceInterval: 30 * time.Second,
		retention:                   120 * time.Hour,
	}
	return r
}

func mustConfig(t *testing.T) *config.Config {
	t.Helper()
	conf, err := config.Load(minimalConfig)
	require.NoError(t, err)
	return conf
}

func TestReloader_SwapsComponents(t *testing.T) {
	r := newTestReloader(t)
	t.Cleanup(func() { _ = r.stop() })

	// Initial apply installs a running inhibitor and dispatcher.
	require.NoError(t, r.reload(mustConfig(t)))
	dispatcher1 := r.dispatcher.Load()
	inh1 := r.inhibitor.Load()
	require.NotNil(t, dispatcher1)
	require.NotNil(t, inh1)

	// A second apply must stop the old pair and publish fresh instances.
	require.NoError(t, r.reload(mustConfig(t)))
	require.NotNil(t, r.dispatcher.Load())
	require.NotNil(t, r.inhibitor.Load())
	require.NotSame(t, dispatcher1, r.dispatcher.Load(), "dispatcher should be replaced on reload")
	require.NotSame(t, inh1, r.inhibitor.Load(), "inhibitor should be replaced on reload")
}

func TestReloader_ErrorLeavesPreviousStateIntact(t *testing.T) {
	r := newTestReloader(t)
	t.Cleanup(func() { _ = r.stop() })

	require.NoError(t, r.reload(mustConfig(t)))
	dispatcher1 := r.dispatcher.Load()
	inh1 := r.inhibitor.Load()

	// A template that fails to parse makes reload error out before it
	// swaps anything, so the previously active components stay in place.
	bad := filepath.Join(t.TempDir(), "bad.tmpl")
	require.NoError(t, os.WriteFile(bad, []byte("{{ .Foo "), 0o600))
	conf := mustConfig(t)
	conf.Templates = []string{bad}

	require.Error(t, r.reload(conf))
	require.Same(t, dispatcher1, r.dispatcher.Load(), "dispatcher must be unchanged after a failed reload")
	require.Same(t, inh1, r.inhibitor.Load(), "inhibitor must be unchanged after a failed reload")
}

// TestReloader_GroupMutedByFollowsConfig checks that whether a group is muted
// is always evaluated against the time intervals of the same configuration
// as the group's route, before and after a reload that renames the interval.
func TestReloader_GroupMutedByFollowsConfig(t *testing.T) {
	r := newTestReloader(t)
	t.Cleanup(func() { _ = r.stop() })

	const confTmpl = `
route:
  receiver: team-X
  group_by: ['alertname']
  group_wait: 1h
  routes:
  - receiver: team-X
    mute_time_intervals: ['%s']
receivers:
- name: team-X
time_intervals:
- name: %s
  time_intervals:
  - weekdays: ['monday:sunday']
`
	load := func(name string) *config.Config {
		conf, err := config.Load(fmt.Sprintf(confTmpl, name, name))
		require.NoError(t, err)
		return conf
	}
	mutedBy := func(t *testing.T) []string {
		t.Helper()
		var groups dispatch.AlertGroups
		// The dispatcher ingests alerts asynchronously after each reload.
		require.Eventually(t, func() bool {
			var err error
			groups, _, err = r.groups(context.Background(),
				func(*dispatch.Route) bool { return true },
				func(*alert.Alert, time.Time) bool { return true },
			)
			return err == nil && len(groups) == 1
		}, 5*time.Second, 10*time.Millisecond)
		return groups[0].MutedBy
	}

	require.NoError(t, r.reload(load("always_old")))
	now := time.Now()
	require.NoError(t, r.alerts.Put(context.Background(), alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "Foo"},
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Hour),
	}, now, false)))
	require.Equal(t, []string{"always_old"}, mutedBy(t))

	// The reload renames the interval. The new dispatcher resolves the
	// route's new interval name against the new definitions.
	require.NoError(t, r.reload(load("always_new")))
	require.Equal(t, []string{"always_new"}, mutedBy(t))
}

func TestReloader_StopIsNilSafe(t *testing.T) {
	r := newTestReloader(t)
	// stop before any reload (both pointers nil) must not panic.
	require.NoError(t, r.stop())
}
