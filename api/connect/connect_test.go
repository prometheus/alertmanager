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
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/config"
)

func TestConnectAPI(t *testing.T) {
	t.Parallel()

	t.Run("pins registered procedures", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []string{
			"/status.v3alpha.StatusService/GetStatus",
			"/grpc.health.v1.Health/Check",
			"/grpc.health.v1.Health/Watch",
			"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
		}, NewAPI(Options{}).Procedures())
	})

	t.Run("rejects unregistered reflection procedures", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{StreamConcurrency: 1})
		srv := newTestServer(t, api.Handler(), true)
		client := newH2CClient(t, 5*time.Second)

		for _, service := range []string{grpcreflect.ReflectV1ServiceName, grpcreflect.ReflectV1AlphaServiceName} {
			t.Run(service, func(t *testing.T) {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/"+service+"/Unknown", http.NoBody)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/grpc")
				req.Header.Set("TE", "trailers")
				resp, err := client.Do(req)
				require.NoError(t, err)
				require.Equal(t, 2, resp.ProtoMajor)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, strconv.Itoa(int(connect.CodeUnimplemented)), resp.Trailer.Get("Grpc-Status"))
				require.Eventually(t, func() bool { return len(api.admission.streams) == 0 }, waitTimeout, pollInterval)
			})
		}
	})

	t.Run("allows unlimited message and request body sizes for non-positive values", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			opts Options
		}{
			{name: "zero", opts: Options{}},
			{name: "negative", opts: Options{
				ReadMaxBytes:        -1,
				SendMaxBytes:        -1,
				MaxRequestBodyBytes: -1,
				UnaryTimeout:        -time.Second,
				StreamIdleTimeout:   -time.Second,
				StreamLifetime:      -time.Second,
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				api := NewAPI(tc.opts)
				require.Zero(t, api.readMaxBytes)
				require.Zero(t, api.sendMaxBytes)
				require.Zero(t, api.maxRequestBytes)
				require.Zero(t, api.admission.unaryTimeout)
				require.Zero(t, api.admission.streamIdleTimeout)
				require.Zero(t, api.admission.streamLifetime)
			})
		}
	})

	t.Run("panics on duplicate metric registration", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		NewAPI(Options{Registerer: reg})
		require.Panics(t, func() { NewAPI(Options{Registerer: reg}) })
	})

	t.Run("initializes admission metrics", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		NewAPI(Options{Registerer: reg})

		families, err := reg.Gather()
		require.NoError(t, err)
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
					require.Zero(t, metric.GetGauge().GetValue())
				} else {
					require.Zero(t, metric.GetCounter().GetValue())
				}
			}
		}
		require.Equal(t, expected, counts)
	})

	t.Run("registers bounded unary lifecycle metrics", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		api := NewAPI(Options{Registerer: reg})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.NoError(t, err)
		families, err := reg.Gather()
		require.NoError(t, err)

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
		require.Equal(t, map[string]string{
			"outcome":   "ok",
			"procedure": "GetStatus",
			"service":   statusv3alphaconnect.StatusServiceName,
		}, labels)
	})

	t.Run("observes errors from caller interceptors", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		api := NewAPI(Options{Registerer: reg})
		api.Update(&config.Config{})
		interceptor := connect.UnaryInterceptorFunc(func(connect.UnaryFunc) connect.UnaryFunc {
			return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
			}
		})
		srv := newTestServer(t, api.Handler(connect.WithInterceptors(interceptor)), false)
		client := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)

		_, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		families, err := reg.Gather()
		require.NoError(t, err)

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
		require.Equal(t, connect.CodePermissionDenied.String(), outcome)
	})
}

