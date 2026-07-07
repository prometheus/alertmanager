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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyLimitHandler(t *testing.T) {
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
	srv := httptest.NewServer(api.concurrencyLimitHandler(dst))
	t.Cleanup(srv.Close)
	client := &http.Client{Timeout: time.Second}

	firstDone := make(chan error, 1)
	go func() {
		resp, err := client.Get(srv.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		firstDone <- err
	}()
	<-entered

	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, nil)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	unblock()
	require.NoError(t, <-firstDone)
}
