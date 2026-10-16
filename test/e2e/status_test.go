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
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/common/version"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"

	"github.com/prometheus/alertmanager/api/silence/v3alpha/silencev3alphaconnect"
	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
)

func TestStatusService(t *testing.T) {
	t.Run("GetStatus succeeds over supported transports", func(t *testing.T) {
		tests := []struct {
			name        string
			routePrefix string
			tlsEnabled  bool
			nativeGRPC  bool
			opts        []connect.ClientOption
		}{
			{name: "Connect POST at the root prefix"},
			{name: "Connect HTTP GET at the root prefix", opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web at the root prefix", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC at the server root", nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
			{name: "Connect POST under a route prefix", routePrefix: "/alertmanager"},
			{name: "Connect HTTP GET under a route prefix", routePrefix: "/alertmanager", opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web under a route prefix", routePrefix: "/alertmanager", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC with a route prefix configured", routePrefix: "/alertmanager", nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
			{name: "Connect POST over TLS at the root prefix", tlsEnabled: true},
			{name: "Connect HTTP GET over TLS at the root prefix", tlsEnabled: true, opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web over TLS at the root prefix", tlsEnabled: true, opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC over TLS at the server root", tlsEnabled: true, nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
			{name: "Connect POST over TLS under a route prefix", routePrefix: "/alertmanager", tlsEnabled: true},
			{name: "Connect HTTP GET over TLS under a route prefix", routePrefix: "/alertmanager", tlsEnabled: true, opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web over TLS under a route prefix", routePrefix: "/alertmanager", tlsEnabled: true, opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC over TLS with a route prefix configured", routePrefix: "/alertmanager", tlsEnabled: true, nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				inst := startInstance(t, instanceOptions{routePrefix: tc.routePrefix, tlsEnabled: tc.tlsEnabled})
				httpClient := connect.HTTPClient(inst.httpClient)
				basePath := inst.apiPath()
				if tc.nativeGRPC {
					httpClient = inst.rpcClient
					basePath = ""
				}
				client := inst.statusClient(httpClient, basePath, tc.opts...)
				ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
				defer cancel()

				resp, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
				require.NoError(t, err)

				status := resp.Msg.GetStatus()
				require.Equal(t, version.Version, status.GetVersionInfo().GetVersion())
				require.NotEmpty(t, status.GetConfig().GetOriginal())
				require.False(t, status.GetStartTime().AsTime().IsZero())
				require.Equal(t, statusv3alpha.ClusterStatus_STATE_DISABLED, status.GetCluster().GetState())
			})
		}
	})

	t.Run("rejects transports outside their configured prefix", func(t *testing.T) {
		// A basePath of "api" is resolved to the instance's prefixed API path.
		tests := []struct {
			name        string
			routePrefix string
			basePath    string
			nativeGRPC  bool
			opts        []connect.ClientOption
		}{
			{name: "Connect HTTP GET at the server root", opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web at the server root", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC under /api", basePath: "api", nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
			{name: "Connect HTTP GET outside a route prefix", routePrefix: "/alertmanager", basePath: "/api", opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web outside a route prefix", routePrefix: "/alertmanager", basePath: "/api", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "native gRPC under a prefixed /api", routePrefix: "/alertmanager", basePath: "api", nativeGRPC: true, opts: []connect.ClientOption{connect.WithGRPC()}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				inst := startInstance(t, instanceOptions{routePrefix: tc.routePrefix})
				httpClient := connect.HTTPClient(inst.httpClient)
				basePath := tc.basePath
				if basePath == "api" {
					basePath = inst.apiPath()
				}
				if tc.nativeGRPC {
					httpClient = inst.rpcClient
				}
				client := inst.statusClient(httpClient, basePath, tc.opts...)
				ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
				defer cancel()

				_, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
				require.Error(t, err)
			})
		}
	})

	t.Run("exposes native health and reflection at the server root", func(t *testing.T) {
		tests := []struct {
			name        string
			routePrefix string
			tlsEnabled  bool
		}{
			{name: "without a route prefix"},
			{name: "with a route prefix", routePrefix: "/alertmanager"},
			{name: "without a route prefix over TLS", tlsEnabled: true},
			{name: "with a route prefix over TLS", routePrefix: "/alertmanager", tlsEnabled: true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				inst := startInstance(t, instanceOptions{routePrefix: tc.routePrefix, tlsEnabled: tc.tlsEnabled})
				ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
				defer cancel()

				transportCredentials := credentials.TransportCredentials(insecure.NewCredentials())
				if tc.tlsEnabled {
					transportCredentials = credentials.NewTLS(inst.tlsConfig.Clone())
				}
				conn, err := grpc.NewClient(inst.app.Addr(), grpc.WithTransportCredentials(transportCredentials))
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })

				healthClient := healthv1.NewHealthClient(conn)
				for _, service := range []string{"", statusv3alphaconnect.StatusServiceName} {
					var remote peer.Peer
					health, err := healthClient.Check(ctx, &healthv1.HealthCheckRequest{Service: service}, grpc.Peer(&remote))
					require.NoError(t, err)
					require.Equal(t, healthv1.HealthCheckResponse_SERVING, health.GetStatus())
					if tc.tlsEnabled {
						info, ok := remote.AuthInfo.(credentials.TLSInfo)
						require.True(t, ok)
						require.Equal(t, "h2", info.State.NegotiatedProtocol)
					}
				}

				stream, err := reflectionv1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
				require.NoError(t, err)
				require.NoError(t, stream.Send(&reflectionv1.ServerReflectionRequest{
					MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{},
				}))
				response, err := stream.Recv()
				require.NoError(t, err)

				services := response.GetListServicesResponse().GetService()
				names := make([]string, 0, len(services))
				for _, service := range services {
					names = append(names, service.GetName())
				}
				require.Contains(t, names, statusv3alphaconnect.StatusServiceName)
				require.Contains(t, names, silencev3alphaconnect.SilenceServiceName)
			})
		}
	})

	t.Run("honors exporter-toolkit HTTP/2 disablement", func(t *testing.T) {
		inst := startInstance(t, instanceOptions{tlsEnabled: true, disableHTTP2: true})
		config := inst.tlsConfig.Clone()
		config.NextProtos = []string{"h2", "http/1.1"}
		conn, err := tls.Dial("tcp", inst.app.Addr(), config)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		require.NotEqual(t, "h2", conn.ConnectionState().NegotiatedProtocol)
	})

	t.Run("cancels active streams during shutdown", func(t *testing.T) {
		inst := startInstance(t, instanceOptions{})
		conn, err := grpc.NewClient(inst.app.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		stream, err := reflectionv1.NewServerReflectionClient(conn).ServerReflectionInfo(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&reflectionv1.ServerReflectionRequest{
			MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{},
		}))
		_, err = stream.Recv()
		require.NoError(t, err)

		stopDone := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			stopDone <- inst.app.Stop(ctx)
		}()
		select {
		case stopErr := <-stopDone:
			require.NoError(t, stopErr)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the instance to stop")
		}
		_, err = stream.Recv()
		require.Error(t, err)
	})
}
