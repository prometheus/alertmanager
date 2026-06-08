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
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/exporter-toolkit/web"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/tracing"
)

// shutdownTimeout bounds how long Stop waits for the HTTP server to drain
// in-flight requests before forcing teardown.
const shutdownTimeout = 5 * time.Second

// App is a running (or runnable) Alertmanager instance built from Options.
//
// Compared to the top-level Run function, App exposes lifecycle hooks
// (Start, Stop, Addr, Reload) so callers — typically tests — can drive an
// instance without OS signals and discover the actually-bound HTTP
// address (useful when listening on ":0").
//
// Construct an App with New, then call Start to begin serving HTTP. The
// caller is responsible for calling Stop, ideally via a deferred call so
// teardown also runs on panic. An App is single-use: calling Start more
// than once is an error.
type App struct {
	opts   Options
	logger *slog.Logger

	// Lifecycle dependencies retained for use by Start, Reload, and Stop.
	coordinator *config.Coordinator
	tracingMgr  *tracing.Manager
	server      *http.Server
	listeners   []net.Listener

	// webReload is the channel exposed by httpserver.Register for the
	// /-/reload HTTP endpoint. We read from it in reloadRouter.
	webReload chan chan error

	// serveErrc carries errors from the HTTP serve goroutine. It is closed
	// when the goroutine exits cleanly.
	serveErrc chan error

	// cleanups is the LIFO teardown stack: New (via setup) registers
	// cleanups in source order; Stop drains them in reverse so that
	// shutdown order mirrors the original `defer` chain in Run. Each
	// entry carries a name so Stop can log which step failed and return
	// an aggregated error.
	cleanups []cleanup

	// mtx serializes Start and Stop so they cannot interleave. An atomic
	// flag alone is insufficient: a Stop that observed started==false
	// while a concurrent Start had already launched its goroutines (but
	// not yet recorded the fact) would skip tearing them down and leak
	// them. Holding mtx for the whole body of each method instead means a
	// Start racing a Stop either runs entirely before Stop — and is then
	// torn down by it — or observes stopped and declines to launch
	// anything at all. mtx also guards started, stopped, startErr, stopErr
	// and the router channels below.
	mtx sync.Mutex

	// started records whether Start launched the serve/reload goroutines;
	// stopped records whether Stop has run. Stop uses started to decide
	// whether draining serveErrc and tearing down the reload router is
	// meaningful — if Start never ran, nothing will ever close serveErrc and
	// the drain would deadlock (e.g. during setup-failure rollback).
	// Start uses stopped to refuse to launch goroutines after a Stop.
	started bool
	stopped bool

	// startErr/stopErr memoise the outcome of the first Start/Stop so
	// repeated calls are idempotent and return the same result.
	startErr error
	stopErr  error

	// routerQuit signals the reload-routing goroutine (started by Start)
	// to exit; routerDone is closed by that goroutine on exit. Both are
	// allocated under mtx in Start and only read under mtx in Stop.
	routerQuit chan struct{}
	routerDone chan struct{}
}

// New wires every Alertmanager subsystem according to opts but does not
// start serving HTTP yet. On error, partial setup is rolled back via the
// same cleanup stack that Stop would drain on success.
func New(opts Options) (*App, error) {
	a := &App{
		opts:      opts,
		serveErrc: make(chan error, 1),
		webReload: make(chan chan error),
	}
	if err := a.setup(); err != nil {
		// Roll back partial setup (Stop is idempotent and nil-safe).
		_ = a.Stop(context.Background())
		return nil, err
	}
	return a, nil
}

// Start begins serving HTTP traffic on the listeners established by New.
// It returns immediately; the listen goroutine signals any error via the
// channel drained by serveLoop. Subsequent calls are no-ops.
func (a *App) Start() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if a.started {
		// Idempotent: report the outcome of the first call.
		return a.startErr
	}
	if a.stopped {
		return errors.New("alertmanager/app: App.Start called after Stop")
	}
	if a.server == nil || len(a.listeners) == 0 {
		a.startErr = errors.New("alertmanager/app: App.Start called before successful New")
		return a.startErr
	}

	a.routerQuit = make(chan struct{})
	a.routerDone = make(chan struct{})

	// reloadRouter consumes /-/reload requests and opts.Reload sends so
	// they trigger reloads regardless of whether the caller is using
	// Run (which also runs serveLoop) or the lifecycle API directly
	// (which doesn't). Without this goroutine the /-/reload HTTP
	// handler would block forever in embedded mode because its send
	// on an unbuffered channel has no receiver.
	go a.reloadRouter()

	go func() {
		err := web.ServeMultiple(a.listeners, a.server, a.opts.WebConfig, a.logger)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("Listen error", "err", err)
			a.serveErrc <- err
		}
		close(a.serveErrc)
	}()

	a.started = true
	return nil
}

// reloadRouter forwards reload triggers (HTTP /-/reload and opts.Reload)
// to the config coordinator until routerQuit closes. It is started by
// Start and stopped by Stop after the HTTP server has finished draining,
// so that any in-flight /-/reload handlers can complete their
// send/receive cycle through this goroutine.
func (a *App) reloadRouter() {
	defer close(a.routerDone)
	// Copy opts.Reload into a local so the select below can nil it out to
	// disable that case if an embedder closes it (see the comment there).
	reloadCh := a.opts.Reload
	for {
		select {
		case <-a.routerQuit:
			return
		// opts.Reload is the fire-and-forget trigger (SIGHUP in the
		// binary, or a programmatic send by an embedder). There is no
		// caller waiting for a result, so reload errors are only logged.
		// The channel is embedder-owned and may be closed; the comma-ok
		// detects that (a closed channel always reads ready and would
		// hot-loop) and disables just this case by nil-ing reloadCh.
		case _, ok := <-reloadCh:
			if !ok {
				reloadCh = nil
				continue
			}
			if err := a.coordinator.Reload(); err != nil {
				a.logger.Error("configuration reload failed", "err", err)
			}
		// webReload is the request/response trigger from the /-/reload
		// HTTP handler: it sends a reply channel and blocks for the
		// outcome, so we propagate the reload error back over errc
		// instead of logging it. This channel is App-owned (allocated in
		// New, never closed externally), so it needs no comma-ok guard.
		case errc := <-a.webReload:
			errc <- a.coordinator.Reload()
		}
	}
}

