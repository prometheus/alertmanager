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
	return newTestReloaderHook(t, nil)
}

func newTestReloaderHook(t *testing.T, beforeGroups func()) *reloader {
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

	// The reloader owns the dispatcher/inhibitor; the API's GroupFunc
	// reads them through r, which is assigned just below (mirroring setup).
	var r *reloader

	apih, err := api.New(api.Options{
		Alerts:          alerts,
		Silences:        silences,
		GroupMutedFunc:  groupMarker.Muted,
		Logger:          logger,
		Registry:        reg,
		RequestDuration: m.requestDuration,
		GroupFunc: func(ctx context.Context, rf func(*dispatch.Route) bool, af func(*alert.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error) {
			if beforeGroups != nil {
				beforeGroups()
			}
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

const inhibitConfig = `route:
  receiver: old
  group_by: ['alertname']
  group_wait: 1s
  group_interval: 1s
  repeat_interval: 1s
inhibit_rules:
  - source_matchers: [alertname="Source"]
    target_matchers: [alertname="Target"]
    equal: [cluster]
receivers:
  - name: old
`

const clearInhibitConfig = `route:
  receiver: fresh
  group_by: ['alertname']
  group_wait: 1s
  group_interval: 1s
  repeat_interval: 1s
receivers:
  - name: fresh
`

// gateAlerts blocks the second inhibitor subscription so a test can observe
// the reload while the new inhibitor has not finished loading.
type gateAlerts struct {
	provider.Alerts

	mu            sync.Mutex
	inhibitorSubs int
	entered       chan struct{}
	release       chan struct{}
	signal        sync.Once
}

func (g *gateAlerts) SlurpAndSubscribe(name string) ([]*alert.Alert, provider.AlertIterator) {
	if name == "inhibitor" {
		g.mu.Lock()
		g.inhibitorSubs++
		n := g.inhibitorSubs
		g.mu.Unlock()
		if n == 2 {
			g.signal.Do(func() { close(g.entered) })
			<-g.release
		}
	}
	return g.Alerts.SlurpAndSubscribe(name)
}

func loadConfig(t *testing.T, raw string) *config.Config {
	t.Helper()
	conf, err := config.Load(raw)
	require.NoError(t, err)
	return conf
}

func putPair(t *testing.T, alerts provider.Alerts) {
	t.Helper()
	now := time.Now()
	mk := func(name string) *alert.Alert {
		return alert.New(model.Alert{
			Labels:   model.LabelSet{"alertname": model.LabelValue(name), "cluster": "a"},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		}, now, false)
	}
	require.NoError(t, alerts.Put(context.Background(), mk("Source"), mk("Target")))
}

func apiMux(t *testing.T, r *reloader) http.Handler {
	t.Helper()
	return r.apih.Register(route.New(), "/")
}

func getJSON(t *testing.T, h http.Handler, path string) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.Bytes()
}

type listedAlert struct {
	Labels    map[string]string `json:"labels"`
	Receivers []struct {
		Name string `json:"name"`
	} `json:"receivers"`
	Status struct {
		InhibitedBy []string `json:"inhibitedBy"`
	} `json:"status"`
}

func findAlert(t *testing.T, body []byte, name string) listedAlert {
	t.Helper()
	var alerts []listedAlert
	require.NoError(t, json.Unmarshal(body, &alerts))
	for _, a := range alerts {
		if a.Labels["alertname"] == name {
			return a
		}
	}
	t.Fatalf("alert %s missing from %s", name, body)
	return listedAlert{}
}

func TestReloader_ReloadDoesNotPublishConfigBeforeInhibitor(t *testing.T) {
	t.Parallel()

	r := newTestReloader(t)
	t.Cleanup(func() { _ = r.stop() })
	gate := &gateAlerts{
		Alerts:  r.alerts,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate.release) }) }
	t.Cleanup(release)
	r.alerts = gate

	putPair(t, r.alerts)
	require.NoError(t, r.reload(loadConfig(t, inhibitConfig)))
	h := apiMux(t, r)

	before := findAlert(t, getJSON(t, h, "/api/v2/alerts"), "Target")
	require.Equal(t, "old", before.Receivers[0].Name)
	require.NotEmpty(t, before.Status.InhibitedBy)

	errc := make(chan error, 1)
	go func() {
		errc <- r.reload(loadConfig(t, clearInhibitConfig))
	}()
	select {
	case <-gate.entered:
	case err := <-errc:
		t.Fatalf("reload finished before the new inhibitor loaded: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the new inhibitor to subscribe")
	}

	during := findAlert(t, getJSON(t, h, "/api/v2/alerts"), "Target")
	require.Equal(t, "old", during.Receivers[0].Name, "API config changed before the new inhibitor was published")
	require.NotEmpty(t, during.Status.InhibitedBy, "in-progress reload dropped the previous inhibition rules")

	release()
	select {
	case err := <-errc:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reload")
	}

	after := findAlert(t, getJSON(t, h, "/api/v2/alerts"), "Target")
	require.Equal(t, "fresh", after.Receivers[0].Name)
	require.Empty(t, after.Status.InhibitedBy)
}

func TestReloader_InFlightRequestKeepsSnapshottedInhibitor(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	r := newTestReloaderHook(t, func() {
		once.Do(func() { close(entered) })
		<-release
	})
	t.Cleanup(func() { _ = r.stop() })
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(letGo)

	putPair(t, r.alerts)
	require.NoError(t, r.reload(loadConfig(t, inhibitConfig)))
	h := apiMux(t, r)

	type result struct {
		code int
		body string
	}
	got := make(chan result, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/alerts/groups?active=true&inhibited=false&silenced=true", nil)
		h.ServeHTTP(rec, req)
		got <- result{code: rec.Code, body: rec.Body.String()}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the in-flight request to snapshot the API callback")
	}

	require.NoError(t, r.reload(loadConfig(t, clearInhibitConfig)))
	letGo()

	var res result
	select {
	case res = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the in-flight request")
	}
	require.Equal(t, http.StatusOK, res.code, res.body)

	var groups []struct {
		Alerts []struct {
			Labels map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.body), &groups))
	var names []string
	for _, g := range groups {
		for _, a := range g.Alerts {
			names = append(names, a.Labels["alertname"])
		}
	}
	require.Contains(t, names, "Source")
	require.NotContains(t, names, "Target", "in-flight request used the new inhibitor with the previous API callback")
}
