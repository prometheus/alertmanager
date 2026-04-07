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
	"reflect"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestConditionalTracerDisabled(t *testing.T) {
	tracingEnabled.Store(false)
	t.Cleanup(func() { tracingEnabled.Store(false) })

	tracer := NewTracer("test")
	ctx := context.Background()
	retCtx, span := tracer.Start(ctx, "test-span")

	// When disabled, Start should return the original context and a noop span.
	require.Equal(t, ctx, retCtx)
	require.IsType(t, noop.Span{}, span)

	// Noop span operations should not panic.
	span.SetAttributes(attribute.String("key", "value"))
	span.AddEvent("event")
	span.End()
}

func TestConditionalTracerEnabled(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}
	require.NoError(t, m.ApplyConfig(cfg))
	t.Cleanup(m.Stop)

	tracer := NewTracer("test")
	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")
	defer span.End()

	// When enabled, Start should return a real span (not noop).
	require.NotEqual(t, noop.Span{}, span)
	require.True(t, span.SpanContext().IsValid())
}

func TestTracingEnabledFlagTransitions(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	t.Cleanup(m.Stop)

	// Initially disabled.
	require.False(t, tracingEnabled.Load())

	// Enable tracing.
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}
	require.NoError(t, m.ApplyConfig(cfg))
	require.True(t, tracingEnabled.Load())

	// Disable tracing.
	require.NoError(t, m.ApplyConfig(TracingConfig{}))
	require.False(t, tracingEnabled.Load())
}

func TestApplyConfigNoGapDuringReload(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}
	require.NoError(t, m.ApplyConfig(cfg))
	t.Cleanup(m.Stop)

	require.True(t, tracingEnabled.Load())
	tpBefore := otel.GetTracerProvider()

	// Reload with a different config — tracing should never be disabled.
	cfg2 := TracingConfig{
		Endpoint:   "localhost:5678",
		ClientType: TracingClientHTTP,
	}
	require.NoError(t, m.ApplyConfig(cfg2))

	// tracingEnabled should still be true (was never set to false).
	require.True(t, tracingEnabled.Load())
	require.NotEqual(t, tpBefore, otel.GetTracerProvider())
}

func TestApplyConfigBuildFailurePreservesState(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}
	require.NoError(t, m.ApplyConfig(cfg))
	t.Cleanup(m.Stop)

	require.True(t, tracingEnabled.Load())
	tpBefore := otel.GetTracerProvider()
	shutdownBefore := m.shutdownFunc

	// Apply an invalid config that will cause buildTracerProvider to fail.
	badCfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: "invalid-client-type",
	}
	require.Error(t, m.ApplyConfig(badCfg))

	// State should be preserved — tracing still enabled with original provider.
	require.True(t, tracingEnabled.Load())
	require.Equal(t, tpBefore, otel.GetTracerProvider())
	require.NotNil(t, m.shutdownFunc)
	// shutdownFunc should be the same as before the failed apply.
	require.Equal(t, reflect.ValueOf(shutdownBefore).Pointer(), reflect.ValueOf(m.shutdownFunc).Pointer())
}

func TestInstallingNewTracerProvider(t *testing.T) {
	tpBefore := otel.GetTracerProvider()

	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}

	require.NoError(t, m.ApplyConfig(cfg))
	require.NotEqual(t, tpBefore, otel.GetTracerProvider())
}

func TestReinstallingTracerProvider(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
		Headers: &commoncfg.Headers{
			Headers: map[string]commoncfg.Header{
				"foo": {Values: []string{"bar"}},
			},
		},
	}

	require.NoError(t, m.ApplyConfig(cfg))
	tpFirstConfig := otel.GetTracerProvider()

	// Trying to apply the same config should not reinstall provider.
	require.NoError(t, m.ApplyConfig(cfg))
	require.Equal(t, tpFirstConfig, otel.GetTracerProvider())

	cfg2 := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientHTTP,
		Headers: &commoncfg.Headers{
			Headers: map[string]commoncfg.Header{
				"bar": {Values: []string{"foo"}},
			},
		},
	}

	require.NoError(t, m.ApplyConfig(cfg2))
	require.NotEqual(t, tpFirstConfig, otel.GetTracerProvider())
	tpSecondConfig := otel.GetTracerProvider()

	// Setting previously unset option should reinstall provider.
	cfg2.Compression = "gzip"
	require.NoError(t, m.ApplyConfig(cfg2))
	require.NotEqual(t, tpSecondConfig, otel.GetTracerProvider())
}

func TestReinstallingTracerProviderWithTLS(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
		TLSConfig: &commoncfg.TLSConfig{
			CAFile: "testdata/ca.cer",
		},
	}

	require.NoError(t, m.ApplyConfig(cfg))
	tpFirstConfig := otel.GetTracerProvider()

	// Trying to apply the same config with TLS should reinstall provider.
	require.NoError(t, m.ApplyConfig(cfg))
	require.NotEqual(t, tpFirstConfig, otel.GetTracerProvider())
}

func TestUninstallingTracerProvider(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}

	require.NoError(t, m.ApplyConfig(cfg))
	require.NotEqual(t, noop.NewTracerProvider(), otel.GetTracerProvider())

	// Uninstall by passing empty config.
	cfg2 := TracingConfig{}

	require.NoError(t, m.ApplyConfig(cfg2))
	// Make sure we get a no-op tracer provider after uninstallation.
	require.Equal(t, noop.NewTracerProvider(), otel.GetTracerProvider())
}

func TestTracerProviderShutdown(t *testing.T) {
	m := NewManager(promslog.NewNopLogger())
	cfg := TracingConfig{
		Endpoint:   "localhost:1234",
		ClientType: TracingClientGRPC,
	}

	require.NoError(t, m.ApplyConfig(cfg))
	m.Stop()

	// Check if we closed the done channel.
	_, ok := <-m.done
	require.False(t, ok)
}
