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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("API routing", func() {
	DescribeTable("serves v1 and v2 alongside the Connect API",
		func(routePrefix, path string, tlsEnabled bool, expectedStatus int) {
			inst := startInstance(routePrefix, tlsEnabled)
			resp, err := inst.httpClient.Get(inst.webURL(path))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(resp.Body.Close)
			Expect(resp.StatusCode).To(Equal(expectedStatus))
		},
		Entry("v2 over h2c at the root", "", "/api/v2/status", false, http.StatusOK),
		Entry("v1 over h2c at the root", "", "/api/v1/status", false, http.StatusGone),
		Entry("v2 over h2c under a route prefix", "/alertmanager", "/api/v2/status", false, http.StatusOK),
		Entry("v1 over h2c under a route prefix", "/alertmanager", "/api/v1/status", false, http.StatusGone),
		Entry("v2 over TLS at the root", "", "/api/v2/status", true, http.StatusOK),
		Entry("v1 over TLS at the root", "", "/api/v1/status", true, http.StatusGone),
		Entry("v2 over TLS under a route prefix", "/alertmanager", "/api/v2/status", true, http.StatusOK),
		Entry("v1 over TLS under a route prefix", "/alertmanager", "/api/v1/status", true, http.StatusGone),
	)
})
