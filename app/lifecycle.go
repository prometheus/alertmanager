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
	"net/http"
	"slices"
	"time"

	"github.com/prometheus/exporter-toolkit/web"
)

// shutdownTimeout bounds the graceful-teardown phase of Stop: the HTTP
// server drain plus the subsequent waits for the reload router and serve
// goroutine to exit. It applies even when the caller's context never
// cancels (e.g. context.Background()), so Stop always returns in bounded
// time before running the remaining cleanups.
const shutdownTimeout = 5 * time.Second

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
//
// It takes mtx for the duration of the reload so it cannot interleave with
// Stop: a reload swaps in (and starts) a new dispatcher/inhibitor, whereas
// Stop's cleanup tears the live ones down. Without this coupling a reload
// racing Stop could start a fresh dispatcher/inhibitor *after* Stop tore
// down the old ones, leaking those goroutines. Holding mtx also lets us
// refuse outright once Stop has begun. (The SIGHUP/HTTP reload paths route
// through reloadRouter, which Stop drains before running cleanups, so they
// are already safe; this guards the directly-callable entry point.)
func (a *App) Reload() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if a.coordinator == nil {
		return errors.New("alertmanager/app: App.Reload called before successful New")
	}
	if a.stopped {
		return errors.New("alertmanager/app: App.Reload called after Stop")
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
	// Bound the whole teardown by shutdownTimeout. Deriving it from ctx
	// lets a caller request a faster shutdown via a tighter deadline, but
	// the WithTimeout guarantees a finite bound even when ctx never
	// cancels on its own — e.g. the context.Background() passed by Run's
	// deferred Stop and by New's setup-failure rollback. Using it for the
	// waits below (not just Shutdown) is what keeps Stop from hanging on a
	// stuck reload or serve goroutine regardless of the caller's ctx.
	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	// Stop accepting new HTTP traffic first so in-flight handlers
	// don't observe collaborators being torn down underneath them. The
	// reload router is still running at this point so any in-flight
	// /-/reload handler can complete its send/receive cycle and unblock
	// Shutdown.
	if a.server != nil {
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			a.logger.Warn("graceful HTTP shutdown failed", "err", err)
			stopErr = err
		}
	}
	// HTTP is fully drained; no new /-/reload requests can arrive.
	// Terminate the reload router and wait for it to exit before
	// running cleanups (Coordinator is among them). The wait is bounded
	// in normal operation (the router exits as soon as routerQuit is
	// closed), but a stuck coordinator.Reload could block it, so we cap
	// it with shutdownCtx. Abandoning the goroutine on timeout is
	// acceptable — teardown is best-effort past this point — and we
	// surface it in the returned error.
	if started {
		close(a.routerQuit)
		select {
		case <-a.routerDone:
		case <-shutdownCtx.Done():
			a.logger.Warn("timed out waiting for reload router to exit; abandoning it", "err", shutdownCtx.Err())
			stopErr = errors.Join(stopErr, fmt.Errorf("reload router shutdown: %w", shutdownCtx.Err()))
		}
	}
	// Drain serveErrc so the listen goroutine, if any, exits before we
	// release listener resources. ServeMultiple returns once all
	// per-listener Serve calls return (which happens once Shutdown
	// completes), so this drain is bounded — but, as above, we also cap
	// it with shutdownCtx so Stop can't hang here either.
	//
	// Guard on `started` because serveErrc is allocated in New (so it can
	// be non-nil here) but only closed by Start's serve goroutine —
	// without this guard, Stop would deadlock when called from New's
	// rollback path on setup failure.
	if started && a.serveErrc != nil {
	drain:
		for {
			select {
			case _, ok := <-a.serveErrc:
				if !ok {
					break drain
				}
			case <-shutdownCtx.Done():
				a.logger.Warn("timed out draining serve errors; abandoning the serve goroutine", "err", shutdownCtx.Err())
				stopErr = errors.Join(stopErr, fmt.Errorf("serve drain: %w", shutdownCtx.Err()))
				break drain
			}
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
//
// The cleanups slice is mutated without locking, so registration is
// confined to the single-threaded construction phase (setup, called from
// New before the App is handed to the caller). It is therefore not safe to
// call once the App may be running, i.e. concurrently with Stop.
func (a *App) onStop(name string, fn func() error) {
	a.cleanups = append(a.cleanups, cleanup{name: name, stop: fn})
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
			// A listener error may have landed on serveErrc at the same
			// moment ctx was cancelled; select picks a ready case at
			// random, so it could choose ctx.Done() and mask the error.
			// Non-blocking drain to surface it rather than reporting a
			// clean shutdown over a real failure.
			select {
			case err, ok := <-a.serveErrc:
				if ok {
					return fmt.Errorf("alertmanager: HTTP listener failed: %w", err)
				}
			default:
			}
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
