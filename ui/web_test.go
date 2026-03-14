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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

func TestWebRoutes(t *testing.T) {
	router := route.New()
	Register(router)

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
