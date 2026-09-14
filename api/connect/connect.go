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
	"sync"
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
	// Peer provides cluster status. A nil value disables clustering.
	Peer cluster.ClusterPeer
	// Registerer registers API metrics. A nil value keeps usable metrics without
	// registering them.
	Registerer prometheus.Registerer
	// UnaryConcurrency limits Connect unary RPCs independently. Non-positive
	// values use GOMAXPROCS or 8, whichever is larger.
	UnaryConcurrency int
	// StreamConcurrency limits Connect streams independently. Non-positive
	// values use GOMAXPROCS or 8, whichever is larger.
	StreamConcurrency int
	// UnaryTimeout limits Connect unary RPCs, including request reads.
	// Non-positive values disable the timeout.
	UnaryTimeout time.Duration
	// StreamIdleTimeout limits the time between messages on a Connect stream.
	// Non-positive values disable the timeout.
	StreamIdleTimeout time.Duration
	// StreamLifetime limits the total lifetime of a Connect stream.
	// Non-positive values disable the timeout.
	StreamLifetime time.Duration
	// ReadMaxBytes limits each incoming protobuf message. Non-positive values do
	// not set a limit.
	ReadMaxBytes int
	// SendMaxBytes limits each outgoing protobuf message. Non-positive values do
	// not set a limit.
	SendMaxBytes int
	// MaxRequestBodyBytes limits a unary request body before decoding.
	// Non-positive values do not set a limit.
	MaxRequestBodyBytes int64
}

type effectiveOptions struct {
	peer                cluster.ClusterPeer
	registerer          prometheus.Registerer
	unaryConcurrency    int
	streamConcurrency   int
	unaryTimeout        time.Duration
	streamIdleTimeout   time.Duration
	streamLifetime      time.Duration
	readMaxBytes        int
	sendMaxBytes        int
	maxRequestBodyBytes int64
}

func (o Options) resolve() effectiveOptions {
	defaultConcurrency := max(runtime.GOMAXPROCS(0), 8)
	unaryConcurrency := o.UnaryConcurrency
	if unaryConcurrency < 1 {
		unaryConcurrency = defaultConcurrency
	}
	streamConcurrency := o.StreamConcurrency
	if streamConcurrency < 1 {
		streamConcurrency = defaultConcurrency
	}
	return effectiveOptions{
		peer:                o.Peer,
		registerer:          o.Registerer,
		unaryConcurrency:    unaryConcurrency,
		streamConcurrency:   streamConcurrency,
		unaryTimeout:        max(o.UnaryTimeout, 0),
		streamIdleTimeout:   max(o.StreamIdleTimeout, 0),
		streamLifetime:      max(o.StreamLifetime, 0),
		readMaxBytes:        max(o.ReadMaxBytes, 0),
		sendMaxBytes:        max(o.SendMaxBytes, 0),
		maxRequestBodyBytes: max(o.MaxRequestBodyBytes, 0),
	}
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
	unaryInFlight       *prometheus.GaugeVec
	unaryLimitExceeded  *prometheus.CounterVec
	unaryDuration       *prometheus.HistogramVec
	unaryDeadlines      *prometheus.CounterVec
	streamInFlight      *prometheus.GaugeVec
	streamLimitExceeded *prometheus.CounterVec
	streamLifetime      *prometheus.HistogramVec
}

