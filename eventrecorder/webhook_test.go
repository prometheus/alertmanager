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
	"encoding/json"
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
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v2"

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

func readRequestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("failed to read request body: %v", err)
	}
	return body
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
	cfg := WebhookOutputConfig{URL: u}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)
	defer wo.Close()

	require.Equal(t, "webhook:"+srv.URL, wo.Name())

	n, err := wo.SendEvent(sampleEvent())
	require.NoError(t, err)
	require.Positive(t, n)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	// The POST body is the protojson encoding of the event.
	var event map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(received[0], &event))
	require.Contains(t, string(received[0]), "alertmanagerStartupEvent")
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
	cfg := WebhookOutputConfig{
		URL:     u,
		Workers: 8,
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	const n = 50
	for range n {
		_, err := wo.SendEvent(sampleEvent())
		require.NoError(t, err)
	}

	require.NoError(t, wo.Close())
	require.Equal(t, int64(n), count.Load())
}

func TestWebhookOutput_Batching(t *testing.T) {
	requests := make(chan []byte, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- readRequestBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := NewWebhookOutput(WebhookOutputConfig{
		URL:                mustParseURL(t, srv.URL),
		Workers:            4,
		Batch:              true,
		BatchMaxEvents:     3,
		BatchFlushInterval: model.Duration(time.Hour),
	}, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 3 {
		_, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
	}
	require.NoError(t, out.Close())

	var events []json.RawMessage
	require.NoError(t, json.Unmarshal(<-requests, &events))
	require.Len(t, events, 3)
	require.Empty(t, requests)
}

func TestWebhookOutput_BatchingFlushesOnInterval(t *testing.T) {
	requests := make(chan []byte, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- readRequestBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := NewWebhookOutput(WebhookOutputConfig{
		URL:                mustParseURL(t, srv.URL),
		Workers:            1,
		Batch:              true,
		BatchMaxEvents:     10,
		BatchFlushInterval: model.Duration(10 * time.Millisecond),
	}, testWebhookDrops(), slog.Default())
	require.NoError(t, err)
	defer out.Close()

	for range 2 {
		_, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
	}

	select {
	case body := <-requests:
		var events []json.RawMessage
		require.NoError(t, json.Unmarshal(body, &events))
		require.Len(t, events, 2)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch interval flush")
	}
}

func TestWebhookOutput_BatchingByEncodedSize(t *testing.T) {
	requests := make(chan []byte, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- readRequestBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	event := sampleEvent()
	encoded, err := protojson.Marshal(event)
	require.NoError(t, err)
	out, err := NewWebhookOutput(WebhookOutputConfig{
		URL:                mustParseURL(t, srv.URL),
		Workers:            1,
		Batch:              true,
		BatchMaxEvents:     10,
		BatchMaxBytes:      2*len(encoded) + 2, // One byte below two events plus JSON framing.
		BatchFlushInterval: model.Duration(time.Hour),
	}, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 2 {
		_, err := out.SendEvent(event)
		require.NoError(t, err)
	}
	require.NoError(t, out.Close())

	for range 2 {
		var events []json.RawMessage
		require.NoError(t, json.Unmarshal(<-requests, &events))
		require.Len(t, events, 1)
	}
	require.Empty(t, requests)
}

func TestWebhookOutput_BatchingCloseFlushesPartialBatch(t *testing.T) {
	requests := make(chan []byte, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- readRequestBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := NewWebhookOutput(WebhookOutputConfig{
		URL:                mustParseURL(t, srv.URL),
		Workers:            1,
		Batch:              true,
		BatchMaxEvents:     10,
		BatchFlushInterval: model.Duration(time.Hour),
	}, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 2 {
		_, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
	}
	require.NoError(t, out.Close())

	var events []json.RawMessage
	require.NoError(t, json.Unmarshal(<-requests, &events))
	require.Len(t, events, 2)
}

func TestWebhookOutput_BatchingRetriesWholeBatch(t *testing.T) {
	requests := make(chan []byte, 3)
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- readRequestBody(t, r)
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := NewWebhookOutput(WebhookOutputConfig{
		URL:                mustParseURL(t, srv.URL),
		Workers:            1,
		MaxRetries:         2,
		RetryBackoff:       model.Duration(time.Millisecond),
		Batch:              true,
		BatchMaxEvents:     2,
		BatchFlushInterval: model.Duration(time.Hour),
	}, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 2 {
		_, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
	}
	require.NoError(t, out.Close())

	first := <-requests
	second := <-requests
	require.JSONEq(t, string(first), string(second))
	var events []json.RawMessage
	require.NoError(t, json.Unmarshal(second, &events))
	require.Len(t, events, 2)
	require.Empty(t, requests)
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
	cfg := WebhookOutputConfig{
		URL:          u,
		MaxRetries:   3,
		RetryBackoff: model.Duration(10 * time.Millisecond),
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	_, err = wo.SendEvent(sampleEvent())
	require.NoError(t, err)

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
	cfg := WebhookOutputConfig{
		URL:          u,
		MaxRetries:   2,
		RetryBackoff: model.Duration(10 * time.Millisecond),
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	_, err = wo.SendEvent(sampleEvent())
	require.NoError(t, err)

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
	cfg := WebhookOutputConfig{
		URL:     u,
		Workers: 1,
	}
	wo, err := NewWebhookOutput(cfg, testWebhookDrops(), slog.Default())
	require.NoError(t, err)

	for range 5 {
		_, err := wo.SendEvent(sampleEvent())
		require.NoError(t, err)
	}

	require.NoError(t, wo.Close())
	require.Equal(t, int64(5), count.Load())
}

// --- config tests.

func TestWebhookOutputConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, c WebhookOutputConfig)
	}{
		{
			name: "valid minimal",
			yaml: "url: https://example.com/hook\n",
			check: func(t *testing.T, c WebhookOutputConfig) {
				require.NotNil(t, c.URL)
				require.Equal(t, "https://example.com/hook", c.URL.String())
			},
		},
		{
			name: "valid with tunables",
			yaml: "url: https://example.com/h\ntimeout: 5s\nworkers: 8\nmax_retries: 5\nretry_backoff: 250ms\n",
			check: func(t *testing.T, c WebhookOutputConfig) {
				require.Equal(t, model.Duration(5*time.Second), c.Timeout)
				require.Equal(t, 8, c.Workers)
				require.Equal(t, 5, c.MaxRetries)
				require.Equal(t, model.Duration(250*time.Millisecond), c.RetryBackoff)
			},
		},
		{
			name: "valid with batching",
			yaml: "url: https://example.com/h\nbatch: true\nbatch_max_events: 200\nbatch_max_bytes: 2097152\nbatch_flush_interval: 500ms\n",
			check: func(t *testing.T, c WebhookOutputConfig) {
				require.True(t, c.Batch)
				require.Equal(t, 200, c.BatchMaxEvents)
				require.Equal(t, 2097152, c.BatchMaxBytes)
				require.Equal(t, model.Duration(500*time.Millisecond), c.BatchFlushInterval)
			},
		},
		{
			name:    "batch settings without batching",
			yaml:    "url: https://example.com/h\nbatch_max_events: 200\n",
			wantErr: true,
		},
		{
			name:    "missing url",
			yaml:    "{}\n",
			wantErr: true,
		},
		{
			// SecretURL.UnmarshalYAML treats "<secret>" specially and
			// installs an empty url.URL{} so that round-tripping a
			// redacted config (e.g. from the Alertmanager API via
			// amtool) doesn't fail.  An empty URL must still be
			// rejected here as it would be unusable at runtime.
			name:    "placeholder secret url",
			yaml:    "url: <secret>\n",
			wantErr: true,
		},
		{
			// Wrong scheme should be rejected by SecretURL.UnmarshalYAML
			// itself (ParseURL only accepts http/https), so the error
			// surfaces before our validator runs.
			name:    "non-http scheme",
			yaml:    "url: ftp://example.com/\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c WebhookOutputConfig
			err := yaml.Unmarshal([]byte(tc.yaml), &c)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, c)
			}
		})
	}
}

func TestEventRecorderConfigEqual_Webhook(t *testing.T) {
	a := Config{WebhookOutputs: []WebhookOutputConfig{{
		URL:        mustParseURL(t, "https://example.com/hook"),
		Timeout:    model.Duration(10 * time.Second),
		Workers:    4,
		MaxRetries: 3,
	}}}
	b := Config{WebhookOutputs: []WebhookOutputConfig{{
		URL:        mustParseURL(t, "https://example.com/hook"),
		Timeout:    model.Duration(10 * time.Second),
		Workers:    4,
		MaxRetries: 3,
	}}}
	require.True(t, configEqual(a, b))

	// Differing URL.
	b.WebhookOutputs[0].URL = mustParseURL(t, "https://example.com/other")
	require.False(t, configEqual(a, b))
	b.WebhookOutputs[0].URL = a.WebhookOutputs[0].URL

	// Differing workers.
	b.WebhookOutputs[0].Workers = 8
	require.False(t, configEqual(a, b))
	b.WebhookOutputs[0].Workers = a.WebhookOutputs[0].Workers
	b.WebhookOutputs[0].Batch = true
	require.False(t, configEqual(a, b))

	a.WebhookOutputs[0].Batch = true
	b.WebhookOutputs[0].BatchMaxEvents = defaultHTTPBatchMaxEvents
	b.WebhookOutputs[0].BatchMaxBytes = defaultHTTPBatchMaxBytes
	b.WebhookOutputs[0].BatchFlushInterval = model.Duration(defaultHTTPBatchInterval)
	require.True(t, configEqual(a, b))
}
