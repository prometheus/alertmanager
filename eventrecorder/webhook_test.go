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
	cfg := Output{
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
	cfg := Output{
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
	cfg := Output{
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
	cfg := Output{
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
	cfg := Output{
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

// --- config tests.

func TestOutput_UnmarshalYAML_Webhook(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, o Output)
	}{
		{
			name: "valid minimal",
			yaml: "type: webhook\nurl: https://example.com/hook\n",
			check: func(t *testing.T, o Output) {
				require.Equal(t, OutputWebhook, o.Type)
				require.NotNil(t, o.URL)
				require.Equal(t, "https://example.com/hook", o.URL.String())
			},
		},
		{
			name: "valid with tunables",
			yaml: "type: webhook\nurl: https://example.com/h\ntimeout: 5s\nworkers: 8\nmax_retries: 5\nretry_backoff: 250ms\n",
			check: func(t *testing.T, o Output) {
				require.Equal(t, model.Duration(5*time.Second), o.Timeout)
				require.Equal(t, 8, o.Workers)
				require.Equal(t, 5, o.MaxRetries)
				require.Equal(t, model.Duration(250*time.Millisecond), o.RetryBackoff)
			},
		},
		{
			name:    "missing url",
			yaml:    "type: webhook\n",
			wantErr: true,
		},
		{
			// SecretURL.UnmarshalYAML treats "<secret>" specially and
			// installs an empty url.URL{} so that round-tripping a
			// redacted config (e.g. from the Alertmanager API via
			// amtool) doesn't fail.  An empty URL must still be
			// rejected here as it would be unusable at runtime.
			name:    "placeholder secret url",
			yaml:    "type: webhook\nurl: <secret>\n",
			wantErr: true,
		},
		{
			// Wrong scheme should be rejected by SecretURL.UnmarshalYAML
			// itself (ParseURL only accepts http/https), so the error
			// surfaces before our validator runs.
			name:    "non-http scheme",
			yaml:    "type: webhook\nurl: ftp://example.com/\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o Output
			err := yaml.Unmarshal([]byte(tc.yaml), &o)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, o)
			}
		})
	}
}

func TestEventRecorderConfigEqual_Webhook(t *testing.T) {
	a := Config{Outputs: []Output{{
		Type:       OutputWebhook,
		URL:        mustParseURL(t, "https://example.com/hook"),
		Timeout:    model.Duration(10 * time.Second),
		Workers:    4,
		MaxRetries: 3,
	}}}
	b := Config{Outputs: []Output{{
		Type:       OutputWebhook,
		URL:        mustParseURL(t, "https://example.com/hook"),
		Timeout:    model.Duration(10 * time.Second),
		Workers:    4,
		MaxRetries: 3,
	}}}
	require.True(t, configEqual(a, b))

	// Differing URL.
	b.Outputs[0].URL = mustParseURL(t, "https://example.com/other")
	require.False(t, configEqual(a, b))
	b.Outputs[0].URL = a.Outputs[0].URL

	// Differing workers.
	b.Outputs[0].Workers = 8
	require.False(t, configEqual(a, b))
}
