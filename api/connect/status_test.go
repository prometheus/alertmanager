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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/common/version"
	"github.com/stretchr/testify/require"

	statusv3 "github.com/prometheus/alertmanager/api/status/v3"
	"github.com/prometheus/alertmanager/api/status/v3/statusv3connect"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
)

// fakeMember is a test double for cluster.ClusterMember.
type fakeMember struct {
	name    string
	address string
}

func (m fakeMember) Name() string    { return m.name }
func (m fakeMember) Address() string { return m.address }

// fakePeer is a test double for cluster.ClusterPeer.
type fakePeer struct {
	name   string
	status string
	peers  []cluster.ClusterMember
}

func (p fakePeer) Name() string                   { return p.name }
func (p fakePeer) Status() string                 { return p.status }
func (p fakePeer) Peers() []cluster.ClusterMember { return p.peers }

type blockingPeer struct {
	enteredOnce sync.Once
	releaseOnce sync.Once
	entered     chan struct{}
	release     chan struct{}
}

func (p *blockingPeer) Name() string   { return "self" }
func (p *blockingPeer) Status() string { return "ready" }
func (p *blockingPeer) Peers() []cluster.ClusterMember {
	p.enteredOnce.Do(func() { close(p.entered) })
	<-p.release
	return nil
}
func (p *blockingPeer) unblock() { p.releaseOnce.Do(func() { close(p.release) }) }

func TestGetStatus_ClusterDisabled(t *testing.T) {
	api := NewAPI(nil, nil)
	api.Update(&config.Config{})

	resp, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3.GetStatusRequest{}))
	require.NoError(t, err)

	got := resp.Msg.GetStatus()
	require.NotNil(t, got)

	require.Equal(t, version.Version, got.GetVersionInfo().GetVersion())
	require.Equal(t, version.Revision, got.GetVersionInfo().GetRevision())
	require.Equal(t, version.Branch, got.GetVersionInfo().GetBranch())
	require.Equal(t, version.GoVersion, got.GetVersionInfo().GetGoVersion())

	require.NotEmpty(t, got.GetConfig().GetOriginal())
	require.NotNil(t, got.GetStartTime())

	require.Equal(t, statusv3.ClusterStatus_STATE_DISABLED, got.GetCluster().GetState())
	require.Empty(t, got.GetCluster().GetName())
	require.Empty(t, got.GetCluster().GetPeers())
}

func TestGetStatus_ClusterEnabled_SortsPeers(t *testing.T) {
	peer := fakePeer{
		name:   "self",
		status: "ready",
		peers: []cluster.ClusterMember{
			fakeMember{name: "c-node", address: "10.0.0.3:9094"},
			fakeMember{name: "a-node", address: "10.0.0.1:9094"},
			fakeMember{name: "b-node", address: "10.0.0.2:9094"},
		},
	}

	api := NewAPI(peer, nil)
	api.Update(&config.Config{})

	resp, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3.GetStatusRequest{}))
	require.NoError(t, err)

	cl := resp.Msg.GetStatus().GetCluster()
	require.Equal(t, "self", cl.GetName())
	require.Equal(t, statusv3.ClusterStatus_STATE_READY, cl.GetState())

	names := make([]string, 0, len(cl.GetPeers()))
	for _, p := range cl.GetPeers() {
		names = append(names, p.GetName())
	}
	require.Equal(t, []string{"a-node", "b-node", "c-node"}, names)
}

func TestGetStatusDoesNotBlockUpdate(t *testing.T) {
	peer := &blockingPeer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer peer.unblock()

	api := NewAPI(peer, nil)
	api.Update(&config.Config{})

	statusDone := make(chan error, 1)
	go func() {
		_, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3.GetStatusRequest{}))
		statusDone <- err
	}()
	<-peer.entered

	updateDone := make(chan struct{})
	go func() {
		api.Update(&config.Config{})
		close(updateDone)
	}()

	select {
	case <-updateDone:
	case <-time.After(time.Second):
		t.Fatal("Update blocked behind GetStatus")
	}

	peer.unblock()
	require.NoError(t, <-statusDone)
}

func TestClusterState(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want statusv3.ClusterStatus_State
	}{
		{"ready", statusv3.ClusterStatus_STATE_READY},
		{"settling", statusv3.ClusterStatus_STATE_SETTLING},
		{"", statusv3.ClusterStatus_STATE_UNSPECIFIED},
		{"bogus", statusv3.ClusterStatus_STATE_UNSPECIFIED},
	} {
		require.Equal(t, tc.want, clusterState(tc.in))
	}
}

// TestGetStatus_OverHTTP exercises the full ConnectRPC wiring over HTTP,
// using both the Connect and gRPC protocols to prove the handler works on
// both transports. The gRPC protocol requires HTTP/2, so both the server
// and client are configured for unencrypted HTTP/2 (cleartext h2c) via the
// standard library's http.Protocols.
func TestGetStatus_OverHTTP(t *testing.T) {
	api := NewAPI(nil, nil)
	api.Update(&config.Config{})

	methods := make(chan string, 2)
	handler := api.Handler()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods <- r.Method
		handler.ServeHTTP(w, r)
	}))
	serverProtocols := new(http.Protocols)
	serverProtocols.SetHTTP1(true)
	serverProtocols.SetUnencryptedHTTP2(true)
	srv.Config.Protocols = serverProtocols
	srv.Start()
	t.Cleanup(srv.Close)

	// The Connect protocol works over HTTP/1.1 too, but the native gRPC
	// protocol requires HTTP/2. A single cleartext-HTTP/2 (h2c) client
	// therefore serves both subtests below.
	clientProtocols := new(http.Protocols)
	clientProtocols.SetUnencryptedHTTP2(true)
	h2cClient := &http.Client{Transport: &http.Transport{Protocols: clientProtocols}}

	for _, tc := range []struct {
		name       string
		wantMethod string
		opts       []connect.ClientOption
	}{
		{name: "connect", wantMethod: http.MethodGet, opts: []connect.ClientOption{connect.WithHTTPGet()}},
		{name: "grpc", wantMethod: http.MethodPost, opts: []connect.ClientOption{connect.WithGRPC()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := statusv3connect.NewStatusServiceClient(h2cClient, srv.URL, tc.opts...)
			resp, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3.GetStatusRequest{}))
			require.NoError(t, err)
			require.Equal(t, tc.wantMethod, <-methods)
			require.Equal(t, "no-store", resp.Header().Get("Cache-Control"))
			require.Equal(t, version.Version, resp.Msg.GetStatus().GetVersionInfo().GetVersion())
			require.Equal(t, statusv3.ClusterStatus_STATE_DISABLED, resp.Msg.GetStatus().GetCluster().GetState())
		})
	}
}
