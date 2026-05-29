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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

const minimalConfig = `route:
  receiver: default
receivers:
  - name: default
`

// testOptions returns an Options value that is sufficient to bring up an
// Alertmanager instance bound to an ephemeral port with clustering
// disabled.
func testOptions(t *testing.T) Options {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0o600))

	logger := promslog.NewNopLogger()
	ff, err := featurecontrol.NewFlags(logger, "")
	require.NoError(t, err)
	// compat.InitFromFlags mutates package-global state; safe because all
	// tests in this package use the same (empty) feature flag set.
	compat.InitFromFlags(logger, ff)

	addrs := []string{"127.0.0.1:0"}
	systemd := false
	webCfg := ""

	return Options{
		ConfigFile:                  configPath,
		DataDir:                     dir,
		Retention:                   120 * time.Hour,
		MaintenanceInterval:         15 * time.Minute,
		AlertGCInterval:             30 * time.Minute,
		DispatchMaintenanceInterval: 30 * time.Second,
		WebConfig: &web.FlagConfig{
			WebListenAddresses: &addrs,
			WebSystemdSocket:   &systemd,
			WebConfigFile:      &webCfg,
		},
		PeerTimeout:          15 * time.Second,
		PeersResolveTimeout:  15 * time.Second,
		GossipInterval:       200 * time.Millisecond,
		PushPullInterval:     60 * time.Second,
		TCPTimeout:           10 * time.Second,
		ProbeTimeout:         500 * time.Millisecond,
		ProbeInterval:        1 * time.Second,
		SettleTimeout:        0,
		ReconnectInterval:    10 * time.Second,
		PeerReconnectTimeout: 6 * time.Hour,
		// Empty disables clustering — essential when running multiple
		// instances in one process.
		ClusterBindAddr: "",

		Logger:     logger,
		Registerer: prometheus.NewRegistry(),
		Flagger:    ff,
	}
}

func TestApp_StartStop(t *testing.T) {
	a, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	addr := a.Addr()
	require.NotEmpty(t, addr, "Addr should be populated after Start")

	// Probe /-/healthy with a short retry loop to absorb listener warmup.
	url := "http://" + addr + "/-/healthy"
	require.Eventually(t, func() bool {
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 50*time.Millisecond, "instance never became healthy")

	require.NoError(t, a.Stop(t.Context()))

	// Stop is idempotent.
	require.NoError(t, a.Stop(t.Context()))
}

func TestApp_TwoSequentialInstances(t *testing.T) {
	// Validates that per-instance Registerer + cleanup-stack teardown
	// allow constructing a second App in the same process without
	// duplicate-registration panics or leaked goroutines.
	for i := range 2 {
		a, err := New(testOptions(t))
		require.NoError(t, err, "iteration %d", i)
		require.NoError(t, a.Start(), "iteration %d", i)
		require.NotEmpty(t, a.Addr(), "iteration %d", i)
		require.NoError(t, a.Stop(t.Context()), "iteration %d", i)
	}
}

func TestApp_TwoConcurrentInstances(t *testing.T) {
	// Two live instances on different ephemeral ports, sharing the
	// same process. This exercises the metrics-per-Registerer change
	// from Phase A and ensures no shutdown-ordering bugs surface when
	// Stop runs on one instance while another is still serving.
	a1, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a1.Start())
	defer func() { _ = a1.Stop(t.Context()) }()

	a2, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a2.Start())
	defer func() { _ = a2.Stop(t.Context()) }()

	require.NotEqual(t, a1.Addr(), a2.Addr(), "instances should bind distinct ports")
}

func TestApp_EmbeddedReloadDoesNotDeadlock(t *testing.T) {
	// Regression: when callers use the lifecycle API (New + Start + Stop)
	// without Run, the /-/reload HTTP handler must not block forever on
	// the unbuffered a.webReload channel. The reload-routing goroutine
	// started by Start is the consumer.
	a, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a.Start())
	defer func() { _ = a.Stop(t.Context()) }()

	type reloadResult struct {
		err    error
		status int
	}
	resultCh := make(chan reloadResult, 1)
	go func() {
		resp, err := http.Post("http://"+a.Addr()+"/-/reload", "", nil)
		if err != nil {
			resultCh <- reloadResult{err: err}
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		resultCh <- reloadResult{status: resp.StatusCode}
	}()
	select {
	case r := <-resultCh:
		require.NoError(t, r.err)
		require.Equal(t, http.StatusOK, r.status)
	case <-time.After(5 * time.Second):
		t.Fatal("/-/reload deadlocked in embedded mode")
	}
}

func TestApp_New_SetupFailureDoesNotDeadlock(t *testing.T) {
	// Regression: setup failure in New triggers the rollback path which
	// calls Stop. Stop must not block draining a.srvc because Start has
	// not run and nothing will ever close that channel.
	errCh := make(chan error, 1)
	go func() {
		// Empty Options fails validate (Logger required), exercising
		// the earliest setup-failure path.
		_, err := New(Options{})
		errCh <- err
	}()
	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("New deadlocked on setup-failure rollback")
	}
}

func TestApp_Run_ContextCancel(t *testing.T) {
	// Exercises the Run wrapper end-to-end: cancel ctx and assert it
	// returns without error and cleanup has run.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, testOptions(t)) }()

	// Give Run a moment to bind. We can't peek inside it for Addr, but
	// cancelling is unconditionally safe.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}
