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
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
)

// Options configures the Connect API.
type Options struct {
	Peer                cluster.ClusterPeer
	Registerer          prometheus.Registerer
	UnaryConcurrency    int
	StreamConcurrency   int
	UnaryTimeout        time.Duration
	ReadMaxBytes        int
	SendMaxBytes        int
	MaxRequestBodyBytes int64
}

type procedureDescriptor struct {
	path       string
	service    string
	procedure  string
	streamType connect.StreamType
}

type serviceDescriptor struct {
	name       string
	procedures []procedureDescriptor
	handler    func(...connect.HandlerOption) (string, http.Handler)
}

type rpcMetrics struct {
	unaryInFlight   *prometheus.GaugeVec
	unaryRejected   *prometheus.CounterVec
	unaryDuration   *prometheus.HistogramVec
	unaryDeadlines  *prometheus.CounterVec
	streamsActive   *prometheus.GaugeVec
	streamsRejected *prometheus.CounterVec
}

func newRPCMetrics(reg prometheus.Registerer) (*rpcMetrics, error) {
	labels := []string{"service", "procedure"}
	outcomeLabels := []string{"service", "procedure", "outcome"}
	metrics := &rpcMetrics{
		unaryInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_api_connect_unary_requests_in_flight",
			Help: "Current number of admitted Connect unary RPCs.",
		}, labels),
		unaryRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_unary_admission_rejections_total",
			Help: "Total number of Connect unary RPCs rejected by admission control.",
		}, labels),
		unaryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alertmanager_api_connect_unary_request_duration_seconds",
			Help:    "Duration of Connect unary RPCs.",
			Buckets: prometheus.DefBuckets,
		}, outcomeLabels),
		unaryDeadlines: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_unary_deadline_exceeded_total",
			Help: "Total number of Connect unary RPCs that exceeded their deadline.",
		}, labels),
		streamsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_api_connect_streams_active",
			Help: "Current number of admitted Connect streams.",
		}, labels),
		streamsRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_stream_admission_rejections_total",
			Help: "Total number of Connect streams rejected by admission control.",
		}, labels),
	}
	if reg != nil {
		for _, collector := range []prometheus.Collector{
			metrics.unaryInFlight,
			metrics.unaryRejected,
			metrics.unaryDuration,
			metrics.unaryDeadlines,
			metrics.streamsActive,
			metrics.streamsRejected,
		} {
			if err := reg.Register(collector); err != nil {
				return nil, fmt.Errorf("register Connect API metrics: %w", err)
			}
		}
	}
	return metrics, nil
}

// API implements the ConnectRPC service handlers for the Connect API.
type API struct {
	peer            cluster.ClusterPeer
	uptime          time.Time
	admission       *admissionInterceptor
	peerSnapshotSem chan struct{}
	services        []serviceDescriptor
	procedures      map[string]procedureDescriptor
	readMaxBytes    int
	sendMaxBytes    int
	maxRequestBytes int64

	configSnapshot atomic.Pointer[string]
}

// NewAPI returns a new Connect API handler. Peer may be nil when clustering
// is disabled.
func NewAPI(opts Options) (*API, error) {
	metrics, err := newRPCMetrics(opts.Registerer)
	if err != nil {
		return nil, err
	}
	api := &API{
		peer:            opts.Peer,
		uptime:          time.Now(),
		peerSnapshotSem: make(chan struct{}, 1),
		procedures:      make(map[string]procedureDescriptor),
		readMaxBytes:    opts.ReadMaxBytes,
		sendMaxBytes:    opts.SendMaxBytes,
		maxRequestBytes: opts.MaxRequestBodyBytes,
	}
	api.services = api.serviceDescriptors()
	for _, service := range api.services {
		for _, procedure := range service.procedures {
			api.procedures[procedure.path] = procedure
		}
	}
	api.admission = &admissionInterceptor{
		unary:        make(chan struct{}, defaultConcurrency(opts.UnaryConcurrency)),
		streams:      make(chan struct{}, defaultConcurrency(opts.StreamConcurrency)),
		unaryTimeout: opts.UnaryTimeout,
		procedures:   api.procedures,
		metrics:      metrics,
	}
	return api, nil
}

func defaultConcurrency(concurrency int) int {
	if concurrency < 1 {
		return max(runtime.GOMAXPROCS(0), 8)
	}
	return concurrency
}

func procedure(service, name string, streamType connect.StreamType) procedureDescriptor {
	return procedureDescriptor{
		path:       "/" + service + "/" + name,
		service:    service,
		procedure:  name,
		streamType: streamType,
	}
}

