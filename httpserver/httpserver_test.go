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
	"time"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

func TestReloadSuccess(t *testing.T) {
	reloadCh := make(chan chan error)
	router := route.New()
	Register(router, reloadCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/-/reload", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}()

	errc := <-reloadCh
	errc <- nil

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return")
	}
}

func TestReloadError(t *testing.T) {
	reloadCh := make(chan chan error)
	router := route.New()
	Register(router, reloadCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/-/reload", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Contains(t, w.Body.String(), "bad config")
	}()

	errc := <-reloadCh
	errc <- errors.New("bad config")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return")
	}
}

func TestReloadClientDisconnectBeforeEnqueue(t *testing.T) {
	// reloadCh is never consumed, so the handler blocks on enqueue.
	// Cancelling the context should unblock it.
	reloadCh := make(chan chan error)
	router := route.New()
	Register(router, reloadCh)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/-/reload", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	}()

	// Give the handler time to block on reloadCh send.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not unblock after context cancellation")
	}
}

func TestReloadClientDisconnectDuringReload(t *testing.T) {
	// The handler enqueues successfully but the client disconnects
	// before the reload result arrives. The buffered channel ensures
	// the main goroutine (sender) does not block.
	reloadCh := make(chan chan error)
	router := route.New()
	Register(router, reloadCh)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/-/reload", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	}()

	errc := <-reloadCh
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not unblock after context cancellation")
	}

	// Simulate the main goroutine sending the result after the handler
	// has already returned. This must not block thanks to the buffered channel.
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		errc <- nil
	}()

	select {
	case <-sendDone:
	case <-time.After(5 * time.Second):
		t.Fatal("sender blocked on errc — channel must be buffered")
	}
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
