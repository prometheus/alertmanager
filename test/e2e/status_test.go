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
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"

	statusv3 "github.com/prometheus/alertmanager/api/status/v3"
	"github.com/prometheus/alertmanager/api/status/v3/statusv3connect"
)

var _ = Describe("StatusService", func() {
	var inst *instance

	BeforeEach(func() {
		inst = startInstance()
	})

	DescribeTable("GetStatus succeeds over supported transports",
		func(httpClient connect.HTTPClient, basePath string, opts ...connect.ClientOption) {
			client := inst.statusClient(httpClient, basePath, opts...)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetStatus(ctx, connect.NewRequest(&statusv3.GetStatusRequest{}))
			Expect(err).NotTo(HaveOccurred())

			status := resp.Msg.GetStatus()
			Expect(status.GetVersionInfo().GetVersion()).To(Equal(version.Version))
			Expect(status.GetConfig().GetOriginal()).NotTo(BeEmpty())
			Expect(status.GetStartTime().AsTime()).NotTo(BeZero())
			Expect(status.GetCluster().GetState()).To(Equal(statusv3.ClusterStatus_STATE_DISABLED))
		},
		Entry("Connect protocol", connect.HTTPClient(http.DefaultClient), "/api", connect.WithHTTPGet()),
		Entry("gRPC-Web protocol", connect.HTTPClient(http.DefaultClient), "/api", connect.WithGRPCWeb()),
		Entry("gRPC protocol", connect.HTTPClient(newInsecureHTTP2Client()), "", connect.WithGRPC()),
	)

	DescribeTable("rejects transports outside their configured prefix",
		func(httpClient connect.HTTPClient, basePath string, opts ...connect.ClientOption) {
			client := inst.statusClient(httpClient, basePath, opts...)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := client.GetStatus(ctx, connect.NewRequest(&statusv3.GetStatusRequest{}))
			Expect(err).To(HaveOccurred())
		},
		Entry("Connect protocol at root", connect.HTTPClient(http.DefaultClient), "", connect.WithHTTPGet()),
		Entry("gRPC-Web protocol at root", connect.HTTPClient(http.DefaultClient), "", connect.WithGRPCWeb()),
		Entry("gRPC protocol under /api", connect.HTTPClient(newInsecureHTTP2Client()), "/api", connect.WithGRPC()),
	)

	It("exposes server reflection at root", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := grpc.NewClient(strings.TrimPrefix(inst.baseURL, "http://"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(conn.Close()).To(Succeed()) })

		stream, err := reflectionv1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(stream.Send(&reflectionv1.ServerReflectionRequest{
			MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{},
		})).To(Succeed())
		response, err := stream.Recv()
		Expect(err).NotTo(HaveOccurred())

		services := response.GetListServicesResponse().GetService()
		names := make([]string, 0, len(services))
		for _, service := range services {
			names = append(names, service.GetName())
		}
		Expect(names).To(ContainElement(statusv3connect.StatusServiceName))
	})
})
