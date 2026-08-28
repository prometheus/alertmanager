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

package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyLimitHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var enteredOnce sync.Once
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()

		dst := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				enteredOnce.Do(func() { close(entered) })
				<-release
			}
			w.WriteHeader(http.StatusNoContent)
		})
		api := &API{
			requestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "test_requests_in_flight",
			}),
			concurrencyLimitExceeded: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_concurrency_limit_exceeded_total",
			}),
			inFlightSem: make(chan struct{}, 1),
		}
		handler := api.concurrencyLimitHandler(dst)

		firstDone := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			firstDone <- recorder
		}()
		<-entered

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
		require.Equal(t, http.StatusNoContent, recorder.Code)

		unblock()
		first := <-firstDone
		require.Equal(t, http.StatusNoContent, first.Code)

		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusNoContent, recorder.Code)
	})
}

// TestInstrumentConnectHandlerBoundsCardinality ensures that Connect/gRPC
// requests to unregistered paths collapse to a single placeholder handler
// label instead of being recorded verbatim, which would let a client inflate
// metric cardinality by hitting arbitrary paths.
func TestInstrumentConnectHandlerBoundsCardinality(t *testing.T) {
	const servicePrefix = "/status.v3alpha.StatusService/"

	for _, mountPrefix := range []string{"", "/alertmanager/api"} {
		t.Run(mountPrefix, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			requestDuration := prometheus.NewHistogramVec(
				prometheus.HistogramOpts{Name: "test_http_request_duration_seconds"},
				[]string{"handler", "method", "code"},
			)
			reg.MustRegister(requestDuration)

			api := &API{requestDuration: requestDuration}

			// The inner handler mimics the ConnectRPC mux: 200 for the registered
			// procedure, 404 for everything else (including unknown methods on a
			// known service).
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == servicePrefix+"GetStatus" {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.NotFound(w, r)
			})
			h := api.instrumentConnectHandler(
				mountPrefix,
				[]string{servicePrefix},
				http.StripPrefix(mountPrefix, inner),
			)

			for _, procedure := range []string{
				servicePrefix + "GetStatus",
				// Unknown methods on a known service must not each get their own
				// label; they collapse onto the service prefix.
				servicePrefix + "Evil1",
				servicePrefix + "Evil2",
				// Entirely unregistered paths collapse onto the placeholder.
				"/attacker/controlled/1",
				"/attacker/controlled/2",
			} {
				recorder := httptest.NewRecorder()
				h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, mountPrefix+procedure, nil))
			}

			families, err := reg.Gather()
			require.NoError(t, err)

			handlers := map[string]struct{}{}
			for _, fam := range families {
				if fam.GetName() != "test_http_request_duration_seconds" {
					continue
				}
				for _, m := range fam.GetMetric() {
					for _, lp := range m.GetLabel() {
						if lp.GetName() == "handler" {
							handlers[lp.GetValue()] = struct{}{}
						}
					}
				}
			}

			require.Equal(t, map[string]struct{}{
				servicePrefix:     {},
				unmatchedRPCLabel: {},
			}, handlers)
		})
	}
}
