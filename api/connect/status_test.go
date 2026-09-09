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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

type flushOnlyResponseWriter struct {
	http.ResponseWriter
}

func (w flushOnlyResponseWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

var _ = Describe("StatusService", func() {
	It("returns status when clustering is disabled", func() {
		api := newTestAPI(Options{})
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

		api := newTestAPI(Options{Peer: peer})
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

		api := newTestAPI(Options{Peer: peer})
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
		api := newTestAPI(Options{Peer: peer, Registerer: prometheus.NewRegistry(), UnaryConcurrency: 1, UnaryTimeout: 20 * time.Millisecond})
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
		labels := prometheus.Labels{"service": statusv3alphaconnect.StatusServiceName, "procedure": "GetStatus"}
		Expect(testutil.ToFloat64(api.admission.metrics.unaryDeadlines.With(labels))).To(Equal(float64(2)))
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
			api := newTestAPI(Options{})
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

	It("rejects oversized incoming messages before handler execution", func() {
		peer := &blockingPeer{entered: make(chan struct{}), release: make(chan struct{})}
		api := newTestAPI(Options{Peer: peer, ReadMaxBytes: 1, MaxRequestBodyBytes: 1024})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		DeferCleanup(peer.unblock)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(context.Background(), connect.NewRequest(request))
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))
		Consistently(peer.entered, 50*time.Millisecond).ShouldNot(BeClosed())
	})

	It("allows handler options to override message limits", func() {
		api := newTestAPI(Options{ReadMaxBytes: 1, SendMaxBytes: 1})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler(connect.WithReadMaxBytes(1024), connect.WithSendMaxBytes(1024*1024)))
		DeferCleanup(srv.Close)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(context.Background(), connect.NewRequest(request))
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects oversized unary request bodies", func() {
		api := newTestAPI(Options{ReadMaxBytes: 1024, MaxRequestBodyBytes: 1})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)

		request := &statusv3alpha.GetStatusRequest{}
		request.ProtoReflect().SetUnknown([]byte{0x08, 0x01})
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := client.GetStatus(context.Background(), connect.NewRequest(request))
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))
	})

	It("rejects oversized outgoing messages", func() {
		api := newTestAPI(Options{SendMaxBytes: 1})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))
	})

	It("bounds slow unary uploads before handler execution", func() {
		peer := &blockingPeer{entered: make(chan struct{}), release: make(chan struct{})}
		api := newTestAPI(Options{Peer: peer, UnaryConcurrency: 1, UnaryTimeout: 500 * time.Millisecond})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		DeferCleanup(peer.unblock)

		reader, writer := io.Pipe()
		DeferCleanup(writer.Close)
		req, err := http.NewRequest(http.MethodPost, srv.URL+statusv3alphaconnect.StatusServiceGetStatusProcedure, reader)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/proto")
		req.ContentLength = 1
		done := make(chan *http.Response, 1)
		go func() {
			resp, _ := srv.Client().Do(req)
			done <- resp
		}()
		Eventually(func() int { return len(api.admission.unary) }).Should(Equal(1))

		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err = client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))

		var resp *http.Response
		Eventually(done, time.Second).Should(Receive(&resp))
		Expect(resp).NotTo(BeNil())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(peer.calls.Load()).To(BeZero())

		peer.unblock()
		_, err = client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects unread request bodies without blocking", func() {
		peer := &blockingPeer{entered: make(chan struct{}), release: make(chan struct{})}
		api := newTestAPI(Options{Peer: peer, UnaryConcurrency: 1})
		api.Update(&config.Config{})
		handler := api.Handler()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(flushOnlyResponseWriter{ResponseWriter: w}, r)
		}))
		DeferCleanup(srv.Close)
		DeferCleanup(peer.unblock)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		firstDone := make(chan error, 1)
		go func() {
			_, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
			firstDone <- err
		}()
		Eventually(peer.entered, 5*time.Second).Should(BeClosed())

		for _, tc := range []struct {
			path       string
			statusCode int
		}{
			{path: "/unknown.Service/Unknown", statusCode: http.StatusNotImplemented},
			{path: statusv3alphaconnect.StatusServiceGetStatusProcedure, statusCode: http.StatusTooManyRequests},
		} {
			reader, writer := io.Pipe()
			DeferCleanup(func() { _ = writer.Close() })
			req, err := http.NewRequest(http.MethodPost, srv.URL+tc.path, reader)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/proto")
			req.ContentLength = 1
			done := make(chan *http.Response, 1)
			go func() {
				resp, _ := srv.Client().Do(req)
				done <- resp
			}()

			var resp *http.Response
			Eventually(done, time.Second).Should(Receive(&resp))
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).To(Equal(tc.statusCode))
			Expect(resp.Body.Close()).To(Succeed())
			Expect(writer.Close()).To(Succeed())
		}

		peer.unblock()
		var firstErr error
		Eventually(firstDone, 5*time.Second).Should(Receive(&firstErr))
		Expect(firstErr).NotTo(HaveOccurred())
	})
})