func newRPCMetrics(reg prometheus.Registerer) *rpcMetrics {
	labels := []string{"service", "procedure"}
	outcomeLabels := []string{"service", "procedure", "outcome"}
	metrics := &rpcMetrics{
		unaryInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_api_connect_unary_requests_in_flight",
			Help: "Current number of admitted Connect API unary RPCs.",
		}, labels),
		unaryLimitExceeded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_unary_concurrency_limit_exceeded_total",
			Help: "Total number of Connect API unary RPCs rejected because the concurrency limit was reached.",
		}, labels),
		unaryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alertmanager_api_connect_unary_request_duration_seconds",
			Help:    "Duration of Connect API unary RPCs.",
			Buckets: prometheus.DefBuckets,
		}, outcomeLabels),
		unaryDeadlines: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_unary_deadline_exceeded_total",
			Help: "Total number of Connect API unary RPCs whose configured server timeout expired, independent of the final RPC outcome.",
		}, labels),
		streamInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "alertmanager_api_connect_stream_requests_in_flight",
			Help: "Current number of admitted Connect API streams.",
		}, labels),
		streamLimitExceeded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alertmanager_api_connect_stream_concurrency_limit_exceeded_total",
			Help: "Total number of Connect API streams rejected because the concurrency limit was reached.",
		}, labels),
		streamLifetime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alertmanager_api_connect_stream_lifetime_seconds",
			Help:    "Lifetime of Connect API streams.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 14),
		}, outcomeLabels),
	}
	if reg != nil {
		reg.MustRegister(
			metrics.unaryInFlight,
			metrics.unaryLimitExceeded,
			metrics.unaryDuration,
			metrics.unaryDeadlines,
			metrics.streamInFlight,
			metrics.streamLimitExceeded,
			metrics.streamLifetime,
		)
	}
	return metrics
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
	activeMutex     sync.Mutex
	activeRPCs      map[*rpcLifecycle]struct{}
	draining        atomic.Bool

	configSnapshot atomic.Pointer[string]
}

// NewAPI returns a new Connect API handler. Peer may be nil when clustering
// is disabled.
func NewAPI(opts Options) *API {
	effective := opts.resolve()
	metrics := newRPCMetrics(effective.registerer)
	api := &API{
		peer:            effective.peer,
		uptime:          time.Now(),
		peerSnapshotSem: make(chan struct{}, 1),
		procedures:      make(map[string]procedureDescriptor),
		readMaxBytes:    effective.readMaxBytes,
		sendMaxBytes:    effective.sendMaxBytes,
		maxRequestBytes: effective.maxRequestBodyBytes,
		activeRPCs:      make(map[*rpcLifecycle]struct{}),
	}
	api.services = api.serviceDescriptors()
	for _, service := range api.services {
		for _, procedure := range service.procedures {
			api.procedures[procedure.path] = procedure
			labels := prometheus.Labels{"service": procedure.service, "procedure": procedure.procedure}
			if procedure.streamType == connect.StreamTypeUnary {
				metrics.unaryInFlight.With(labels)
				metrics.unaryLimitExceeded.With(labels)
				metrics.unaryDeadlines.With(labels)
			} else {
				metrics.streamInFlight.With(labels)
				metrics.streamLimitExceeded.With(labels)
			}
		}
	}
	api.admission = &admissionInterceptor{
		unary:             make(chan struct{}, effective.unaryConcurrency),
		streams:           make(chan struct{}, effective.streamConcurrency),
		unaryTimeout:      effective.unaryTimeout,
		streamIdleTimeout: effective.streamIdleTimeout,
		streamLifetime:    effective.streamLifetime,
		procedures:        api.procedures,
		metrics:           metrics,
	}
	return api
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

type rpcLifecycleContextKey struct{}

type rpcLifecycle struct {
	started      time.Time
	cancel       context.CancelCauseFunc
	idleTimeout  time.Duration
	writeTimeout time.Duration
	controller   *http.ResponseController
	mutex        sync.Mutex
	idleTimer    *time.Timer
	finished     bool
	decoded      atomic.Bool
	observed     atomic.Bool
	stream       bool
}

func (l *rpcLifecycle) terminate(cause error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.finished {
		return
	}
	l.cancel(cause)
	if l.controller == nil {
		return
	}
	now := time.Now()
	if l.stream || !l.decoded.Load() {
		_ = l.controller.SetReadDeadline(now)
	}
	if l.stream {
		_ = l.controller.SetWriteDeadline(now)
	} else if l.writeTimeout > 0 {
		_ = l.controller.SetWriteDeadline(now.Add(l.writeTimeout))
	}
}

func (l *rpcLifecycle) touch() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.finished {
		return
	}
	if l.idleTimer != nil {
		l.idleTimer.Stop()
		l.idleTimer.Reset(l.idleTimeout)
	}
}

func (l *rpcLifecycle) stop() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.finished = true
	if l.idleTimer != nil {
		l.idleTimer.Stop()
	}
}

