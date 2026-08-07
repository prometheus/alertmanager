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

package apiconnect

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/status/v3/statusv3connect"
	"github.com/prometheus/alertmanager/config"
)

// TestGRPCHealth verifies the gRPC Health Checking Protocol handler is
// mounted and reports SERVING for both the overall server ("") and the
// registered StatusService. The Health service is queried over the Connect
// protocol with JSON, which needs only an HTTP/1.1 client.
func TestGRPCHealth(t *testing.T) {
	api := NewAPI(nil, nil)
	api.Update(&config.Config{})

	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	for _, service := range []string{"", statusv3connect.StatusServiceName} {
		reqBody, err := json.Marshal(map[string]string{"service": service})
		require.NoError(t, err)

		resp, err := srv.Client().Post(
			srv.URL+"/grpc.health.v1.Health/Check",
			"application/json",
			bytes.NewReader(reqBody),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var out struct {
			Status string `json:"status"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		require.Equal(t, "SERVING_STATUS_SERVING", out.Status)
	}
}