var _ = Describe("Connect API", func() {
	It("pins registered procedures", func() {
		Expect(newTestAPI(Options{}).Procedures()).To(Equal([]string{
			"/status.v3alpha.StatusService/GetStatus",
			"/grpc.health.v1.Health/Check",
			"/grpc.health.v1.Health/Watch",
			"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
		}))
	})

	It("rejects unregistered reflection procedures", func() {
		api := newTestAPI(Options{StreamConcurrency: 1})
		srv := httptest.NewUnstartedServer(api.Handler())
		serverProtocols := new(http.Protocols)
		serverProtocols.SetUnencryptedHTTP2(true)
		srv.Config.Protocols = serverProtocols
		srv.Start()
		DeferCleanup(srv.Close)

		clientProtocols := new(http.Protocols)
		clientProtocols.SetUnencryptedHTTP2(true)
		transport := &http.Transport{Protocols: clientProtocols}
		client := &http.Client{Transport: transport}
		DeferCleanup(transport.CloseIdleConnections)

		for _, service := range []string{grpcreflect.ReflectV1ServiceName, grpcreflect.ReflectV1AlphaServiceName} {
			req, err := http.NewRequest(http.MethodPost, srv.URL+"/"+service+"/Unknown", http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/grpc")
			req.Header.Set("TE", "trailers")
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ProtoMajor).To(Equal(2))
			_, err = io.Copy(io.Discard, resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Body.Close()).To(Succeed())
			Expect(resp.Trailer.Get("Grpc-Status")).To(Equal(strconv.Itoa(int(connect.CodeUnimplemented))))
			Expect(api.admission.streams).To(HaveLen(0))
		}
	})

	It("allows unlimited message and request body sizes for non-positive values", func() {
		for _, opts := range []Options{
			{},
			{ReadMaxBytes: -1, SendMaxBytes: -1, MaxRequestBodyBytes: -1, UnaryTimeout: -time.Second},
		} {
			api := newTestAPI(opts)
			Expect(api.readMaxBytes).To(BeZero())
			Expect(api.sendMaxBytes).To(BeZero())
			Expect(api.maxRequestBytes).To(BeZero())
			Expect(api.admission.unaryTimeout).To(BeZero())
		}
	})

	It("panics on duplicate metric registration", func() {
		reg := prometheus.NewRegistry()
		newTestAPI(Options{Registerer: reg})
		Expect(func() { NewAPI(Options{Registerer: reg}) }).To(Panic())
	})

	It("initializes admission metrics", func() {
		reg := prometheus.NewRegistry()
		newTestAPI(Options{Registerer: reg})

		families, err := reg.Gather()
		Expect(err).NotTo(HaveOccurred())
		expected := map[string]int{
			"alertmanager_api_connect_unary_requests_in_flight":                2,
			"alertmanager_api_connect_unary_concurrency_limit_exceeded_total":  2,
			"alertmanager_api_connect_unary_deadline_exceeded_total":           2,
			"alertmanager_api_connect_stream_requests_in_flight":               3,
			"alertmanager_api_connect_stream_concurrency_limit_exceeded_total": 3,
		}
		counts := map[string]int{}
		for _, family := range families {
			if _, ok := expected[family.GetName()]; !ok {
				continue
			}
			counts[family.GetName()] = len(family.GetMetric())
			for _, metric := range family.GetMetric() {
				if metric.GetGauge() != nil {
					Expect(metric.GetGauge().GetValue()).To(BeZero())
				} else {
					Expect(metric.GetCounter().GetValue()).To(BeZero())
				}
			}
		}
		Expect(counts).To(Equal(expected))
	})

	It("registers bounded unary lifecycle metrics", func() {
		reg := prometheus.NewRegistry()
		api := newTestAPI(Options{Registerer: reg})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(err).NotTo(HaveOccurred())
		families, err := reg.Gather()
		Expect(err).NotTo(HaveOccurred())

		var labels map[string]string
		for _, family := range families {
			if family.GetName() != "alertmanager_api_connect_unary_request_duration_seconds" {
				continue
			}
			labels = map[string]string{}
			for _, pair := range family.GetMetric()[0].GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
		}
		Expect(labels).To(Equal(map[string]string{
			"outcome":   "ok",
			"procedure": "GetStatus",
			"service":   statusv3alphaconnect.StatusServiceName,
		}))
	})

	It("observes errors from caller interceptors", func() {
		reg := prometheus.NewRegistry()
		api := newTestAPI(Options{Registerer: reg})
		api.Update(&config.Config{})
		interceptor := connect.UnaryInterceptorFunc(func(connect.UnaryFunc) connect.UnaryFunc {
			return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
			}
		})
		srv := httptest.NewServer(api.Handler(connect.WithInterceptors(interceptor)))
		DeferCleanup(srv.Close)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(connect.CodeOf(err)).To(Equal(connect.CodePermissionDenied))
		families, err := reg.Gather()
		Expect(err).NotTo(HaveOccurred())

		var outcome string
		for _, family := range families {
			if family.GetName() != "alertmanager_api_connect_unary_request_duration_seconds" {
				continue
			}
			for _, pair := range family.GetMetric()[0].GetLabel() {
				if pair.GetName() == "outcome" {
					outcome = pair.GetValue()
				}
			}
		}
		Expect(outcome).To(Equal(connect.CodePermissionDenied.String()))
	})
})