func rpcLifecycleFromContext(ctx context.Context) (*rpcLifecycle, bool) {
	lifecycle, ok := ctx.Value(rpcLifecycleContextKey{}).(*rpcLifecycle)
	return lifecycle, ok && lifecycle != nil
}

type unaryRequestStateContextKey struct{}

type unaryTimeoutError struct{}

func (unaryTimeoutError) Error() string {
	return "configured Connect unary timeout expired"
}

type unaryRequestState struct {
	started   time.Time
	lifecycle *rpcLifecycle
	observed  atomic.Bool
}

func withUnaryRequestState(ctx context.Context, state *unaryRequestState) context.Context {
	return context.WithValue(ctx, unaryRequestStateContextKey{}, state)
}

func unaryRequestStateFromContext(ctx context.Context) *unaryRequestState {
	state, ok := ctx.Value(unaryRequestStateContextKey{}).(*unaryRequestState)
	if !ok || state == nil {
		panic("missing Connect unary request state")
	}
	return state
}

// admissionInterceptor gives unary RPCs and streams independent capacity so
// slow Connect clients cannot consume the API v2 GET request allowance.
type admissionInterceptor struct {
	unary             chan struct{}
	streams           chan struct{}
	unaryTimeout      time.Duration
	streamIdleTimeout time.Duration
	streamLifetime    time.Duration
	procedures        map[string]procedureDescriptor
	metrics           *rpcMetrics
}

func (i *admissionInterceptor) descriptor(path string) procedureDescriptor {
	if procedure, ok := i.procedures[path]; ok {
		return procedure
	}
	panic("missing Connect procedure descriptor for " + path)
}

func (i *admissionInterceptor) enter(desc procedureDescriptor) (func(), error) {
	labels := prometheus.Labels{"service": desc.service, "procedure": desc.procedure}
	if desc.streamType == connect.StreamTypeUnary {
		select {
		case i.unary <- struct{}{}:
			i.metrics.unaryInFlight.With(labels).Inc()
			return func() {
				<-i.unary
				i.metrics.unaryInFlight.With(labels).Dec()
			}, nil
		default:
			i.metrics.unaryLimitExceeded.With(labels).Inc()
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent unary RPCs reached"))
		}
	}
	select {
	case i.streams <- struct{}{}:
		i.metrics.streamInFlight.With(labels).Inc()
		return func() {
			<-i.streams
			i.metrics.streamInFlight.With(labels).Dec()
		}, nil
	default:
		i.metrics.streamLimitExceeded.With(labels).Inc()
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("maximum concurrent streams reached"))
	}
}

func (i *admissionInterceptor) observe(desc procedureDescriptor, started time.Time, err error) {
	outcome := "ok"
	if err != nil {
		outcome = connect.CodeOf(err).String()
	}
	labels := prometheus.Labels{"service": desc.service, "procedure": desc.procedure, "outcome": outcome}
	if desc.streamType == connect.StreamTypeUnary {
		i.metrics.unaryDuration.With(labels).Observe(time.Since(started).Seconds())
		return
	}
	i.metrics.streamLifetime.With(labels).Observe(time.Since(started).Seconds())
}

func normalizeContextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}
	cause := context.Cause(ctx)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(cause, context.DeadlineExceeded), errors.Is(cause, unaryTimeoutError{}):
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	case errors.Is(err, context.Canceled), errors.Is(cause, context.Canceled):
		return connect.NewError(connect.CodeCanceled, context.Canceled)
	default:
		return err
	}
}

func (i *admissionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		desc := i.descriptor(req.Spec().Procedure)
		state := unaryRequestStateFromContext(ctx)
		state.lifecycle.decoded.Store(true)
		if state.lifecycle.controller != nil {
			_ = state.lifecycle.controller.SetReadDeadline(time.Time{})
		}
		response, err := next(ctx, req)
		err = normalizeContextError(ctx, err)
		i.observe(desc, state.started, err)
		state.observed.Store(true)
		state.lifecycle.observed.Store(true)
		return response, err
	}
}

