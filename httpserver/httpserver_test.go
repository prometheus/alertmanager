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

package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

// serveReload runs the reload handler in a separate goroutine and returns
// the recorder plus a channel closed when the handler returns.
func serveReload(ctx context.Context, router *route.Router) (*httptest.ResponseRecorder, <-chan struct{}) {
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/-/reload", nil).WithContext(ctx)
		router.ServeHTTP(w, req)
	}()
	return w, done
}

func TestReloadSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reloadCh := make(chan chan error)
		router := route.New()
		Register(router, reloadCh)

		w, done := serveReload(t.Context(), router)

		errc := <-reloadCh
		errc <- nil

		<-done
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestReloadError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reloadCh := make(chan chan error)
		router := route.New()
		Register(router, reloadCh)

		w, done := serveReload(t.Context(), router)

		errc := <-reloadCh
		errc <- errors.New("bad config")

		<-done
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Contains(t, w.Body.String(), "bad config")
	})
}

func TestReloadClientDisconnectBeforeEnqueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// reloadCh is never consumed, so the handler blocks on enqueue.
		// Cancelling the context should unblock it.
		reloadCh := make(chan chan error)
		router := route.New()
		Register(router, reloadCh)

		ctx, cancel := context.WithCancel(t.Context())
		w, done := serveReload(ctx, router)

		// Wait until the handler is durably blocked on the reloadCh send.
		synctest.Wait()
		cancel()

		<-done
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestReloadClientDisconnectDuringReload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The handler enqueues successfully but the client disconnects
		// before the reload result arrives. The buffered channel ensures
		// the sender does not block.
		reloadCh := make(chan chan error)
		router := route.New()
		Register(router, reloadCh)

		ctx, cancel := context.WithCancel(t.Context())
		w, done := serveReload(ctx, router)

		errc := <-reloadCh
		cancel()

		<-done
		require.Equal(t, http.StatusServiceUnavailable, w.Code)

		// Simulate the reloader sending the result after the handler has
		// already returned. This must not block thanks to the buffered
		// channel; if it did, synctest would report a deadlock.
		errc <- nil
	})
}

func TestDebugHandlersWithRoutePrefix(t *testing.T) {
	reloadCh := make(chan chan error)

	// Test with route prefix
	routePrefix := "/prometheus/alertmanager"
	router := route.New().WithPrefix(routePrefix)
	Register(router, reloadCh)

	// Test GET request to pprof index (note: pprof index returns text/html)
	req := httptest.NewRequest("GET", routePrefix+"/debug/pprof/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "/debug/pprof/", "pprof page did not load with expected content when using a route prefix")

	// Test GET request to pprof heap endpoint
	req = httptest.NewRequest("GET", routePrefix+"/debug/pprof/heap", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Test without route prefix (should also work)
	router2 := route.New()
	Register(router2, reloadCh)

	req = httptest.NewRequest("GET", "/debug/pprof/", nil)
	w = httptest.NewRecorder()
	router2.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "/debug/pprof/", "pprof page did not load with expected content")
}