func TestRPCAdmission(t *testing.T) {
	t.Parallel()

	t.Run("defaults unary and stream concurrency independently", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{UnaryConcurrency: 1})
		require.Equal(t, 1, cap(api.admission.unary))
		require.GreaterOrEqual(t, cap(api.admission.streams), 8)

		api = NewAPI(Options{StreamConcurrency: 1})
		require.GreaterOrEqual(t, cap(api.admission.unary), 8)
		require.Equal(t, 1, cap(api.admission.streams))
	})

	t.Run("does not count parent deadlines as configured unary timeouts", func(t *testing.T) {
		t.Parallel()

		for _, timeout := range []time.Duration{0, time.Hour} {
			t.Run(timeout.String(), func(t *testing.T) {
				t.Parallel()

				api := NewAPI(Options{UnaryTimeout: timeout})
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
				defer cancel()
				request := httptest.NewRequestWithContext(ctx, http.MethodPost, statusv3alphaconnect.StatusServiceGetStatusProcedure, nil)
				handler := api.controlHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					<-r.Context().Done()
				}), connect.NewErrorWriter())
				handler.ServeHTTP(httptest.NewRecorder(), request)

				labels := prometheus.Labels{"service": statusv3alphaconnect.StatusServiceName, "procedure": "GetStatus"}
				require.Zero(t, testutil.ToFloat64(api.admission.metrics.unaryDeadlines.With(labels)))
			})
		}
	})

	t.Run("limits unary RPCs independently", func(t *testing.T) {
		t.Parallel()

		admission := &admissionInterceptor{
			unary:   make(chan struct{}, 1),
			streams: make(chan struct{}, 1),
			metrics: newRPCMetrics(nil),
		}
		unary := procedureDescriptor{service: "test", procedure: "Unary", streamType: connect.StreamTypeUnary}
		stream := procedureDescriptor{service: "test", procedure: "Stream", streamType: connect.StreamTypeServer}

		releaseUnary, err := admission.enter(unary)
		require.NoError(t, err)
		_, err = admission.enter(unary)
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))

		releaseStream, err := admission.enter(stream)
		require.NoError(t, err)
		releaseStream()
		releaseUnary()

		releaseUnary, err = admission.enter(unary)
		require.NoError(t, err)
		releaseUnary()
	})

	t.Run("limits and releases streams over HTTP", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{UnaryConcurrency: 1, StreamConcurrency: 1})
		api.Update(&config.Config{})
		srv := newTestServer(t, api.Handler(), false)
		healthClient := grpchealth.NewClient(srv.Client(), srv.URL)
		labels := prometheus.Labels{"service": grpchealth.HealthV1ServiceName, "procedure": "Watch"}
		streamsInFlight := func() float64 {
			return testutil.ToFloat64(api.admission.metrics.streamInFlight.With(labels))
		}

		firstCtx, cancelFirst := context.WithCancel(t.Context())
		defer cancelFirst()
		first := healthClient.Watch(firstCtx, &grpchealth.CheckRequest{})
		event := requireRecv(t, first, waitTimeout)
		require.NoError(t, event.Err)
		require.Eventually(t, func() bool { return streamsInFlight() == 1 }, waitTimeout, pollInterval)

		statusClient := statusv3alphaconnect.NewStatusServiceClient(srv.Client(), srv.URL)
		_, err := statusClient.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
		require.NoError(t, err)

		second := healthClient.Watch(t.Context(), &grpchealth.CheckRequest{})
		event = requireRecv(t, second, waitTimeout)
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(event.Err))
		require.Equal(t, 1.0, testutil.ToFloat64(api.admission.metrics.streamLimitExceeded.With(labels)))

		cancelFirst()
		requireClosed(t, first, waitTimeout)
		require.Eventually(t, func() bool { return streamsInFlight() == 0 }, waitTimeout, pollInterval)

		thirdCtx, cancelThird := context.WithCancel(t.Context())
		defer cancelThird()
		third := healthClient.Watch(thirdCtx, &grpchealth.CheckRequest{})
		event = requireRecv(t, third, waitTimeout)
		require.NoError(t, event.Err)
		cancelThird()
		requireClosed(t, third, waitTimeout)
	})

	t.Run("bounds idle streams before the first message is decoded", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{StreamConcurrency: 1, StreamIdleTimeout: 500 * time.Millisecond, StreamLifetime: 2 * time.Second})
		srv := newTestServer(t, api.Handler(), true)
		client := newH2CClient(t, 2*time.Second)

		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", reader)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/grpc")
		done := make(chan struct{})
		go func() {
			resp, _ := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			close(done)
		}()
		_, err = writer.Write([]byte{0})
		require.NoError(t, err)
		require.Eventually(t, func() bool { return len(api.admission.streams) == 1 }, waitTimeout, pollInterval)
		requireClosed(t, done, 2*time.Second)
		require.Eventually(t, func() bool { return len(api.admission.streams) == 0 }, waitTimeout, pollInterval)
	})

	t.Run("bounds writes when terminating decoded unary RPCs", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancelCause(t.Context())
		writer := &deadlineResponseWriter{header: http.Header{}}
		lifecycle := &rpcLifecycle{
			cancel:       cancel,
			writeTimeout: time.Second,
			controller:   http.NewResponseController(writer),
		}
		lifecycle.decoded.Store(true)

		started := time.Now()
		lifecycle.terminate(context.DeadlineExceeded)
		require.True(t, writer.readDeadline.IsZero())
		require.True(t, writer.writeDeadline.After(started))
		require.ErrorIs(t, context.Cause(ctx), context.DeadlineExceeded)
	})

	t.Run("releases stream capacity after lifetime expiration", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{StreamConcurrency: 1, StreamLifetime: 10 * time.Millisecond})
		wrapped := api.admission.WrapStreamingHandler(func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})

		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(wrapped(t.Context(), fakeStreamingConn{})))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(wrapped(t.Context(), fakeStreamingConn{})))
	})

	t.Run("releases stream capacity after cancellation", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{StreamConcurrency: 1})
		wrapped := api.admission.WrapStreamingHandler(func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		require.Equal(t, connect.CodeCanceled, connect.CodeOf(wrapped(ctx, fakeStreamingConn{})))
	})

	t.Run("releases stream capacity after idle expiration", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{StreamConcurrency: 1, StreamIdleTimeout: 10 * time.Millisecond, StreamLifetime: time.Second})
		wrapped := api.admission.WrapStreamingHandler(func(ctx context.Context, _ connect.StreamingHandlerConn) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})

		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(wrapped(t.Context(), fakeStreamingConn{})))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(wrapped(t.Context(), fakeStreamingConn{})))
	})

	t.Run("normalizes handler and lifecycle errors", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		require.NoError(t, normalizeContextError(ctx, nil))
		specific := connect.NewError(connect.CodeInvalidArgument, context.Canceled)
		require.Same(t, specific, normalizeContextError(ctx, specific))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(normalizeContextError(ctx, context.DeadlineExceeded)))
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(normalizeContextError(ctx, context.Canceled)))

		generic := errors.New("failed")
		require.Same(t, generic, normalizeContextError(ctx, generic))
		expired, cancel := context.WithCancelCause(ctx)
		cancel(context.DeadlineExceeded)
		require.NoError(t, normalizeContextError(expired, nil))
		require.Same(t, specific, normalizeContextError(expired, specific))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(normalizeContextError(expired, generic)))
	})

	t.Run("panics on missing internal request state", func(t *testing.T) {
		t.Parallel()

		admission := &admissionInterceptor{procedures: map[string]procedureDescriptor{}}
		require.PanicsWithValue(t, "missing Connect procedure descriptor for /missing", func() { admission.descriptor("/missing") })
		require.PanicsWithValue(t, "missing Connect unary request state", func() { unaryRequestStateFromContext(t.Context()) })
	})
}
