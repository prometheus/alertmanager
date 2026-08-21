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

// Package config provides configuration loading utilities.
package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultHTTPConfigTimeout = 30 * time.Second

// ConfigLoader abstracts where the raw configuration bytes come from.
type ConfigLoader interface {
	// Load returns the raw configuration bytes.
	Load(ctx context.Context) ([]byte, error)
	// Source returns an identifier for this loader suitable for logs (credentials redacted for HTTP).
	Source() string
}

// fileLoader loads configuration from a local file.
type fileLoader struct{ path string }

// NewFileLoader creates a ConfigLoader that reads from the given file path.
func NewFileLoader(p string) ConfigLoader { return &fileLoader{path: p} }

// Source implements ConfigLoader.
func (f *fileLoader) Source() string { return f.path }

// Load implements ConfigLoader for file-based configuration.
// It reads the configuration file from the filesystem and returns the raw bytes.
// Errors are wrapped to preserve the error chain for proper error handling.
func (f *fileLoader) Load(_ context.Context) ([]byte, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}
	return data, nil
}

// httpLoader loads configuration via a simple HTTP GET request.
type httpLoader struct{ url string }

// SanitizeURL redacts any credentials from the URL for logging purposes.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	sanitized := rawURL
	if parsed.User != nil {
		sanitized = parsed.Redacted()
	}

	if parsed.RawQuery != "" {
		sanitized = strings.Replace(sanitized, parsed.RawQuery, "[redacted]", 1)
	}

	return sanitized
}

// NewHTTPLoader creates a ConfigLoader that fetches the configuration from the given URL.
func NewHTTPLoader(u string) ConfigLoader { return &httpLoader{url: u} }

// Source implements ConfigLoader.
func (h *httpLoader) Source() string { return SanitizeURL(h.url) }

// Load implements ConfigLoader for HTTP-based configuration.
// It fetches the configuration from the specified HTTP URL with a request timeout.
// Errors are wrapped to preserve the error chain for proper error handling.
func (h *httpLoader) Load(ctx context.Context) ([]byte, error) {
	client := &http.Client{
		Timeout: defaultHTTPConfigTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	return data, nil
}
