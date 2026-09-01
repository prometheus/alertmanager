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
	"crypto/tls"
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
)

var _ = Describe("StatusService", func() {
	DescribeTable("GetStatus succeeds over supported transports",
		func(routePrefix string, tlsEnabled, nativeGRPC bool, opts []connect.ClientOption) {
			inst := startInstance(routePrefix, tlsEnabled)
			httpClient := connect.HTTPClient(inst.httpClient)
			basePath := inst.apiPath()
			if nativeGRPC {
				httpClient = inst.rpcClient
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
		Entry("Connect POST over h2c at the root prefix", "", false, false, []connect.ClientOption{}),
		Entry("Connect HTTP GET over h2c at the root prefix", "", false, false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web over h2c at the root prefix", "", false, false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC over h2c at the server root", "", false, true, []connect.ClientOption{connect.WithGRPC()}),
		Entry("Connect POST over h2c under a route prefix", "/alertmanager", false, false, []connect.ClientOption{}),
		Entry("Connect HTTP GET over h2c under a route prefix", "/alertmanager", false, false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web over h2c under a route prefix", "/alertmanager", false, false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC over h2c with a route prefix configured", "/alertmanager", false, true, []connect.ClientOption{connect.WithGRPC()}),
		Entry("Connect POST over TLS at the root prefix", "", true, false, []connect.ClientOption{}),
		Entry("Connect HTTP GET over TLS at the root prefix", "", true, false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web over TLS at the root prefix", "", true, false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC over TLS at the server root", "", true, true, []connect.ClientOption{connect.WithGRPC()}),
		Entry("Connect POST over TLS under a route prefix", "/alertmanager", true, false, []connect.ClientOption{}),
		Entry("Connect HTTP GET over TLS under a route prefix", "/alertmanager", true, false, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web over TLS under a route prefix", "/alertmanager", true, false, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("native gRPC over TLS with a route prefix configured", "/alertmanager", true, true, []connect.ClientOption{connect.WithGRPC()}),
	)

	DescribeTable("rejects transports outside their configured prefix",
		func(routePrefix, basePath string, nativeGRPC bool, opts []connect.ClientOption) {
			inst := startInstance(routePrefix, false)
			httpClient := connect.HTTPClient(inst.httpClient)
			if basePath == "api" {
				basePath = inst.apiPath()
			}
			if nativeGRPC {
				httpClient = inst.rpcClient
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
		func(routePrefix string, tlsEnabled bool) {
			inst := startInstance(routePrefix, tlsEnabled)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			transportCredentials := credentials.TransportCredentials(insecure.NewCredentials())
			if tlsEnabled {
				transportCredentials = credentials.NewTLS(inst.tlsConfig.Clone())
			}
			conn, err := grpc.NewClient(inst.app.Addr(), grpc.WithTransportCredentials(transportCredentials))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(conn.Close)

			healthClient := healthv1.NewHealthClient(conn)
			for _, service := range []string{"", statusv3alphaconnect.StatusServiceName} {
				var remote peer.Peer
				health, err := healthClient.Check(ctx, &healthv1.HealthCheckRequest{Service: service}, grpc.Peer(&remote))
				Expect(err).NotTo(HaveOccurred())
				Expect(health.GetStatus()).To(Equal(healthv1.HealthCheckResponse_SERVING))
				if tlsEnabled {
					info, ok := remote.AuthInfo.(credentials.TLSInfo)
					Expect(ok).To(BeTrue())
					Expect(info.State.NegotiatedProtocol).To(Equal("h2"))
				}
			}

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
		Entry("over h2c without a route prefix", "", false),
		Entry("over h2c with a route prefix", "/alertmanager", false),
		Entry("over TLS without a route prefix", "", true),
		Entry("over TLS with a route prefix", "/alertmanager", true),
	)

	It("honors exporter-toolkit HTTP/2 disablement", func() {
		inst := startInstance("", true, false)
		config := inst.tlsConfig.Clone()
		config.NextProtos = []string{"h2", "http/1.1"}
		conn, err := tls.Dial("tcp", inst.app.Addr(), config)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(conn.Close)
		Expect(conn.ConnectionState().NegotiatedProtocol).NotTo(Equal("h2"))
	})

	It("cancels active streams during shutdown", func() {
		inst := startInstance("", false)
		conn, err := grpc.NewClient(inst.app.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(conn.Close)
		stream, err := reflectionv1.NewServerReflectionClient(conn).ServerReflectionInfo(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(stream.Send(&reflectionv1.ServerReflectionRequest{
			MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{},
		})).To(Succeed())
		_, err = stream.Recv()
		Expect(err).NotTo(HaveOccurred())

		stopDone := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stopDone <- inst.app.Stop(ctx)
		}()
		var stopErr error
		Eventually(stopDone, 2*time.Second).Should(Receive(&stopErr))
		Expect(stopErr).NotTo(HaveOccurred())
		_, err = stream.Recv()
		Expect(err).To(HaveOccurred())
	})
})
