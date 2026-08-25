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
		func(routePrefix, path string, expectedStatus int) {
			inst := startInstance(routePrefix)
			resp, err := inst.httpClient.Get(inst.webURL(path))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(resp.Body.Close)
			Expect(resp.StatusCode).To(Equal(expectedStatus))
		},
		Entry("v2 at the root", "", "/api/v2/status", http.StatusOK),
		Entry("v1 at the root", "", "/api/v1/status", http.StatusGone),
		Entry("v2 under a route prefix", "/alertmanager", "/api/v2/status", http.StatusOK),
		Entry("v1 under a route prefix", "/alertmanager", "/api/v1/status", http.StatusGone),
	)
})
