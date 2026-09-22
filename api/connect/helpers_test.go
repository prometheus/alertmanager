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
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/prometheus/alertmanager/cluster"
)

const (
	// Upper bound on how long tests wait for asynchronous conditions.
	waitTimeout = 5 * time.Second
	// Interval at which asynchronous conditions are re-checked.
	pollInterval = 10 * time.Millisecond
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

// blockingPeer is a cluster.ClusterPeer whose Peers call blocks until
// unblock is called, which lets tests hold a request inside the handler.
type blockingPeer struct {
	enteredOnce sync.Once
	releaseOnce sync.Once
	calls       atomic.Int64
	entered     chan struct{}
	release     chan struct{}
}

// newBlockingPeer returns a blockingPeer that is released when the test ends.
func newBlockingPeer(t testing.TB) *blockingPeer {
	t.Helper()
	p := &blockingPeer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	t.Cleanup(p.unblock)
	return p
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

type fakeStreamingConn struct{}

func (fakeStreamingConn) Spec() connect.Spec {
	return connect.Spec{Procedure: "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", StreamType: connect.StreamTypeBidi}
}
func (fakeStreamingConn) Peer() connect.Peer           { return connect.Peer{} }
func (fakeStreamingConn) Receive(any) error            { return nil }
func (fakeStreamingConn) RequestHeader() http.Header   { return http.Header{} }
func (fakeStreamingConn) Send(any) error               { return nil }
func (fakeStreamingConn) ResponseHeader() http.Header  { return http.Header{} }
func (fakeStreamingConn) ResponseTrailer() http.Header { return http.Header{} }

type deadlineResponseWriter struct {
	header        http.Header
	readDeadline  time.Time
	writeDeadline time.Time
}

func (w *deadlineResponseWriter) Header() http.Header       { return w.header }
func (*deadlineResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (*deadlineResponseWriter) WriteHeader(int)             {}
func (w *deadlineResponseWriter) SetReadDeadline(t time.Time) error {
	w.readDeadline = t
	return nil
}

func (w *deadlineResponseWriter) SetWriteDeadline(t time.Time) error {
	w.writeDeadline = t
	return nil
}

// newTestServer starts an httptest.Server for handler and closes it when the
// test ends. With h2c set, the server also accepts unencrypted HTTP/2, which
// the native gRPC protocol requires.
func newTestServer(t testing.TB, handler http.Handler, h2c bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	if h2c {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)
		srv.Config.Protocols = protocols
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// newH2CClient returns an http.Client that speaks unencrypted HTTP/2 and
// releases its idle connections when the test ends.
func newH2CClient(t testing.TB, timeout time.Duration) *http.Client {
	t.Helper()
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: timeout}
}

// requireRecv receives one value from ch or fails the test after timeout.
func requireRecv[T any](t testing.TB, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed while waiting for a value")
		}
		return v
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for a value", timeout)
	}
	panic("unreachable")
}

// requireClosed drains ch until it is closed or fails the test after timeout.
func requireClosed[T any](t testing.TB, ch <-chan T, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("timed out after %s waiting for channel to close", timeout)
		}
	}
}

// isClosed reports whether ch is closed without blocking. Buffered values are
// consumed, so it is only meaningful for signal channels.
func isClosed[T any](ch <-chan T) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}