func (i *admissionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

type activityConn struct {
	connect.StreamingHandlerConn
	lifecycle *rpcLifecycle
}

func (c *activityConn) Receive(message any) error {
	err := c.StreamingHandlerConn.Receive(message)
	if err == nil {
		c.lifecycle.touch()
	}
	return err
}

func (c *activityConn) Send(message any) error {
	err := c.StreamingHandlerConn.Send(message)
	if err == nil {
		c.lifecycle.touch()
	}
	return err
}

func (i *admissionInterceptor) unaryContext(ctx context.Context, controller *http.ResponseController) (context.Context, *rpcLifecycle, func()) {
	baseCtx, cancel := context.WithCancelCause(ctx)
	lifecycle := &rpcLifecycle{
		started:      time.Now(),
		cancel:       cancel,
		writeTimeout: i.unaryTimeout,
		controller:   controller,
	}
	unaryCtx := baseCtx
	var timeoutCancel context.CancelFunc
	var stopTimeout func() bool
	if i.unaryTimeout > 0 {
		unaryCtx, timeoutCancel = context.WithTimeoutCause(baseCtx, i.unaryTimeout, unaryTimeoutError{})
		stopTimeout = context.AfterFunc(unaryCtx, func() {
			if errors.Is(context.Cause(unaryCtx), unaryTimeoutError{}) {
				lifecycle.terminate(unaryTimeoutError{})
			}
		})
		if deadline, ok := unaryCtx.Deadline(); ok {
			_ = controller.SetReadDeadline(deadline)
		}
	}
	return context.WithValue(unaryCtx, rpcLifecycleContextKey{}, lifecycle), lifecycle, func() {
		if stopTimeout != nil {
			stopTimeout()
		}
		lifecycle.stop()
		if timeoutCancel != nil {
			timeoutCancel()
		}
		cancel(context.Canceled)
	}
}

func (i *admissionInterceptor) streamContext(ctx context.Context, controller *http.ResponseController) (context.Context, *rpcLifecycle, func()) {
	streamCtx, cancel := context.WithCancelCause(ctx)
	lifecycle := &rpcLifecycle{
		started:     time.Now(),
		cancel:      cancel,
		idleTimeout: i.streamIdleTimeout,
		controller:  controller,
		stream:      true,
	}
	if lifecycle.idleTimeout > 0 {
		lifecycle.idleTimer = time.AfterFunc(lifecycle.idleTimeout, func() { lifecycle.terminate(context.DeadlineExceeded) })
	}
	var lifetimeTimer *time.Timer
	if i.streamLifetime > 0 {
		lifetimeTimer = time.AfterFunc(i.streamLifetime, func() { lifecycle.terminate(context.DeadlineExceeded) })
	}
	return context.WithValue(streamCtx, rpcLifecycleContextKey{}, lifecycle), lifecycle, func() {
		if lifetimeTimer != nil {
			lifetimeTimer.Stop()
		}
		lifecycle.stop()
		cancel(context.Canceled)
	}
}

func (i *admissionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		desc := i.descriptor(conn.Spec().Procedure)
		lifecycle, ok := rpcLifecycleFromContext(ctx)
		if !ok {
			release, err := i.enter(desc)
			if err != nil {
				return err
			}
			defer release()
			var cleanup func()
			ctx, lifecycle, cleanup = i.streamContext(ctx, nil)
			defer cleanup()
		}
		if lifecycle.idleTimeout > 0 {
			conn = &activityConn{StreamingHandlerConn: conn, lifecycle: lifecycle}
		}
		err := normalizeContextError(ctx, next(ctx, conn))
		i.observe(desc, lifecycle.started, err)
		lifecycle.observed.Store(true)
		return err
	}
}

func (api *API) registerRPC(lifecycle *rpcLifecycle) bool {
	api.activeMutex.Lock()
	defer api.activeMutex.Unlock()
	if api.draining.Load() {
		return false
	}
	api.activeRPCs[lifecycle] = struct{}{}
	return true
}

func (api *API) unregisterRPC(lifecycle *rpcLifecycle) {
	api.activeMutex.Lock()
	delete(api.activeRPCs, lifecycle)
	api.activeMutex.Unlock()
}