func (api *API) serviceDescriptors() []serviceDescriptor {
	status := serviceDescriptor{
		name: statusv3alphaconnect.StatusServiceName,
		procedures: []procedureDescriptor{
			procedure(statusv3alphaconnect.StatusServiceName, "GetStatus", connect.StreamTypeUnary),
		},
		handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
			return statusv3alphaconnect.NewStatusServiceHandler(api, opts...)
		},
	}
	advertised := []string{status.name}
	checker := grpchealth.NewStaticChecker(advertised...)
	reflector := grpcreflect.NewStaticReflector(advertised...)
	return []serviceDescriptor{
		status,
		{
			name: grpchealth.HealthV1ServiceName,
			procedures: []procedureDescriptor{
				procedure(grpchealth.HealthV1ServiceName, "Check", connect.StreamTypeUnary),
				procedure(grpchealth.HealthV1ServiceName, "Watch", connect.StreamTypeServer),
			},
			handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return grpchealth.NewHandler(checker, opts...)
			},
		},
		{
			name: grpcreflect.ReflectV1ServiceName,
			procedures: []procedureDescriptor{
				procedure(grpcreflect.ReflectV1ServiceName, "ServerReflectionInfo", connect.StreamTypeBidi),
			},
			handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return grpcreflect.NewHandlerV1(reflector, opts...)
			},
		},
		{
			name: grpcreflect.ReflectV1AlphaServiceName,
			procedures: []procedureDescriptor{
				procedure(grpcreflect.ReflectV1AlphaServiceName, "ServerReflectionInfo", connect.StreamTypeBidi),
			},
			handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return grpcreflect.NewHandlerV1Alpha(reflector, opts...)
			},
		},
	}
}

type (
	admittedContextKey           struct{}
	observationContextKey        struct{}
	requestStartContextKey       struct{}
	responseControllerContextKey struct{}
)

// admissionInterceptor gives unary RPCs and streams independent capacity so
// slow Connect clients cannot consume the API v2 GET request allowance.
type admissionInterceptor struct {
	unary        chan struct{}
	streams      chan struct{}
	unaryTimeout time.Duration
	procedures   map[string]procedureDescriptor
	metrics      *rpcMetrics
}

func (i *admissionInterceptor) descriptor(path string, streamType connect.StreamType) procedureDescriptor {
	if procedure, ok := i.procedures[path]; ok {
		return procedure
	}
	return procedureDescriptor{service: "unknown", procedure: "unknown", streamType: streamType}
}

func (i *admissionInterceptor) enter(desc procedureDescriptor) (func(), error) {
	labels := prometheus.Labels{"service": desc.service, "procedure": desc.procedure}
	if desc.streamType == connect.StreamTypeUnary {
		select {
		case i.unary <- struct{}{}:
			if i.metrics != nil {
				i.metrics.unaryInFlight.With(labels).Inc()
			}
			return func() {
				<-i.unary
				if i.metrics != nil {
					i.metrics.unaryInFlight.With(labels).Dec()
				}
			}, nil
		default:
			if i.metrics != nil {
				i.metrics.unaryRejected.With(labels).Inc()
			}
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent unary RPCs reached"))
		}
	}
	select {
	case i.streams <- struct{}{}:
		if i.metrics != nil {
			i.metrics.streamsActive.With(labels).Inc()
		}
		return func() {
			<-i.streams
			if i.metrics != nil {
				i.metrics.streamsActive.With(labels).Dec()
			}
		}, nil
	default:
		if i.metrics != nil {
			i.metrics.streamsRejected.With(labels).Inc()
		}
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent streams reached"))
	}
}

func (i *admissionInterceptor) startTime(ctx context.Context) time.Time {
	if started, ok := ctx.Value(requestStartContextKey{}).(time.Time); ok {
		return started
	}
	return time.Now()
}

func (i *admissionInterceptor) observe(desc procedureDescriptor, started time.Time, err error) {
	if i.metrics == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = connect.CodeOf(err).String()
	}
	if desc.streamType == connect.StreamTypeUnary {
		labels := prometheus.Labels{"service": desc.service, "procedure": desc.procedure, "outcome": outcome}
		i.metrics.unaryDuration.With(labels).Observe(time.Since(started).Seconds())
	}
}

func normalizeContextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}
	cause := context.Cause(ctx)
	if errors.Is(cause, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	case errors.Is(err, context.Canceled), errors.Is(cause, context.Canceled):
		return connect.NewError(connect.CodeCanceled, context.Canceled)
	default:
		return err
	}
}

