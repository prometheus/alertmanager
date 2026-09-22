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

package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIRouting verifies that API v1 and v2 keep being served alongside the
// Connect API, both at the root and under a route prefix.
func TestAPIRouting(t *testing.T) {
	tests := []struct {
		name        string
		routePrefix string
		path        string
		wantStatus  int
	}{
		{name: "v2 at the root", routePrefix: "", path: "/api/v2/status", wantStatus: http.StatusOK},
		{name: "v1 at the root", routePrefix: "", path: "/api/v1/status", wantStatus: http.StatusGone},
		{name: "v2 under a route prefix", routePrefix: "/alertmanager", path: "/api/v2/status", wantStatus: http.StatusOK},
		{name: "v1 under a route prefix", routePrefix: "/alertmanager", path: "/api/v1/status", wantStatus: http.StatusGone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := startInstance(t, tc.routePrefix)
			resp, err := inst.httpClient.Get(inst.webURL(tc.path))
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}
