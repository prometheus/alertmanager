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
	"sync"
	"sync/atomic"
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

	// srvc carries errors from the HTTP serve goroutine. It is closed
	// when the goroutine exits cleanly.
	srvc chan error

	// cleanups is the LIFO teardown stack: New (via setup) registers
	// cleanups in source order; Stop drains them in reverse so that
	// shutdown order mirrors the original `defer` chain in Run. Each
	// entry carries a name so Stop can log which step failed and return
	// an aggregated error.
	cleanups []cleanup

	startedOnce sync.Once
	startErr    error
	// started records whether the serve goroutine in Start was actually
	// launched. Stop uses this to decide whether draining a.srvc and
	// tearing down the reload router is meaningful — if Start never ran,
	// nothing will ever close srvc and the drain would deadlock (e.g.,
	// during setup-failure rollback).
	//
	// It is published with Store(true) only *after* routerQuit/routerDone
	// have been allocated (see Start). Because an atomic Load that
	// observes that Store also observes everything sequenced before it,
	// any Stop that sees started==true is guaranteed to see non-nil
	// channels — so it can never close/receive on a nil channel.
	started atomic.Bool

	// routerQuit signals the reload-routing goroutine (started by Start)
	// to exit; routerDone is closed by that goroutine on exit. Both are
	// allocated in Start before started is set and only read when
	// started.Load() is true.
	routerQuit chan struct{}
	routerDone chan struct{}

	stoppedOnce sync.Once
}

// New wires every Alertmanager subsystem according to opts but does not
// start serving HTTP yet. On error, partial setup is rolled back via the
// same cleanup stack that Stop would drain on success.
func New(opts Options) (*App, error) {
	a := &App{
		opts:      opts,
		srvc:      make(chan error, 1),
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
	a.startedOnce.Do(func() {
		if a.server == nil || len(a.listeners) == 0 {
			a.startErr = errors.New("alertmanager/app: App.Start called before successful New")
			return
		}

		// Allocate the router channels before publishing started. The
		// Store(true) below is the release that makes these writes
		// visible: a concurrent Stop that observes started==true via
		// Load is therefore guaranteed to see non-nil channels and can
		// never close/receive on nil. (If Stop instead observes
		// started==false it leaves the router untouched entirely.)
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
				a.srvc <- err
			}
			close(a.srvc)
		}()

		// Publish last: everything above happens-before this Store and is
		// thus visible to any Stop whose Load observes it.
		a.started.Store(true)
	})
	return a.startErr
}

// reloadRouter forwards reload triggers (HTTP /-/reload and opts.Reload)
// to the config coordinator until routerQuit closes. It is started by
// Start and stopped by Stop after the HTTP server has finished draining,
// so that any in-flight /-/reload handlers can complete their
// send/receive cycle through this goroutine.
func (a *App) reloadRouter() {
	defer close(a.routerDone)
	for {
		select {
		case <-a.routerQuit:
			return
		case <-a.opts.Reload:
			if err := a.coordinator.Reload(); err != nil {
				a.logger.Error("configuration reload failed", "err", err)
			}
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
// SIGHUP). Safe to call concurrently with the running App.
func (a *App) Reload(_ context.Context) error {
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
	var stopErr error
	a.stoppedOnce.Do(func() {
		// Snapshot the started flag once. The atomic Load pairs with the
		// Store at the end of Start: if it returns true, the
		// routerQuit/routerDone writes that were sequenced before that
		// Store are guaranteed visible here, so the channels are non-nil.
		started := a.started.Load()

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
		// Drain srvc so the listen goroutine, if any, exits before we
		// release listener resources. ServeMultiple returns once all
		// per-listener Serve calls return (which happens once Shutdown
		// completes), so this drain is bounded.
		//
		// Guard on `started` because srvc is allocated in New (so it can
		// be non-nil here) but only closed by Start's serve goroutine —
		// without this guard, Stop would deadlock when called from New's
		// rollback path on setup failure.
		if started && a.srvc != nil {
			for range a.srvc {
				// no-op
			}
		}
		// Run remaining cleanups in reverse-registration (LIFO) order,
		// mirroring Go's `defer` semantics so the in-place transform
		// from `defer X` to `a.onStop(X)` in setup is order-preserving.
		for i := len(a.cleanups) - 1; i >= 0; i-- {
			c := a.cleanups[i]
			if err := c.stop(); err != nil {
				a.logger.Warn("teardown step failed", "step", c.name, "err", err)
				stopErr = errors.Join(stopErr, fmt.Errorf("%s: %w", c.name, err))
			}
		}
	})
	return stopErr
}

// onStop registers a named teardown step to run when Stop is called.
// Cleanups run LIFO. fn should return an error only for failures worth
// surfacing to the caller; steps that cannot fail return nil.
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
		case err, ok := <-a.srvc:
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
