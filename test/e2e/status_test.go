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
	"strings"
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
)

var _ = Describe("StatusService", func() {
	DescribeTable("GetStatus succeeds over supported transports",
		func(routePrefix string, nativeGRPC bool, opts []connect.ClientOption) {
			inst := startInstance(routePrefix)
			httpClient := connect.HTTPClient(inst.httpClient)
			basePath := inst.apiPath()
			if nativeGRPC {
				httpClient = inst.h2cClient
				basePath = ""
			}
			client := inst.statusClient(httpClient, basePath, opts...)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			Expect(err).NotTo(HaveOccurred())

			status := resp.Msg.GetStatus()
			Expect(status.GetVersionInfo().GetVersion()).To(Equal(version.Version))
			Expect(status.GetConfig().GetOriginal()).NotTo(BeEmpty())
			Expect(status.GetStartTime().AsTime()).NotTo(BeZero())
			Expect(status.GetCluster().GetState()).To(Equal(statusv3alpha.ClusterStatus_STATE_DISABLED))
		},
		Entry("Connect POST at the root prefix", "", false, []connect.ClientOption{}),
		Entry("Connect HTTP GET at the root prefix", "", false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web at the root prefix", "", false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC at the server root", "", true, []connect.ClientOption{connect.WithGRPC()}),
		Entry("Connect POST under a route prefix", "/alertmanager", false, []connect.ClientOption{}),
		Entry("Connect HTTP GET under a route prefix", "/alertmanager", false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web under a route prefix", "/alertmanager", false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC with a route prefix configured", "/alertmanager", true, []connect.ClientOption{connect.WithGRPC()}),
	)

	DescribeTable("rejects transports outside their configured prefix",
		func(routePrefix, basePath string, nativeGRPC bool, opts []connect.ClientOption) {
			inst := startInstance(routePrefix)
			httpClient := connect.HTTPClient(inst.httpClient)
			if basePath == "api" {
				basePath = inst.apiPath()
			}
			if nativeGRPC {
				httpClient = inst.h2cClient
			}
			client := inst.statusClient(httpClient, basePath, opts...)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			Expect(err).To(HaveOccurred())
		},
		Entry("Connect HTTP GET at the server root", "", "", false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web at the server root", "", "", false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC under /api", "", "api", true, []connect.ClientOption{connect.WithGRPC()}),
		Entry("Connect HTTP GET outside a route prefix", "/alertmanager", "/api", false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web outside a route prefix", "/alertmanager", "/api", false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC under a prefixed /api", "/alertmanager", "api", true, []connect.ClientOption{connect.WithGRPC()}),
	)

	DescribeTable("exposes native health and reflection at the server root",
		func(routePrefix string) {
			inst := startInstance(routePrefix)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := grpc.NewClient(strings.TrimPrefix(inst.baseURL, "http://"), grpc.WithTransportCredentials(insecure.NewCredentials()))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(conn.Close)

			health, err := healthv1.NewHealthClient(conn).Check(ctx, &healthv1.HealthCheckRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(health.GetStatus()).To(Equal(healthv1.HealthCheckResponse_SERVING))

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
			Expect(names).To(ContainElement(statusv3alphaconnect.StatusServiceName))
		},
		Entry("without a route prefix", ""),
		Entry("with a route prefix", "/alertmanager"),
	)
})
