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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/config"
)

// TestGRPCHealth verifies the gRPC Health Checking Protocol handler is
// mounted and reports SERVING for both the overall server ("") and the
// registered StatusService. The Health service is queried over the Connect
// protocol with JSON, which needs only an HTTP/1.1 client.
func TestGRPCHealth(t *testing.T) {
	t.Parallel()

	api := NewAPI(Options{})
	api.Update(&config.Config{})

	srv := newTestServer(t, api.Handler(), false)
	client := srv.Client()
	client.Timeout = 5 * time.Second

	for _, service := range []string{"", statusv3alphaconnect.StatusServiceName} {
		t.Run("service="+service, func(t *testing.T) {
			t.Parallel()

			reqBody, err := json.Marshal(map[string]string{"service": service})
			require.NoError(t, err)

			resp, err := client.Post(
				srv.URL+"/grpc.health.v1.Health/Check",
				"application/json",
				bytes.NewReader(reqBody),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			var out struct {
				Status string `json:"status"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "SERVING_STATUS_SERVING", out.Status)
		})
	}
}
