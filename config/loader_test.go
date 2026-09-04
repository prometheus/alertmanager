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

package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func TestFileLoader(t *testing.T) {
	t.Run("successful load", func(t *testing.T) {
		loader := NewFileLoader("testdata/conf.good.yml")
		data, err := loader.Load(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, data)
	})

	t.Run("missing file", func(t *testing.T) {
		loader := NewFileLoader("testdata/nonexistent.yml")
		_, err := loader.Load(context.Background())
		require.Error(t, err)
	})

	t.Run("unreadable file", func(t *testing.T) {
		// Use a directory path which is guaranteed to fail when trying to read as a file
		dir := t.TempDir()
		loader := NewFileLoader(dir) // Directory paths cannot be read as files
		_, err := loader.Load(context.Background())
		require.Error(t, err)
	})
}

func TestHTTPLoader(t *testing.T) {
	t.Run("successful HTTP 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("route:\n  receiver: test\nreceivers:\n- name: test"))
		}))
		defer srv.Close()

		loader := NewHTTPLoader(srv.URL)
		data, err := loader.Load(context.Background())
		require.NoError(t, err)
		require.Contains(t, string(data), "receiver: test")
	})

	t.Run("HTTP 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		loader := NewHTTPLoader(srv.URL)
		_, err := loader.Load(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected HTTP status 404")
	})

	t.Run("HTTP 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		loader := NewHTTPLoader(srv.URL)
		_, err := loader.Load(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected HTTP status 500")
	})

	t.Run("network failure", func(t *testing.T) {
		loader := NewHTTPLoader("http://127.0.0.1:99999")
		_, err := loader.Load(context.Background())
		require.Error(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Never respond
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		loader := NewHTTPLoader(srv.URL)
		_, err := loader.Load(ctx)
		require.Error(t, err)
	})

	t.Run("unreadable response body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid: ["))
		}))
		defer srv.Close()

		loader := NewHTTPLoader(srv.URL)
		data, err := loader.Load(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, data)
	})

	t.Run("percent-encoded credentials", func(t *testing.T) {
		// Test with percent-encoded credentials in URL
		username := "testuser"
		password := "p%40ssw%40rd" // p@ssw@rd with @ symbols percent-encoded
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("route:\n  receiver: test\nreceivers:\n- name: test"))
		}))
		defer srv.Close()

		// Create URL with percent-encoded credentials
		urlWithCreds := srv.URL + "?username=" + username + "&password=" + password
		loader := NewHTTPLoader(urlWithCreds)
		data, err := loader.Load(context.Background())
		require.NoError(t, err)
		require.Contains(t, string(data), "receiver: test")
	})
}

func TestCoordinatorReloadWithHTTP(t *testing.T) {
	// Start a mutable HTTP server that returns a valid config.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("route:\n  receiver: test\nreceivers:\n- name: test"))
	}))
	defer srv.Close()

	loader := NewHTTPLoader(srv.URL)
	coord := NewCoordinator(loader, prometheus.NewRegistry(), promslog.NewNopLogger())

	var called bool
	coord.Subscribe(func(*Config) error {
		called = true
		return nil
	})

	err := coord.Reload()
	require.NoError(t, err)
	require.True(t, called)
}

func TestCoordinatorReloadWithHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	loader := NewHTTPLoader(srv.URL)
	coord := NewCoordinator(loader, prometheus.NewRegistry(), promslog.NewNopLogger())

	var called bool
	coord.Subscribe(func(*Config) error {
		called = true
		return nil
	})

	err := coord.Reload()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected HTTP status 500")
	require.False(t, called)
}
