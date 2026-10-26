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
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/version"
	"github.com/stretchr/testify/require"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
)

func TestStatusService(t *testing.T) {
	t.Parallel()

	t.Run("returns status when clustering is disabled", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{})
		api.Update(&config.Config{})

		resp, err := api.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.NoError(t, err)

		got := resp.Msg.GetStatus()
		require.NotNil(t, got)
		require.Equal(t, version.Version, got.GetVersionInfo().GetVersion())
		require.Equal(t, version.Revision, got.GetVersionInfo().GetRevision())
		require.Equal(t, version.Branch, got.GetVersionInfo().GetBranch())
		require.Equal(t, version.GoVersion, got.GetVersionInfo().GetGoVersion())
		require.NotEmpty(t, got.GetConfig().GetOriginal())
		require.NotNil(t, got.GetStartTime())
		require.Equal(t, statusv3alpha.ClusterStatus_STATE_DISABLED, got.GetCluster().GetState())
		require.Empty(t, got.GetCluster().GetName())
		require.Empty(t, got.GetCluster().GetPeers())
	})

	t.Run("returns sorted peers when clustering is enabled", func(t *testing.T) {
		t.Parallel()

		peer := fakePeer{
			name:   "self",
			status: "ready",
			peers: []cluster.ClusterMember{
				fakeMember{name: "c-node", address: "10.0.0.3:9094"},
				fakeMember{name: "a-node", address: "10.0.0.1:9094"},
				fakeMember{name: "b-node", address: "10.0.0.2:9094"},
			},
		}

		api := NewAPI(Options{Peer: peer})
		api.Update(&config.Config{})

		resp, err := api.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.NoError(t, err)

		clusterStatus := resp.Msg.GetStatus().GetCluster()
		require.Equal(t, "self", clusterStatus.GetName())
		require.Equal(t, statusv3alpha.ClusterStatus_STATE_READY, clusterStatus.GetState())

		names := make([]string, 0, len(clusterStatus.GetPeers()))
		for _, peer := range clusterStatus.GetPeers() {
			names = append(names, peer.GetName())
		}
		require.Equal(t, []string{"a-node", "b-node", "c-node"}, names)
	})

	t.Run("does not block updates behind GetStatus", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer})
		api.Update(&config.Config{})

		statusDone := make(chan error, 1)
		go func() {
			_, err := api.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			statusDone <- err
		}()
		requireClosed(t, peer.entered, waitTimeout)

		updateDone := make(chan struct{})
		go func() {
			api.Update(&config.Config{})
			close(updateDone)
		}()
		requireClosed(t, updateDone, waitTimeout)

		peer.unblock()
		require.NoError(t, requireRecv(t, statusDone, waitTimeout))
	})

	t.Run("cancels active unary RPCs during shutdown", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer, UnaryConcurrency: 1})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		done := make(chan error, 1)
		go func() {
			_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			done <- err
		}()
		requireClosed(t, peer.entered, waitTimeout)

		api.Shutdown()
		err := requireRecv(t, done, waitTimeout)
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
		require.Eventually(t, func() bool { return len(api.admission.unary) == 0 }, waitTimeout, pollInterval)
	})

	t.Run("bounds peer snapshots when the unary deadline expires", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer, Registerer: prometheus.NewRegistry(), UnaryConcurrency: 1, UnaryTimeout: 20 * time.Millisecond})
		api.Update(&config.Config{})

		srv := newTestServer(t, api.Handler(), false)
		client := statusv3alphaconnect.NewStatusServiceClient(&http.Client{Timeout: time.Second}, srv.URL)

		for range 2 {
			started := time.Now()
			_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
			require.Less(t, time.Since(started), 500*time.Millisecond)
		}
		labels := prometheus.Labels{"service": statusv3alphaconnect.StatusServiceName, "procedure": "GetStatus"}
		require.Equal(t, 2.0, testutil.ToFloat64(api.admission.metrics.unaryDeadlines.With(labels)))
		require.Eventually(t, func() bool { return peer.calls.Load() == 1 }, time.Second, pollInterval)
	})

	t.Run("maps cluster states", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			input string
			want  statusv3alpha.ClusterStatus_State
		}{
			{name: "ready", input: "ready", want: statusv3alpha.ClusterStatus_STATE_READY},
			{name: "settling", input: "settling", want: statusv3alpha.ClusterStatus_STATE_SETTLING},
			{name: "empty", input: "", want: statusv3alpha.ClusterStatus_STATE_UNSPECIFIED},
			{name: "unknown", input: "bogus", want: statusv3alpha.ClusterStatus_STATE_UNSPECIFIED},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, tc.want, clusterState(tc.input))
			})
		}
	})

	// This exercises the full ConnectRPC wiring over HTTP, using both the
	// Connect and gRPC protocols to prove the handler works on both
	// transports. The gRPC protocol requires HTTP/2, so both the server and
	// client are configured for unencrypted HTTP/2 (cleartext h2c) via the
	// standard library's http.Protocols.
	t.Run("serves status over HTTP", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			wantMethod string
			opts       []connect.ClientOption
		}{
			{name: "Connect POST", wantMethod: http.MethodPost},
			{name: "Connect HTTP GET", wantMethod: http.MethodGet, opts: []connect.ClientOption{connect.WithHTTPGet()}},
			{name: "gRPC-Web", wantMethod: http.MethodPost, opts: []connect.ClientOption{connect.WithGRPCWeb()}},
			{name: "gRPC", wantMethod: http.MethodPost, opts: []connect.ClientOption{connect.WithGRPC()}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				api := NewAPI(Options{})
				api.Update(&config.Config{})

				methods := make(chan string, 1)
				handler := api.Handler()
				srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					methods <- r.Method
					handler.ServeHTTP(w, r)
				}), true)

				// The Connect protocol works over HTTP/1.1 too, but the native
				// gRPC protocol requires HTTP/2. A single cleartext-HTTP/2 (h2c)
				// client therefore serves every case.
				h2cClient := newH2CClient(t, 5*time.Second)
				client := statusv3alphaconnect.NewStatusServiceClient(h2cClient, srv.URL, tc.opts...)
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				resp, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
				require.NoError(t, err)
				require.Equal(t, tc.wantMethod, requireRecv(t, methods, waitTimeout))
				require.Equal(t, "no-store", resp.Header().Get("Cache-Control"))
				require.Equal(t, version.Version, resp.Msg.GetStatus().GetVersionInfo().GetVersion())
				require.Equal(t, statusv3alpha.ClusterStatus_STATE_DISABLED, resp.Msg.GetStatus().GetCluster().GetState())
			})
		}
	})

	t.Run("rejects oversized incoming messages before handler execution", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer, ReadMaxBytes: 1, MaxRequestBodyBytes: 1024})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(t.Context(), connect.NewRequest(request))
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
		require.Never(t, func() bool { return isClosed(peer.entered) }, 50*time.Millisecond, 5*time.Millisecond)
	})

	t.Run("allows handler options to override message limits", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{ReadMaxBytes: 1, SendMaxBytes: 1})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(connect.WithReadMaxBytes(1024), connect.WithSendMaxBytes(1024*1024)), false)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(t.Context(), connect.NewRequest(request))
		require.NoError(t, err)
	})

	t.Run("rejects oversized unary request bodies", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{ReadMaxBytes: 1024, MaxRequestBodyBytes: 1})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(t.Context(), connect.NewRequest(request))
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	})

	t.Run("rejects oversized outgoing messages", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{SendMaxBytes: 1})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	})

	t.Run("bounds slow unary uploads before handler execution", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer, UnaryConcurrency: 1, UnaryTimeout: 500 * time.Millisecond})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)

		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+statusv3alphaconnect.StatusServiceGetStatusProcedure, reader)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/proto")
		req.ContentLength = 1
		done := make(chan *http.Response, 1)
		go func() {
			resp, _ := srv.Client().Do(req)
			done <- resp
		}()
		require.Eventually(t, func() bool { return len(api.admission.unary) == 1 }, waitTimeout, pollInterval)

		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err = client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))

		resp := requireRecv(t, done, time.Second)
		require.NotNil(t, resp)
		require.NoError(t, resp.Body.Close())
		require.Zero(t, peer.calls.Load())

		peer.unblock()
		_, err = client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.NoError(t, err)
	})

	t.Run("rejects unread request bodies without blocking", func(t *testing.T) {
		t.Parallel()

		peer := newBlockingPeer(t)
		api := NewAPI(Options{Peer: peer, UnaryConcurrency: 1})
		api.Update(&config.Config{})
		handler := api.Handler()
		srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(flushOnlyResponseWriter{ResponseWriter: w}, r)
		}), false)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		firstDone := make(chan error, 1)
		go func() {
			_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			firstDone <- err
		}()
		requireClosed(t, peer.entered, waitTimeout)

		// The cases below run in order while the first RPC still holds the
		// only unary slot, so they must not run in parallel.
		tests := []struct {
			name       string
			path       string
			statusCode int
		}{
			{name: "unknown procedure", path: "/unknown.Service/Unknown", statusCode: http.StatusNotImplemented},
			{name: "over capacity", path: statusv3alphaconnect.StatusServiceGetStatusProcedure, statusCode: http.StatusTooManyRequests},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				reader, writer := io.Pipe()
				t.Cleanup(func() { _ = writer.Close() })
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+tc.path, reader)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/proto")
				req.ContentLength = 1
				done := make(chan *http.Response, 1)
				go func() {
					resp, _ := srv.Client().Do(req)
					done <- resp
				}()

				resp := requireRecv(t, done, time.Second)
				require.NotNil(t, resp)
				require.Equal(t, tc.statusCode, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
				require.NoError(t, writer.Close())
			})
		}

		peer.unblock()
		require.NoError(t, requireRecv(t, firstDone, waitTimeout))
	})
}
