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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/config"
)

// TestGRPCHealth verifies the gRPC Health Checking Protocol handler is
// mounted and reports SERVING for both the overall server ("") and the
// registered StatusService. The Health service is queried over the Connect
// protocol with JSON, which needs only an HTTP/1.1 client.
var _ = Describe("gRPC health", func() {
	It("reports serving for the server and StatusService", func() {
		api := NewAPI(Options{})
		api.Update(&config.Config{})

		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		client := srv.Client()
		client.Timeout = 5 * time.Second

		for _, service := range []string{"", statusv3alphaconnect.StatusServiceName} {
			reqBody, err := json.Marshal(map[string]string{"service": service})
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Post(
				srv.URL+"/grpc.health.v1.Health/Check",
				"application/json",
				bytes.NewReader(reqBody),
			)
			Expect(err).NotTo(HaveOccurred())

			var out struct {
				Status string `json:"status"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
			Expect(resp.Body.Close()).To(Succeed())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(out.Status).To(Equal("SERVING_STATUS_SERVING"))
		}
	})
})
