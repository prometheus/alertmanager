// Copyright 2015 Prometheus Team
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

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for %s, got %d. Body: %s", http.StatusOK, req.URL.Path, w.Code, w.Body.String())
	}
	if w.Body.Len() == 0 {
		t.Error("Expected non-empty response body for pprof index")
	}

	// Test GET request to pprof heap endpoint
	req = httptest.NewRequest("GET", routePrefix+"/debug/pprof/heap", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for %s, got %d", http.StatusOK, req.URL.Path, w.Code)
	}

	// Test without route prefix (should also work)
	router2 := route.New()
	Register(router2, reloadCh, logger)

	req = httptest.NewRequest("GET", "/debug/pprof/", nil)
	w = httptest.NewRecorder()
	router2.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for %s without prefix, got %d", http.StatusOK, req.URL.Path, w.Code)
	}
}