func (i *admissionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		desc := i.descriptor("", connect.StreamTypeUnary)
		if req != nil {
			desc = i.descriptor(req.Spec().Procedure, connect.StreamTypeUnary)
		}
		started := i.startTime(ctx)
		if controller, ok := ctx.Value(responseControllerContextKey{}).(*http.ResponseController); ok {
			_ = controller.SetReadDeadline(time.Time{})
		}
		if _, admitted := ctx.Value(admittedContextKey{}).(struct{}); !admitted {
			release, err := i.enter(desc)
			if err != nil {
				return nil, err
			}
			defer release()
			if i.unaryTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, i.unaryTimeout)
				defer cancel()
			}
		}
		response, err := next(ctx, req)
		err = normalizeContextError(ctx, err)
		i.observe(desc, started, err)
		if observed, ok := ctx.Value(observationContextKey{}).(*atomic.Bool); ok {
			observed.Store(true)
		}
		return response, err
	}
}

func (i *admissionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *admissionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		desc := i.descriptor("", connect.StreamTypeBidi)
		if conn != nil {
			desc = i.descriptor(conn.Spec().Procedure, conn.Spec().StreamType)
		}
		if _, admitted := ctx.Value(admittedContextKey{}).(struct{}); !admitted {
			release, err := i.enter(desc)
			if err != nil {
				return err
			}
			defer release()
		}
		return normalizeContextError(ctx, next(ctx, conn))
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

// Procedures returns the fully-qualified URL paths for every procedure
// registered by Handler. Callers use it to bound metric and trace labels:
// any request path that does not exactly match one of these paths yields a
// 404 and should not be recorded verbatim.
func (api *API) Procedures() []string {
	procedures := make([]string, 0, len(api.procedures))
	for _, service := range api.services {
		for _, procedure := range service.procedures {
			procedures = append(procedures, procedure.path)
		}
	}
	return procedures
}

func (api *API) controlHandler(next http.Handler, errorWriter *connect.ErrorWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		desc, ok := api.procedures[r.URL.Path]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		release, err := api.admission.enter(desc)
		if err != nil {
			_ = r.Body.Close()
			_ = errorWriter.Write(w, r, err)
			return
		}
		defer release()

		controller := http.NewResponseController(w)
		started := time.Now()
		observed := &atomic.Bool{}
		ctx := context.WithValue(r.Context(), admittedContextKey{}, struct{}{})
		ctx = context.WithValue(ctx, observationContextKey{}, observed)
		ctx = context.WithValue(ctx, requestStartContextKey{}, started)
		ctx = context.WithValue(ctx, responseControllerContextKey{}, controller)
		if desc.streamType == connect.StreamTypeUnary && api.admission.unaryTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, api.admission.unaryTimeout)
			defer cancel()
			if deadline, ok := ctx.Deadline(); ok {
				_ = controller.SetReadDeadline(deadline)
			}
		}
		defer func() {
			if desc.streamType == connect.StreamTypeUnary && !observed.Load() {
				err := normalizeContextError(ctx, errors.New("rpc ended before handler execution"))
				api.admission.observe(desc, started, err)
			}
		}()
		if desc.streamType == connect.StreamTypeUnary {
			defer func() {
				if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
					api.admission.metrics.unaryDeadlines.With(prometheus.Labels{"service": desc.service, "procedure": desc.procedure}).Inc()
				}
			}()
		}
		defer func() {
			if ctx.Err() == nil {
				_ = controller.SetReadDeadline(time.Time{})
			}
		}()

		request := r.WithContext(ctx)
		handler := next
		if desc.streamType == connect.StreamTypeUnary && api.maxRequestBytes > 0 {
			handler = http.MaxBytesHandler(handler, api.maxRequestBytes)
		}
		handler.ServeHTTP(w, request)
	})
}

// buildHandler registers every ConnectRPC service on a fresh mux.
func (api *API) buildHandler(opts ...connect.HandlerOption) http.Handler {
	opts = append(opts,
		connect.WithReadMaxBytes(api.readMaxBytes),
		connect.WithSendMaxBytes(api.sendMaxBytes),
		connect.WithInterceptors(api.admission),
	)

	mux := http.NewServeMux()
	for _, service := range api.services {
		path, handler := service.handler(opts...)
		if path != "/"+service.name+"/" {
			panic("Connect service descriptor and handler path disagree")
		}
		mux.Handle(path, handler)
	}
	return api.controlHandler(mux, connect.NewErrorWriter(opts...))
}