var _ = Describe("RPC admission", func() {
	It("defaults unary and stream concurrency independently", func() {
		api := newTestAPI(Options{UnaryConcurrency: 1})
		Expect(cap(api.admission.unary)).To(Equal(1))
		Expect(cap(api.admission.streams)).To(BeNumerically(">=", 8))

		api = newTestAPI(Options{StreamConcurrency: 1})
		Expect(cap(api.admission.unary)).To(BeNumerically(">=", 8))
		Expect(cap(api.admission.streams)).To(Equal(1))
	})

	It("does not count parent deadlines as configured unary timeouts", func() {
		for _, timeout := range []time.Duration{0, time.Hour} {
			api := newTestAPI(Options{UnaryTimeout: timeout})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			request := httptest.NewRequest(http.MethodPost, statusv3alphaconnect.StatusServiceGetStatusProcedure, nil).WithContext(ctx)
			handler := api.controlHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			}), connect.NewErrorWriter())
			handler.ServeHTTP(httptest.NewRecorder(), request)
			cancel()

			labels := prometheus.Labels{"service": statusv3alphaconnect.StatusServiceName, "procedure": "GetStatus"}
			Expect(testutil.ToFloat64(api.admission.metrics.unaryDeadlines.With(labels))).To(BeZero())
		}
	})

	It("limits unary RPCs independently", func() {
		admission := &admissionInterceptor{
			unary:   make(chan struct{}, 1),
			streams: make(chan struct{}, 1),
			metrics: newRPCMetrics(nil),
		}
		unary := procedureDescriptor{service: "test", procedure: "Unary", streamType: connect.StreamTypeUnary}
		stream := procedureDescriptor{service: "test", procedure: "Stream", streamType: connect.StreamTypeServer}

		releaseUnary, err := admission.enter(unary)
		Expect(err).NotTo(HaveOccurred())
		_, err = admission.enter(unary)
		Expect(connect.CodeOf(err)).To(Equal(connect.CodeResourceExhausted))

		releaseStream, err := admission.enter(stream)
		Expect(err).NotTo(HaveOccurred())
		releaseStream()
		releaseUnary()

		releaseUnary, err = admission.enter(unary)
		Expect(err).NotTo(HaveOccurred())
		releaseUnary()
	})

	It("limits and releases streams over HTTP", func() {
		api := newTestAPI(Options{UnaryConcurrency: 1, StreamConcurrency: 1})
		api.Update(&config.Config{})
		srv := httptest.NewServer(api.Handler())
		DeferCleanup(srv.Close)
		healthClient := grpchealth.NewClient(srv.Client(), srv.URL)
		labels := prometheus.Labels{"service": grpchealth.HealthV1ServiceName, "procedure": "Watch"}

		firstCtx, cancelFirst := context.WithCancel(context.Background())
		DeferCleanup(cancelFirst)
		first := healthClient.Watch(firstCtx, &grpchealth.CheckRequest{})
		var event grpchealth.WatchEvent
		Eventually(first, 5*time.Second).Should(Receive(&event))
		Expect(event.Err).NotTo(HaveOccurred())
		Eventually(func() float64 {
			return testutil.ToFloat64(api.admission.metrics.streamInFlight.With(labels))
		}).Should(Equal(float64(1)))

		statusClient := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := statusClient.GetStatus(context.Background(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		Expect(err).NotTo(HaveOccurred())

		second := healthClient.Watch(context.Background(), &grpchealth.CheckRequest{})
		Eventually(second, 5*time.Second).Should(Receive(&event))
		Expect(connect.CodeOf(event.Err)).To(Equal(connect.CodeResourceExhausted))
		Expect(testutil.ToFloat64(api.admission.metrics.streamLimitExceeded.With(labels))).To(Equal(float64(1)))

		cancelFirst()
		Eventually(first, 5*time.Second).Should(BeClosed())
		Eventually(func() float64 {
			return testutil.ToFloat64(api.admission.metrics.streamInFlight.With(labels))
		}).Should(BeZero())

		thirdCtx, cancelThird := context.WithCancel(context.Background())
		DeferCleanup(cancelThird)
		third := healthClient.Watch(thirdCtx, &grpchealth.CheckRequest{})
		Eventually(third, 5*time.Second).Should(Receive(&event))
		Expect(event.Err).NotTo(HaveOccurred())
		cancelThird()
		Eventually(third, 5*time.Second).Should(BeClosed())
	})

	It("normalizes only returned handler errors", func() {
		Expect(normalizeContextError(nil)).To(Succeed())
		specific := connect.NewError(connect.CodeInvalidArgument, context.Canceled)
		Expect(normalizeContextError(specific)).To(BeIdenticalTo(specific))
		Expect(connect.CodeOf(normalizeContextError(context.DeadlineExceeded))).To(Equal(connect.CodeDeadlineExceeded))
		Expect(connect.CodeOf(normalizeContextError(context.Canceled))).To(Equal(connect.CodeCanceled))

		generic := errors.New("failed")
		Expect(normalizeContextError(generic)).To(BeIdenticalTo(generic))
	})

	It("panics on missing internal request state", func() {
		admission := &admissionInterceptor{procedures: map[string]procedureDescriptor{}}
		Expect(func() { admission.descriptor("/missing") }).To(PanicWith("missing Connect procedure descriptor for /missing"))
		Expect(func() { unaryRequestStateFromContext(context.Background()) }).To(PanicWith("missing Connect unary request state"))
	})
})
