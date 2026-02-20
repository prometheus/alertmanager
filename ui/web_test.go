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

package ui

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

func TestDebugHandlersWithRoutePrefix(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reloadCh := make(chan chan error)

	// Test with route prefix
	routePrefix := "/prometheus/alertmanager"
	router := route.New().WithPrefix(routePrefix)
	Register(router, reloadCh, logger)

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
	Register(router2, reloadCh, logger)

	req = httptest.NewRequest("GET", "/debug/pprof/", nil)
	w = httptest.NewRecorder()
	router2.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "/debug/pprof/", "pprof page did not load with expected content")
}

func TestWebRoutes(t *testing.T) {
	router := route.New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	Register(router, make(chan chan error), logger)

	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{
			name: "root",
			path: "/",
		},
		{
			name: "script.js",
			path: "/script.js",
		},
		{
			name: "favicon.ico",
			path: "/favicon.ico",
		},
		{
			name: "Lib wildcard path",
			// Replace with any path under `lib`, in case you want to remove elm-datepicker.
			path: "/lib/elm-datepicker/css/elm-datepicker.css",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)
		})
	}
}
