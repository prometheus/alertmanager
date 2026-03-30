// Copyright 2021 The Prometheus Authors
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

package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"
)

const serviceName = "alertmanager"

var tracingEnabled atomic.Bool

var noopSpan = noop.Span{}

// conditionalTracer wraps the global OTel tracer and short-circuits
// Start when tracing is disabled, avoiding allocations entirely.
type conditionalTracer struct {
	noop.Tracer
	name string
}

// NewTracer returns a trace.Tracer that skips span creation when
// tracing is disabled. Use this instead of otel.Tracer().
func NewTracer(name string) trace.Tracer {
	return &conditionalTracer{name: name}
}

func (t *conditionalTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !tracingEnabled.Load() {
		return ctx, noopSpan
	}
	return otel.Tracer(t.name).Start(ctx, spanName, opts...)
}

// Manager is capable of building, (re)installing and shutting down
// the tracer provider.
type Manager struct {
	mtx          sync.Mutex
	logger       *slog.Logger
	done         chan struct{}
	config       TracingConfig
	shutdownFunc func() error
}

// NewManager creates a new tracing manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Run starts the tracing manager. It registers the global text map propagator and error handler.
// It is blocking.
func (m *Manager) Run() {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(otelErrHandler(func(err error) {
		m.logger.Error("OpenTelemetry handler returned an error", "err", err)
	}))
	<-m.done
}

// ApplyConfig takes care of refreshing the tracing configuration by shutting down
// the current tracer provider (if any is registered) and installing a new one.
func (m *Manager) ApplyConfig(cfg TracingConfig) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Update only if a config change is detected. If TLS configuration is
	// set, we have to restart the manager to make sure that new TLS
	// certificates are picked up.
	var blankTLSConfig commoncfg.TLSConfig
	if reflect.DeepEqual(m.config, cfg) && (m.config.TLSConfig == nil || *m.config.TLSConfig == blankTLSConfig) {
		return nil
	}

	// If no endpoint is set, disable tracing and shut down the old provider.
	if cfg.Endpoint == "" {
		tracingEnabled.Store(false)
		otel.SetTracerProvider(noop.NewTracerProvider())
		if m.shutdownFunc != nil {
			if err := m.shutdownFunc(); err != nil {
				m.logger.Warn("Failed to shut down the previous tracer provider", "err", err)
			}
			m.shutdownFunc = nil
		}
		m.config = cfg
		m.logger.Info("Tracing provider uninstalled.")
		return nil
	}

	// Build the new provider before tearing down the old one so that
	// tracing remains available throughout the reload.
	tp, shutdownFunc, err := buildTracerProvider(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("failed to build a new tracer provider: %w", err)
	}

	// Swap to the new provider, then shut down the old one.
	oldShutdown := m.shutdownFunc
	otel.SetTracerProvider(tp)
	tracingEnabled.Store(true)
	m.shutdownFunc = shutdownFunc
	m.config = cfg

	if oldShutdown != nil {
		if err := oldShutdown(); err != nil {
			m.logger.Warn("Failed to shut down the previous tracer provider", "err", err)
		}
	}

	m.logger.Info("Successfully installed a new tracer provider.")
	return nil
}

// Stop gracefully shuts down the tracer provider and stops the tracing manager.
func (m *Manager) Stop() {
	defer close(m.done)

	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.shutdownFunc == nil {
		return
	}

	tracingEnabled.Store(false)
	otel.SetTracerProvider(noop.NewTracerProvider())

	if err := m.shutdownFunc(); err != nil {
		m.logger.Error("failed to shut down the tracer provider", "err", err)
	}
	m.shutdownFunc = nil

	m.logger.Info("Tracing manager stopped")
}

type otelErrHandler func(err error)

func (o otelErrHandler) Handle(err error) {
	o(err)
}

// buildTracerProvider return a new tracer provider ready for installation, together
// with a shutdown function.
func buildTracerProvider(ctx context.Context, tracingCfg TracingConfig) (trace.TracerProvider, func() error, error) {
	client, err := getClient(tracingCfg)
	if err != nil {
		return nil, nil, err
	}

	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	// Create a resource describing the service and the runtime.
	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version.Version),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(tracesdk.ParentBased(
			tracesdk.TraceIDRatioBased(tracingCfg.SamplingFraction),
		)),
		tracesdk.WithResource(res),
	)

	return tp, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return tp.Shutdown(ctx)
	}, nil
}

// headersToMap converts prometheus/common Headers to a simple map[string]string.
// It takes the first value from Values, Secrets, or Files for each header.
func headersToMap(headers *commoncfg.Headers) (map[string]string, error) {
	if headers == nil || len(headers.Headers) == 0 {
		return nil, nil
	}

	result := make(map[string]string)
	for name, header := range headers.Headers {
		if len(header.Values) > 0 {
			result[name] = header.Values[0]
		} else if len(header.Secrets) > 0 {
			result[name] = string(header.Secrets[0])
		} else if len(header.Files) > 0 {
			// Note: Files would need to be read at runtime. For tracing config,
			// we only support direct values and secrets.
			return nil, fmt.Errorf("header files are not supported for tracing configuration")
		}
	}
	return result, nil
}

// getClient returns an appropriate OTLP client (either gRPC or HTTP), based
// on the provided tracing configuration.
func getClient(tracingCfg TracingConfig) (otlptrace.Client, error) {
	var client otlptrace.Client
	switch tracingCfg.ClientType {
	case TracingClientGRPC:
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(tracingCfg.Endpoint)}

		switch {
		case tracingCfg.Insecure:
			opts = append(opts, otlptracegrpc.WithInsecure())
		case tracingCfg.TLSConfig != nil:
			// Use of TLS Credentials forces the use of TLS. Therefore it can
			// only be set when `insecure` is set to false.
			tlsConf, err := commoncfg.NewTLSConfig(tracingCfg.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConf)))
		}

		if tracingCfg.Compression != "" {
			opts = append(opts, otlptracegrpc.WithCompressor(tracingCfg.Compression))
		}

		headers, err := headersToMap(tracingCfg.Headers)
		if err != nil {
			return nil, err
		}
		if len(headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(headers))
		}

		if tracingCfg.Timeout != 0 {
			opts = append(opts, otlptracegrpc.WithTimeout(time.Duration(tracingCfg.Timeout)))
		}

		client = otlptracegrpc.NewClient(opts...)
	case TracingClientHTTP:
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(tracingCfg.Endpoint)}

		switch {
		case tracingCfg.Insecure:
			opts = append(opts, otlptracehttp.WithInsecure())
		case tracingCfg.TLSConfig != nil:
			tlsConf, err := commoncfg.NewTLSConfig(tracingCfg.TLSConfig)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsConf))
		}

		if tracingCfg.Compression == GzipCompression {
			opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}

		headers, err := headersToMap(tracingCfg.Headers)
		if err != nil {
			return nil, err
		}
		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}

		if tracingCfg.Timeout != 0 {
			opts = append(opts, otlptracehttp.WithTimeout(time.Duration(tracingCfg.Timeout)))
		}

		client = otlptracehttp.NewClient(opts...)
	default:
		return nil, fmt.Errorf("unknown tracing client type: %s", tracingCfg.ClientType)
	}

	return client, nil
}
