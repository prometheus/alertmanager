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
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
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

	// Start from DefaultOptions (clustering disabled by default, which is
	// essential when running multiple instances in one process) and only
	// override the per-test bits: paths, the ephemeral listener and the
	// injected dependencies.
	opts := DefaultOptions()
	opts.ConfigFile = configPath
	opts.DataDir = dir
	opts.WebConfig = &web.FlagConfig{
		WebListenAddresses: &addrs,
		WebSystemdSocket:   &systemd,
		WebConfigFile:      &webCfg,
	}
	opts.Logger = logger
	opts.Registerer = prometheus.NewRegistry()
	opts.Flagger = ff
	return opts
}

// waitHealthy blocks until the instance at addr serves /-/healthy with a
// 200, absorbing the brief window between Start returning and the serve
// goroutine accepting connections. It fails the test if the instance
// never becomes healthy.
func waitHealthy(t *testing.T, addr string) {
	t.Helper()
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
}

func TestApp_StartStop(t *testing.T) {
	a, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a.Start())

	addr := a.Addr()
	require.NotEmpty(t, addr, "Addr should be populated after Start")

	waitHealthy(t, addr)

	require.NoError(t, a.Stop(t.Context()))

	// Stop is idempotent.
	require.NoError(t, a.Stop(t.Context()))
}

func TestApp_ClusteredStartStop(t *testing.T) {
	// Bring up an instance with gossip clustering enabled so the
	// peer-dependent branches in setup (AddState/Join/Settle/
	// SetClusterPeer/clusterWait), the reloader's cluster-peer pipeline
	// wiring, and peer.Leave on shutdown are all exercised.
	opts := testOptions(t)
	opts.ClusterBindAddr = "127.0.0.1:0"

	a, err := New(opts)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	waitHealthy(t, a.Addr())

	require.NoError(t, a.Stop(t.Context()))
}

func TestApp_Start_BeforeNewFails(t *testing.T) {
	// Start on a zero-value App (no successful New) must error rather
	// than launch goroutines against a nil server/listeners.
	var a App
	require.Error(t, a.Start())
}

func TestApp_serveLoop(t *testing.T) {
	logger := promslog.NewNopLogger()

	t.Run("listener error is surfaced", func(t *testing.T) {
		a := &App{logger: logger, serveErrc: make(chan error, 1)}
		a.serveErrc <- errors.New("boom")
		err := a.serveLoop(context.Background())
		require.ErrorContains(t, err, "boom")
	})

	t.Run("clean serve goroutine exit", func(t *testing.T) {
		a := &App{logger: logger, serveErrc: make(chan error, 1)}
		close(a.serveErrc) // serve goroutine exited without an error
		require.NoError(t, a.serveLoop(context.Background()))
	})

	t.Run("context cancellation", func(t *testing.T) {
		a := &App{logger: logger, serveErrc: make(chan error, 1)}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, a.serveLoop(ctx))
	})
}

func TestApp_reloadRouterClosedReloadChannel(t *testing.T) {
	// A closed Options.Reload channel must not spin reloadRouter into a
	// hot loop calling coordinator.Reload on every iteration. We assert
	// this deterministically — no sleeps — by counting reloads: a closed
	// reloadCh must contribute zero, so after N synchronous /-/reload
	// round-trips the counter must equal exactly N. With the hot-loop
	// regression it would be far larger.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0o600))

	var reloads atomic.Int64
	coord := config.NewCoordinator(configPath, prometheus.NewRegistry(), promslog.NewNopLogger())
	coord.Subscribe(func(*config.Config) error {
		reloads.Add(1)
		return nil
	})

	reloadCh := make(chan struct{})
	close(reloadCh)

	a := &App{
		logger:      promslog.NewNopLogger(),
		opts:        Options{Reload: reloadCh},
		coordinator: coord,
		routerQuit:  make(chan struct{}),
		routerDone:  make(chan struct{}),
		webReload:   make(chan chan error),
	}

	go a.reloadRouter()

	// Drive N synchronous reloads through the /-/reload path. Each send
	// blocks until reloadRouter has run one coordinator.Reload, so these
	// are exact, ordered steps with no timing assumptions.
	const n = 3
	for range n {
		errc := make(chan error)
		a.webReload <- errc
		require.NoError(t, <-errc)
	}

	close(a.routerQuit)
	select {
	case <-a.routerDone:
	case <-time.After(time.Second):
		t.Fatal("reloadRouter did not exit after routerQuit closed")
	}

	// Read after routerDone (happens-before): the closed reloadCh must
	// have triggered no reloads of its own.
	require.Equal(t, int64(n), reloads.Load())
}