// Shutdown rejects new RPCs and cancels active RPCs.
func (api *API) Shutdown() {
	api.draining.Store(true)
	api.activeMutex.Lock()
	active := make([]*rpcLifecycle, 0, len(api.activeRPCs))
	for lifecycle := range api.activeRPCs {
		active = append(active, lifecycle)
	}
	api.activeMutex.Unlock()
	for _, lifecycle := range active {
		lifecycle.terminate(context.Canceled)
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
// registered by Handler. Callers use it to bound metric and trace labels.
// Handler rejects every request path that is not in this list.
func (api *API) Procedures() []string {
	procedures := make([]string, 0, len(api.procedures))
	for _, service := range api.services {
		for _, procedure := range service.procedures {
			procedures = append(procedures, procedure.path)
		}
	}
	return procedures
}

func writeConnectError(w http.ResponseWriter, r *http.Request, errorWriter *connect.ErrorWriter, err error) {
	controller := http.NewResponseController(w)
	if err := controller.EnableFullDuplex(); err != nil && r.ProtoMajor < 2 {
		w.Header().Set("Connection", "close")
	}
	_ = errorWriter.Write(w, r, err)
	_ = controller.Flush()
}

func (api *API) controlHandler(next http.Handler, errorWriter *connect.ErrorWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		desc, ok := api.procedures[r.URL.Path]
		if !ok {
			writeConnectError(w, r, errorWriter, connect.NewError(connect.CodeUnimplemented, errors.New("unknown Connect procedure")))
			return
		}
		if api.draining.Load() {
			writeConnectError(w, r, errorWriter, connect.NewError(connect.CodeUnavailable, errors.New("connect API is shutting down")))
			return
		}
		release, err := api.admission.enter(desc)
		if err != nil {
			writeConnectError(w, r, errorWriter, err)
			return
		}
		defer release()

		controller := http.NewResponseController(w)
		var lifecycle *rpcLifecycle
		var cleanup func()
		ctx := r.Context()
		if desc.streamType == connect.StreamTypeUnary {
			ctx, lifecycle, cleanup = api.admission.unaryContext(ctx, controller)
		} else {
			ctx, lifecycle, cleanup = api.admission.streamContext(ctx, controller)
		}
		if !api.registerRPC(lifecycle) {
			cleanup()
			writeConnectError(w, r, errorWriter, connect.NewError(connect.CodeUnavailable, errors.New("connect API is shutting down")))
			return
		}
		defer api.unregisterRPC(lifecycle)
		defer cleanup()

		if desc.streamType != connect.StreamTypeUnary {
			defer func() {
				if !lifecycle.observed.Load() {
					err := normalizeContextError(ctx, errors.New("rpc ended before handler execution"))
					api.admission.observe(desc, lifecycle.started, err)
				}
			}()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		state := &unaryRequestState{started: lifecycle.started, lifecycle: lifecycle}
		ctx = withUnaryRequestState(ctx, state)
		defer func() {
			if !state.observed.Load() {
				err := normalizeContextError(ctx, errors.New("rpc ended before handler execution"))
				api.admission.observe(desc, state.started, err)
			}
		}()
		defer func() {
			if errors.Is(context.Cause(ctx), unaryTimeoutError{}) {
				api.admission.metrics.unaryDeadlines.With(prometheus.Labels{"service": desc.service, "procedure": desc.procedure}).Inc()
			}
		}()
		defer func() {
			if lifecycle.controller != nil && ctx.Err() == nil {
				_ = lifecycle.controller.SetReadDeadline(time.Time{})
			}
		}()

		handler := next
		if api.maxRequestBytes > 0 {
			handler = http.MaxBytesHandler(handler, api.maxRequestBytes)
		}
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// buildHandler registers every ConnectRPC service on a fresh mux.
func (api *API) buildHandler(opts ...connect.HandlerOption) http.Handler {
	opts = append([]connect.HandlerOption{
		connect.WithReadMaxBytes(api.readMaxBytes),
		connect.WithSendMaxBytes(api.sendMaxBytes),
		connect.WithInterceptors(api.admission),
	}, opts...)

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
