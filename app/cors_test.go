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
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api"
)

func TestCORS(t *testing.T) {
	const allowed = "https://a.example.com"
	corsHandler, err := api.NewCORSHandler(`https://a\.example\.com`)
	require.NoError(t, err)

	type check struct {
		name            string
		path            string
		origin          string
		wantAllowOrigin string
	}
	tests := []struct {
		name        string
		routePrefix string
		corsHandler func(http.Handler) http.Handler
		checks      []check
	}{
		{
			name:        "enabled",
			corsHandler: corsHandler,
			checks: []check{
				{name: "v2", path: "/api/v2/silences", origin: allowed, wantAllowOrigin: allowed},
				{name: "suffixed origin", path: "/api/v2/silences", origin: "https://a.example.com.evil.net"},
				{name: "other origin", path: "/api/v2/silences", origin: "https://b.example.com"},
				{name: "metrics", path: "/metrics", origin: allowed},
				{name: "reload", path: "/-/reload", origin: allowed},
			},
		},
		{
			name:        "enabled under a route prefix",
			routePrefix: "/alertmanager",
			corsHandler: corsHandler,
			checks: []check{
				{name: "v2", path: "/api/v2/silences", origin: allowed, wantAllowOrigin: allowed},
				{name: "metrics", path: "/metrics", origin: allowed},
			},
		},
		{
			name: "disabled",
			checks: []check{
				{name: "v2", path: "/api/v2/silences", origin: allowed},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := testOptions(t)
			opts.RoutePrefix = tc.routePrefix
			opts.CORSHandler = tc.corsHandler
			a, err := New(opts)
			require.NoError(t, err)
			require.NoError(t, a.Start())
			t.Cleanup(func() { _ = a.Stop(t.Context()) })
			waitHealthy(t, a.Addr()+tc.routePrefix)

			for _, c := range tc.checks {
				t.Run(c.name, func(t *testing.T) {
					req, err := http.NewRequest(http.MethodOptions, "http://"+a.Addr()+tc.routePrefix+c.path, http.NoBody)
					require.NoError(t, err)
					req.Header.Set("Origin", c.origin)
					req.Header.Set("Access-Control-Request-Method", http.MethodPost)

					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					t.Cleanup(func() { _ = resp.Body.Close() })
					require.Equal(t, c.wantAllowOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
				})
			}
		})
	}
}