// Addr returns the address of the first bound listener, suitable for
// dialing a single-listener instance (the common case for tests that
// bind ":0"). Use Addrs if configured with multiple listen addresses.
func (a *App) Addr() string {
	if len(a.listeners) == 0 {
		return ""
	}
	return a.listeners[0].Addr().String()
}

// Addrs returns all bound listener addresses in the order given by
// Options.WebConfig.WebListenAddresses.
func (a *App) Addrs() []string {
	out := make([]string, len(a.listeners))
	for i, l := range a.listeners {
		out[i] = l.Addr().String()
	}
	return out
}

// Reload triggers a configuration reload (the programmatic equivalent of
// SIGHUP). Safe to call concurrently with the running App. The reload is
// synchronous and not cancellable, so it takes no context.
func (a *App) Reload() error {
	if a.coordinator == nil {
		return errors.New("alertmanager/app: App.Reload called before successful New")
	}
	return a.coordinator.Reload()
}

// cleanup is a single named teardown step on the LIFO shutdown stack.
// The name is used purely for logging so operators can see which step
// failed during shutdown.
type cleanup struct {
	name string
	stop func() error
}

// Stop gracefully shuts down the App, draining cleanups in reverse
// registration order so that teardown ordering matches the original
// defer chain in Run. Safe to call multiple times; safe to call before
// Start (it will then merely roll back what setup registered).
//
// It returns an aggregated error combining the graceful HTTP shutdown
// failure (if any) with any errors returned by the teardown steps. Each
// failing step is also logged with its name; one failing step does not
// prevent the others from running.
func (a *App) Stop(ctx context.Context) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if a.stopped {
		// Idempotent: report the outcome of the first call.
		return a.stopErr
	}
	a.stopped = true

	// started is read under mtx, paired with the write in Start: holding
	// mtx across both bodies guarantees we observe a consistent view (and
	// that no Start can launch goroutines after this point).
	started := a.started

	var stopErr error
	// Stop accepting new HTTP traffic first so in-flight handlers
	// don't observe collaborators being torn down underneath them.
	// shutdownTimeout is derived from ctx so callers can request a
	// faster shutdown via a tighter deadline. The reload router is
	// still running at this point so any in-flight /-/reload handler
	// can complete its send/receive cycle and unblock Shutdown.
	if a.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			a.logger.Warn("graceful HTTP shutdown failed", "err", err)
			stopErr = err
		}
	}
	// HTTP is fully drained; no new /-/reload requests can arrive.
	// Terminate the reload router and wait for it to exit before
	// running cleanups (Coordinator is among them).
	if started {
		close(a.routerQuit)
		<-a.routerDone
	}
	// Drain serveErrc so the listen goroutine, if any, exits before we
	// release listener resources. ServeMultiple returns once all
	// per-listener Serve calls return (which happens once Shutdown
	// completes), so this drain is bounded.
	//
	// Guard on `started` because serveErrc is allocated in New (so it can
	// be non-nil here) but only closed by Start's serve goroutine —
	// without this guard, Stop would deadlock when called from New's
	// rollback path on setup failure.
	if started && a.serveErrc != nil {
		for range a.serveErrc {
			// no-op
		}
	}
	// Run remaining cleanups in reverse-registration (LIFO) order,
	// mirroring Go's `defer` semantics so the in-place transform
	// from `defer X` to `a.onStop(X)` in setup is order-preserving.
	for _, c := range slices.Backward(a.cleanups) {
		if err := c.stop(); err != nil {
			a.logger.Warn("teardown step failed", "step", c.name, "err", err)
			stopErr = errors.Join(stopErr, fmt.Errorf("%s: %w", c.name, err))
		}
	}
	a.stopErr = stopErr
	return stopErr
}

// onStop registers a named teardown step to run when Stop is called.
// Cleanups run in LIFO order. Steps return an error only for failures
// worth surfacing to the caller; those that cannot fail return nil.
func (a *App) onStop(name string, fn func() error) {
	a.cleanups = append(a.cleanups, cleanup{name: name, stop: fn})
}

// trackClose couples a resource with its teardown: it registers c.Close
// as a named Stop step right where the resource is acquired, so the two
// can't drift apart (the gap that previously leaked goroutines/handles).
// Use it for resources whose Close takes no arguments; everything else
// uses onStop directly.
func (a *App) trackClose(name string, c interface{ Close() error }) {
	a.onStop(name, c.Close)
}

// serveLoop blocks until ctx is cancelled or an HTTP listener fails. It
// is used by Run only; reload routing is handled by reloadRouter, which
// is started directly from Start so it is also active for embedders that
// drive the App lifecycle without using Run.
func (a *App) serveLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("Shutting down gracefully")
			return nil
		case err, ok := <-a.serveErrc:
			if !ok {
				// Channel closed without an error report — the serve
				// goroutine exited cleanly (ErrServerClosed). Treat
				// this as graceful shutdown.
				return nil
			}
			return fmt.Errorf("alertmanager: HTTP listener failed: %w", err)
		}
	}
}
