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
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/version"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
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
	calls       atomic.Int64
	entered     chan struct{}
	release     chan struct{}
}

func (p *blockingPeer) Name() string   { return "self" }
func (p *blockingPeer) Status() string { return "ready" }
func (p *blockingPeer) Peers() []cluster.ClusterMember {
	p.calls.Add(1)
	p.enteredOnce.Do(func() { close(p.entered) })
	<-p.release
	return nil
}
func (p *blockingPeer) unblock() { p.releaseOnce.Do(func() { close(p.release) }) }

var _ = Describe("StatusService", func() {
	It("returns status when clustering is disabled", func() {
		api := NewAPI(Options{})
		api.Update(&config.Config{})

		resp, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(err).NotTo(HaveOccurred())

		got := resp.Msg.GetStatus()
		Expect(got).NotTo(BeNil())
		Expect(got.GetVersionInfo().GetVersion()).To(Equal(version.Version))
		Expect(got.GetVersionInfo().GetRevision()).To(Equal(version.Revision))
		Expect(got.GetVersionInfo().GetBranch()).To(Equal(version.Branch))
		Expect(got.GetVersionInfo().GetGoVersion()).To(Equal(version.GoVersion))
		Expect(got.GetConfig().GetOriginal()).NotTo(BeEmpty())
		Expect(got.GetStartTime()).NotTo(BeNil())
		Expect(got.GetCluster().GetState()).To(Equal(statusv3alpha.ClusterStatus_STATE_DISABLED))
		Expect(got.GetCluster().GetName()).To(BeEmpty())
		Expect(got.GetCluster().GetPeers()).To(BeEmpty())
	})

	It("returns sorted peers when clustering is enabled", func() {
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

		resp, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(err).NotTo(HaveOccurred())

		clusterStatus := resp.Msg.GetStatus().GetCluster()
		Expect(clusterStatus.GetName()).To(Equal("self"))
		Expect(clusterStatus.GetState()).To(Equal(statusv3alpha.ClusterStatus_STATE_READY))

		names := make([]string, 0, len(clusterStatus.GetPeers()))
		for _, peer := range clusterStatus.GetPeers() {
			names = append(names, peer.GetName())
		}
		Expect(names).To(Equal([]string{"a-node", "b-node", "c-node"}))
	})

	It("does not block updates behind GetStatus", func() {
		peer := &blockingPeer{
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		DeferCleanup(peer.unblock)

		api := NewAPI(Options{Peer: peer})
		api.Update(&config.Config{})

		statusDone := make(chan error, 1)
		go func() {
			_, err := api.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			statusDone <- err
		}()
		Eventually(peer.entered, 5*time.Second).Should(BeClosed())

		updateDone := make(chan struct{})
		go func() {
			api.Update(&config.Config{})
			close(updateDone)
		}()
		Eventually(updateDone, 5*time.Second).Should(BeClosed())

		peer.unblock()
		var statusErr error
		Eventually(statusDone, 5*time.Second).Should(Receive(&statusErr))
		Expect(statusErr).NotTo(HaveOccurred())
	})

	It("bounds peer snapshots when the unary deadline expires", func() {
		peer := &blockingPeer{
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		api := NewAPI(Options{Peer: peer, UnaryTimeout: 20 * time.Millisecond})
		api.Update(&config.Config{})

		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		DeferCleanup(peer.unblock)
		client := statusv3alphaconnect.NewStatusServiceClient(&http.Client{Timeout: time.Second}, srv.URL)

		for range 2 {
			started := time.Now()
			_, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			Expect(connect.CodeOf(err)).To(Equal(connect.CodeDeadlineExceeded))
			Expect(time.Since(started)).To(BeNumerically("<", 500*time.Millisecond))
		}
		Eventually(peer.calls.Load, time.Second).Should(Equal(int64(1)))
	})

	DescribeTable("maps cluster states",
		func(input string, expected statusv3alpha.ClusterStatus_State) {
			Expect(clusterState(input)).To(Equal(expected))
		},
		Entry("ready", "ready", statusv3alpha.ClusterStatus_STATE_READY),
		Entry("settling", "settling", statusv3alpha.ClusterStatus_STATE_SETTLING),
		Entry("empty", "", statusv3alpha.ClusterStatus_STATE_UNSPECIFIED),
		Entry("unknown", "bogus", statusv3alpha.ClusterStatus_STATE_UNSPECIFIED),
	)

	// TestGetStatus_OverHTTP exercises the full ConnectRPC wiring over HTTP,
	// using both the Connect and gRPC protocols to prove the handler works on
	// both transports. The gRPC protocol requires HTTP/2, so both the server
	// and client are configured for unencrypted HTTP/2 (cleartext h2c) via the
	// standard library's http.Protocols.
	DescribeTable("serves status over HTTP",
		func(wantMethod string, opts []connect.ClientOption) {
			api := NewAPI(Options{})
			api.Update(&config.Config{})

			methods := make(chan string, 1)
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
			DeferCleanup(srv.Close)

			// The Connect protocol works over HTTP/1.1 too, but the native gRPC
			// protocol requires HTTP/2. A single cleartext-HTTP/2 (h2c) client
			// therefore serves all subtests below.
			clientProtocols := new(http.Protocols)
			clientProtocols.SetUnencryptedHTTP2(true)
			transport := &http.Transport{Protocols: clientProtocols}
			h2cClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			DeferCleanup(transport.CloseIdleConnections)

			client := statusv3alphaconnect.NewStatusServiceClient(h2cClient, srv.URL, opts...)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, err := client.GetStatus(ctx, connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			Expect(err).NotTo(HaveOccurred())
			Eventually(methods, 5*time.Second).Should(Receive(Equal(wantMethod)))
			Expect(resp.Header().Get("Cache-Control")).To(Equal("no-store"))
			Expect(resp.Msg.GetStatus().GetVersionInfo().GetVersion()).To(Equal(version.Version))
			Expect(resp.Msg.GetStatus().GetCluster().GetState()).To(Equal(statusv3alpha.ClusterStatus_STATE_DISABLED))
		},
		Entry("Connect POST", http.MethodPost, []connect.ClientOption{}),
		Entry("Connect HTTP GET", http.MethodGet, []connect.ClientOption{connect.WithHTTPGet()}),
		Entry("gRPC-Web", http.MethodPost, []connect.ClientOption{connect.WithGRPCWeb()}),
		Entry("gRPC", http.MethodPost, []connect.ClientOption{connect.WithGRPC()}),
	)
})

var _ = Describe("Connect API", func() {
	It("pins registered service prefixes", func() {
		Expect(NewAPI(Options{}).ServicePrefixes()).To(Equal([]string{
			"/status.v3alpha.StatusService/",
			"/grpc.health.v1.Health/",
			"/grpc.reflection.v1.ServerReflection/",
			"/grpc.reflection.v1alpha.ServerReflection/",
		}))
	})
})

var _ = Describe("RPC admission", func() {
	It("defaults unary and stream concurrency independently", func() {
		api := NewAPI(Options{UnaryConcurrency: 1})
		Expect(cap(api.admission.unary)).To(Equal(1))
		Expect(cap(api.admission.streams)).To(BeNumerically(">=", 8))

		api = NewAPI(Options{StreamConcurrency: 1})
		Expect(cap(api.admission.unary)).To(BeNumerically(">=", 8))
		Expect(cap(api.admission.streams)).To(Equal(1))
	})

	It("limits unary RPCs independently", func() {
		admission := &admissionInterceptor{unary: make(chan struct{}, 1), streams: make(chan struct{}, 1)}
		entered := make(chan struct{})
		release := make(chan struct{})
		var enteredOnce sync.Once
		DeferCleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})

		wrapped := admission.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			enteredOnce.Do(func() { close(entered) })
			<-release
			return nil, nil
		})
		firstDone := make(chan error, 1)
		go func() {
			_, err := wrapped(context.Background(), nil)
			firstDone <- err
		}()
		Eventually(entered, 5*time.Second).Should(BeClosed())

		_, err := wrapped(context.Background(), nil)
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))

		close(release)
		var firstErr error
		Eventually(firstDone, 5*time.Second).Should(Receive(&firstErr))
		Expect(firstErr).NotTo(HaveOccurred())

		_, err = wrapped(context.Background(), nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("limits streams independently", func() {
		admission := &admissionInterceptor{unary: make(chan struct{}, 1), streams: make(chan struct{}, 1)}
		entered := make(chan struct{})
		release := make(chan struct{})
		var enteredOnce sync.Once
		DeferCleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})

		wrapped := admission.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error {
			enteredOnce.Do(func() { close(entered) })
			<-release
			return nil
		})
		firstDone := make(chan error, 1)
		go func() { firstDone <- wrapped(context.Background(), nil) }()
		Eventually(entered, 5*time.Second).Should(BeClosed())

		err := wrapped(context.Background(), nil)
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))

		close(release)
		var firstErr error
		Eventually(firstDone, 5*time.Second).Should(Receive(&firstErr))
		Expect(firstErr).NotTo(HaveOccurred())

		Expect(wrapped(context.Background(), nil)).To(Succeed())
	})

	It("sets configured unary deadlines", func() {
		admission := &admissionInterceptor{
			unary:        make(chan struct{}, 1),
			streams:      make(chan struct{}, 1),
			unaryTimeout: 10 * time.Millisecond,
		}
		wrapped := admission.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

		_, err := wrapped(context.Background(), nil)
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeDeadlineExceeded))
	})
})
