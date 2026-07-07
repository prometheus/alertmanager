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
	"context"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/version"

	statusv3 "github.com/prometheus/alertmanager/api/status/v3"
)

var _ = Describe("StatusService", func() {
	var inst *instance

	BeforeEach(func() {
		inst = startInstance()
	})

	// The app's HTTP listener serves cleartext HTTP/1.1, so the Connect and
	// gRPC-Web protocols are exercised here. Native gRPC requires HTTP/2
	// (h2c/TLS) and is covered by the api/connect unit tests.
	DescribeTable("GetStatus succeeds over supported transports",
		func(opts ...connect.ClientOption) {
			client := inst.statusClient(opts...)

			resp, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3.GetStatusRequest{}))
			Expect(err).NotTo(HaveOccurred())

			status := resp.Msg.GetStatus()
			Expect(status.GetVersionInfo().GetVersion()).To(Equal(version.Version))
			Expect(status.GetConfig().GetOriginal()).NotTo(BeEmpty())
			Expect(status.GetStartTime().AsTime()).NotTo(BeZero())
			Expect(status.GetCluster().GetState()).To(Equal(statusv3.ClusterStatus_STATE_DISABLED))
		},
		Entry("Connect protocol"),
		Entry("gRPC-Web protocol", connect.WithGRPCWeb()),
	)
})
