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

package eventrecorder

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

func testWebhookDrops() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_webhook_drops_total",
	}, []string{"output"})
}

func mustParseURL(t *testing.T, raw string) *amcommoncfg.SecretURL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return &amcommoncfg.SecretURL{URL: u}
}

func TestWebhookOutput_SendEvent(t *testing.T) {
	var received [][]byte
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := mustParseURL(t, srv.URL)
	cfg := EventRecorderOutput{
		Type: "webhook",
		URL:  u,
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)
	defer wo.Close()

	require.Equal(t, "webhook:"+srv.URL, wo.Name())

	require.NoError(t, wo.SendEvent([]byte(`{"test":"data"}`)))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	require.JSONEq(t, `{"test":"data"}`, string(received[0]))
	mu.Unlock()
}

func TestWebhookOutput_MultipleWorkers(t *testing.T) {
	var count atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := mustParseURL(t, srv.URL)
	cfg := EventRecorderOutput{
		Type:    "webhook",
		URL:     u,
		Workers: 8,
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	const n = 50
	for range n {
		require.NoError(t, wo.SendEvent([]byte(`{}`)))
	}

	require.NoError(t, wo.Close())
	require.Equal(t, int64(n), count.Load())
}

func TestWebhookOutput_RetryOnFailure(t *testing.T) {
	var attempts atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := mustParseURL(t, srv.URL)
	cfg := EventRecorderOutput{
		Type:         "webhook",
		URL:          u,
		MaxRetries:   3,
		RetryBackoff: model.Duration(10 * time.Millisecond),
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	require.NoError(t, wo.SendEvent([]byte(`{"retry":"test"}`)))

	require.NoError(t, wo.Close())
	require.Equal(t, int64(3), attempts.Load())
}

func TestWebhookOutput_DropsAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := mustParseURL(t, srv.URL)
	cfg := EventRecorderOutput{
		Type:         "webhook",
		URL:          u,
		MaxRetries:   2,
		RetryBackoff: model.Duration(10 * time.Millisecond),
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	require.NoError(t, wo.SendEvent([]byte(`{"drop":"test"}`)))

	require.NoError(t, wo.Close())
	require.Equal(t, int64(2), attempts.Load())
}

func TestWebhookOutput_CloseFlushesQueue(t *testing.T) {
	var count atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := mustParseURL(t, srv.URL)
	cfg := EventRecorderOutput{
		Type:    "webhook",
		URL:     u,
		Workers: 1,
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 5 {
		require.NoError(t, wo.SendEvent([]byte(`{}`)))
	}

	require.NoError(t, wo.Close())
	require.Equal(t, int64(5), count.Load())
}
