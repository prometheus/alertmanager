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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/api"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider"
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

	groupMarker := marker.NewGroupMarker()

	nflogger, err := nflog.New(nflog.Options{
		SnapshotFile: filepath.Join(dir, "nflog"),
		Logger:       logger,
		Metrics:      reg,
	})
	require.NoError(t, err)

	apih, err := api.New(api.Options{
		Alerts:          alerts,
		Silences:        silences,
		GroupMutedFunc:  groupMarker.Muted,
		Logger:          logger,
		Registry:        reg,
		RequestDuration: m.requestDuration,
	})
	require.NoError(t, err)

	extURL, err := url.Parse("http://localhost:9093")
	require.NoError(t, err)

	r := &reloader{
		logger:                      logger,
		alerts:                      alerts,
		silencer:                    silencer,
		groupMarker:                 groupMarker,
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

func TestReloader_StopIsNilSafe(t *testing.T) {
	r := newTestReloader(t)
	// stop before any reload (both pointers nil) must not panic.
	require.NoError(t, r.stop())
}

// blockingAlerts wraps a provider.Alerts and can pause a single, targeted
// SlurpAndSubscribe call. Reload's new inhibitor/dispatcher block on this
// call while loading, so arming it for "inhibitor" lets a test land
// deterministically inside the window where the old (already-stopped)
// dispatcher/inhibitor are still active and reload has not yet published
// anything for the new config.
type blockingAlerts struct {
	provider.Alerts

	mu      sync.Mutex
	armed   bool
	name    string
	entered chan struct{}
	release chan struct{}
}

// arm primes the wrapper to block the next SlurpAndSubscribe(name) call.
// Entered is closed once that call is blocked. The caller must close
// release to let it proceed.
func (b *blockingAlerts) arm(name string) (entered <-chan struct{}, release chan<- struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.armed = true
	b.name = name
	b.entered = make(chan struct{})
	b.release = make(chan struct{})
	return b.entered, b.release
}

func (b *blockingAlerts) SlurpAndSubscribe(name string) ([]*alert.Alert, provider.AlertIterator) {
	b.mu.Lock()
	block := b.armed && b.name == name
	if block {
		b.armed = false
	}
	entered, release := b.entered, b.release
	b.mu.Unlock()

	if block {
		close(entered)
		<-release
	}
	return b.Alerts.SlurpAndSubscribe(name)
}

// TestReloader_ReloadWindowKeepsConfigAndDispatcherInhibitorConsistent covers
// the window in reload() while the new inhibitor/dispatcher are still
// loading: r.apih.Update is called only once, after both finish loading,
// and publishes config/alert-groups/status-prediction as a single atomic
// snapshot bound directly to the new dispatcher/inhibitor. So a concurrent
// request landing in that window must see config A everywhere (status,
// groups, predicted inhibition) rather than a torn mix of config B with
// config A's already-stopped dispatcher/inhibitor.
func TestReloader_ReloadWindowKeepsConfigAndDispatcherInhibitorConsistent(t *testing.T) {
	r := newTestReloader(t)
	t.Cleanup(func() { _ = r.stop() })

	blocking := &blockingAlerts{Alerts: r.alerts}
	r.alerts = blocking

	mux := r.apih.Register(route.New(), "")
	getStatus := func() string {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/status", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}
	// getGroups excludes inhibited alerts so the response distinguishes
	// config A (no inhibit rules, Bar included) from config B (Bar inhibited
	// by Foo, Bar excluded) through the same handler a real client would use.
	getGroups := func() models.AlertGroups {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/alerts/groups?inhibited=false", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var groups models.AlertGroups
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &groups))
		return groups
	}
	hasAlert := func(groups models.AlertGroups, alertname string) bool {
		for _, g := range groups {
			for _, a := range g.Alerts {
				if a.Labels["alertname"] == alertname {
					return true
				}
			}
		}
		return false
	}

	confA, err := config.Load(`route:
  receiver: receiver-a
  group_wait: 0s
  group_interval: 1s
  repeat_interval: 1h
receivers:
  - name: receiver-a
`)
	require.NoError(t, err)
	require.NoError(t, r.reload(confA))

	// Foo/Bar are only related by config B's inhibit rule; under config A
	// (no inhibit rules) Bar is never inhibited.
	now := time.Now()
	foo := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "Foo", "job": "x"},
		StartsAt: now,
		EndsAt:   now.Add(time.Hour),
	}, now, false)
	bar := alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "Bar", "job": "x"},
		StartsAt: now,
		EndsAt:   now.Add(time.Hour),
	}, now, false)
	require.NoError(t, r.alerts.Put(context.Background(), foo, bar))

	confB, err := config.Load(`route:
  receiver: receiver-b
  group_wait: 0s
  group_interval: 1s
  repeat_interval: 1h
receivers:
  - name: receiver-b
inhibit_rules:
  - source_matchers: ['alertname="Foo"']
    target_matchers: ['alertname="Bar"']
    equal: ['job']
`)
	require.NoError(t, err)

	entered, release := blocking.arm("inhibitor")

	reloadErr := make(chan error, 1)
	go func() { reloadErr <- r.reload(confB) }()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("reload did not reach the inhibitor loading window")
	}

	// Inside the window: r.dispatcher/r.inhibitor still reference config A's
	// stopped instances, and r.apih must not have published config B yet.
	staleDispatcher := r.dispatcher.Load()
	staleInhibitor := r.inhibitor.Load()
	require.NotNil(t, staleDispatcher)
	require.NotNil(t, staleInhibitor)

	status := getStatus()
	require.Contains(t, status, "receiver-a", "status must still report config A while its dispatcher/inhibitor are still active")
	require.NotContains(t, status, "receiver-b", "status must not publish config B before its dispatcher/inhibitor are live")

	groups := getGroups()
	require.NotEmpty(t, groups)
	for _, g := range groups {
		require.Equal(t, "receiver-a", *g.Receiver.Name, "groups must match the config currently published by the status endpoint")
	}
	require.True(t, hasAlert(groups, "Bar"),
		"config A has no inhibit rules, so Bar must still be reported when excluding inhibited alerts")

	// Let reload finish and swap in config B's dispatcher/inhibitor.
	close(release)
	require.NoError(t, <-reloadErr)

	require.NotSame(t, staleDispatcher, r.dispatcher.Load())
	require.NotSame(t, staleInhibitor, r.inhibitor.Load())

	status = getStatus()
	require.Contains(t, status, "receiver-b")

	groups = getGroups()
	require.NotEmpty(t, groups)
	for _, g := range groups {
		require.Equal(t, "receiver-b", *g.Receiver.Name)
	}
	require.False(t, hasAlert(groups, "Bar"),
		"config B's inhibit rule should suppress Bar once its inhibitor is live")
}