func TestApp_ConcurrentStartStop(t *testing.T) {
	// Regression: Start and Stop are serialized by a mutex so they cannot
	// interleave. Whatever the order, Stop must never close/receive on a
	// nil channel, and a Start losing the race to Stop must not leak its
	// goroutines (it observes stopped and declines to launch). Run several
	// iterations under -race to shake out the interleavings; the final
	// require.NoError below also fails the test if any goroutine is still
	// blocked on the router (it would deadlock the second Stop).
	for i := range 20 {
		a, err := New(testOptions(t))
		require.NoError(t, err, "iteration %d", i)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = a.Start()
		}()
		go func() {
			defer wg.Done()
			_ = a.Stop(t.Context())
		}()
		wg.Wait()

		// Stop is idempotent; the second call must return cleanly.
		require.NoError(t, a.Stop(t.Context()), "iteration %d", i)
	}
}

func TestApp_StartAfterStopFails(t *testing.T) {
	// Once Stop has run, Start must refuse to launch its goroutines (which
	// would otherwise be leaked, since the only thing that tears them down
	// has already run). This is the deterministic half of the
	// Start/Stop-race invariant exercised by TestApp_ConcurrentStartStop.
	a, err := New(testOptions(t))
	require.NoError(t, err)

	require.NoError(t, a.Stop(t.Context()))

	err = a.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "called after Stop")
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

func TestApp_Addrs(t *testing.T) {
	// Addrs reports every bound listener address, and Addr returns the
	// first of them. Bind two ephemeral ports to make sure both the
	// ordering and the count are honoured.
	opts := testOptions(t)
	addrs := []string{"127.0.0.1:0", "127.0.0.1:0"}
	opts.WebConfig.WebListenAddresses = &addrs

	a, err := New(opts)
	require.NoError(t, err)
	defer func() { _ = a.Stop(t.Context()) }()

	got := a.Addrs()
	require.Len(t, got, 2)
	for i, addr := range got {
		require.NotEmpty(t, addr, "Addrs()[%d] should be a concrete bound address", i)
		// Ephemeral ":0" requests must resolve to a concrete port.
		require.NotContains(t, addr, ":0", "Addrs()[%d] should not retain the :0 port", i)
	}
	require.Equal(t, got[0], a.Addr(), "Addr should equal the first bound address")
}

func TestApp_Reload(t *testing.T) {
	// The programmatic Reload re-reads the config through the coordinator
	// and must succeed for a valid, unchanged configuration.
	a, err := New(testOptions(t))
	require.NoError(t, err)
	defer func() { _ = a.Stop(t.Context()) }()

	require.NoError(t, a.Reload())
}

func TestApp_Reload_BeforeNewFails(t *testing.T) {
	// Calling Reload on a zero-value App (no successful New) must return
	// an error rather than panicking on the nil coordinator.
	var a App
	require.Error(t, a.Reload())
}

func TestApp_ReloadAfterStopFails(t *testing.T) {
	// Once Stop has run, Reload must refuse: a reload swaps in (and
	// starts) a new dispatcher/inhibitor, which would leak past a
	// completed Stop. The mtx + stopped guard turns that into an error.
	a, err := New(testOptions(t))
	require.NoError(t, err)
	require.NoError(t, a.Start())
	require.NoError(t, a.Stop(t.Context()))

	err = a.Reload()
	require.Error(t, err)
	require.Contains(t, err.Error(), "called after Stop")
}

func TestApp_Stop_AggregatesCleanupErrors(t *testing.T) {
	// Stop should run every teardown step even when some fail, and return
	// their errors joined together (named by step).
	a := &App{logger: promslog.NewNopLogger()}
	var order []string
	a.onStop("first", func() error {
		order = append(order, "first")
		return errors.New("boom-first")
	})
	a.onStop("second", func() error {
		order = append(order, "second")
		return nil
	})
	a.onStop("third", func() error {
		order = append(order, "third")
		return errors.New("boom-third")
	})

	err := a.Stop(context.Background())
	require.Error(t, err)
	// LIFO: third runs before second before first.
	require.Equal(t, []string{"third", "second", "first"}, order)
	require.ErrorContains(t, err, "third: boom-third")
	require.ErrorContains(t, err, "first: boom-first")
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

	// Wait until the listener is actually serving so a premature POST
	// can't fail with connection refused and masquerade as a deadlock.
	waitHealthy(t, a.Addr())

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
	// calls Stop. Stop must not block draining a.serveErrc because Start has
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
	// Exercises the Run wrapper end-to-end: it must serve, then return
	// nil (with cleanup run) once ctx is cancelled. Run is opaque (no
	// Addr), so we pin a concrete listen address and wait for the server
	// to actually answer /-/healthy before cancelling — a real readiness
	// signal instead of a sleep.
	opts := testOptions(t)
	addr := freeLoopbackAddr(t)
	addrs := []string{addr}
	opts.WebConfig.WebListenAddresses = &addrs

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()

	waitHealthy(t, addr)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

// freeLoopbackAddr reserves an ephemeral loopback port and returns it as a
// host:port string. The listener is closed before returning, so there is a
// small window before the caller rebinds it; this is the standard trade-off
// for tests that need to know an address up front (e.g. to poll readiness).
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}
