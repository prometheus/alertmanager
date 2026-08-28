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

// Package apiconnect implements the experimental ConnectRPC-based
// Alertmanager API. It is always mounted alongside API v2. Connect and
// gRPC-Web use the version-neutral /api/ prefix, while native gRPC uses the
// server root. Each service is independently versioned (e.g. status.v3alpha); the
// package itself carries no umbrella version.
package apiconnect

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"

	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
)

// Options configures the Connect API.
type Options struct {
	Peer              cluster.ClusterPeer
	UnaryConcurrency  int
	StreamConcurrency int
	UnaryTimeout      time.Duration
}

// API implements the ConnectRPC service handlers for the Connect API.
type API struct {
	peer            cluster.ClusterPeer
	uptime          time.Time
	admission       *admissionInterceptor
	peerSnapshotSem chan struct{}

	configSnapshot atomic.Pointer[string]
}

// NewAPI returns a new Connect API handler. Peer may be nil when clustering
// is disabled.
func NewAPI(opts Options) *API {
	unaryConcurrency := defaultConcurrency(opts.UnaryConcurrency)
	streamConcurrency := defaultConcurrency(opts.StreamConcurrency)
	return &API{
		peer:            opts.Peer,
		uptime:          time.Now(),
		peerSnapshotSem: make(chan struct{}, 1),
		admission: &admissionInterceptor{
			unary:        make(chan struct{}, unaryConcurrency),
			streams:      make(chan struct{}, streamConcurrency),
			unaryTimeout: opts.UnaryTimeout,
		},
	}
}

func defaultConcurrency(concurrency int) int {
	if concurrency < 1 {
		return max(runtime.GOMAXPROCS(0), 8)
	}
	return concurrency
}

// admissionInterceptor gives unary RPCs and streams independent capacity so
// slow Connect clients cannot consume the API v2 GET request allowance.
type admissionInterceptor struct {
	unary        chan struct{}
	streams      chan struct{}
	unaryTimeout time.Duration
}

func (i *admissionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		select {
		case i.unary <- struct{}{}:
			defer func() { <-i.unary }()
		default:
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent unary RPCs reached"))
		}
		if i.unaryTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, i.unaryTimeout)
			defer cancel()
		}
		response, err := next(ctx, req)
		if err == nil || connect.CodeOf(err) != connect.CodeUnknown {
			return response, err
		}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, err)
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, err)
		default:
			return response, err
		}
	}
}

func (i *admissionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *admissionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		select {
		case i.streams <- struct{}{}:
			defer func() { <-i.streams }()
		default:
			return connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent streams reached"))
		}
		return next(ctx, conn)
	}
}

// Update swaps in the currently loaded configuration. It is safe for
// concurrent use with the RPC handlers.
func (api *API) Update(cfg *config.Config) {
	if cfg == nil {
		api.configSnapshot.Store(nil)
		return
	}
	original := cfg.String()
	api.configSnapshot.Store(&original)
}

// Handler returns an http.Handler serving every ConnectRPC service exposed
// by the Connect API: the versioned application services, the gRPC Health
// Checking Protocol (grpc.health.v1.Health), and gRPC server reflection
// (v1 and v1alpha, for tools such as grpcurl). Procedures are
// fully-qualified, so the returned handler is mounted at a single prefix.
func (api *API) Handler(opts ...connect.HandlerOption) http.Handler {
	return api.buildHandler(opts...)
}

// ServicePrefixes returns the URL path prefixes ("/<fully-qualified-service>/")
// for every service registered by Handler. Callers use it to bound the
// cardinality of metric and trace labels: any request path that does not
// match one of these prefixes yields a 404 and should not be recorded
// verbatim.
func (*API) ServicePrefixes() []string {
	return []string{
		"/status.v3alpha.StatusService/",
		"/grpc.health.v1.Health/",
		"/grpc.reflection.v1.ServerReflection/",
		"/grpc.reflection.v1alpha.ServerReflection/",
	}
}

// buildHandler registers every ConnectRPC service on a fresh mux.
func (api *API) buildHandler(opts ...connect.HandlerOption) http.Handler {
	opts = append([]connect.HandlerOption{connect.WithInterceptors(api.admission)}, opts...)

	// serviceNames lists the fully-qualified service names advertised via
	// health checking and reflection.
	serviceNames := []string{
		statusv3alphaconnect.StatusServiceName,
	}

	mux := http.NewServeMux()
	mux.Handle(statusv3alphaconnect.NewStatusServiceHandler(api, opts...))
	mux.Handle(grpchealth.NewHandler(grpchealth.NewStaticChecker(serviceNames...), opts...))

	reflector := grpcreflect.NewStaticReflector(serviceNames...)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, opts...))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, opts...))

	return mux
}
