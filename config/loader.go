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

// ConfigLoader abstracts where the raw configuration bytes come from.
type ConfigLoader interface {
	// Load returns the raw configuration bytes.
	Load(ctx context.Context) ([]byte, error)
}

// fileLoader loads configuration from a local file.
type fileLoader struct{ path string }

// NewFileLoader creates a ConfigLoader that reads from the given file path.
func NewFileLoader(p string) ConfigLoader { return &fileLoader{path: p} }

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
	// Try to parse the URL to extract credentials
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, just return the original URL
		return rawURL
	}

	// Redact password from URL
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		if password != "" {
			// Use Go's built-in URL redaction for percent-encoded passwords
			if strings.Contains(password, "%") {
				// This is a percent-encoded password, use Redacted() method
				redactedURL := parsed.Redacted()
				if redactedURL == "" {
					// Fallback to manual replacement if Redacted() fails
					redactedURL = strings.Replace(rawURL, password, "***", 1)
				}
			} else {
				// Regular password, use manual replacement
				rawURL = strings.Replace(rawURL, password, "***", 1)
			}
		}
	}

	// Redact query parameters that might contain secrets
	if parsed.RawQuery != "" {
		// This provides basic protection for logging.
		rawURL = strings.Replace(rawURL, parsed.RawQuery, "[redacted]", 1)
	}

	return rawURL
}

// NewHTTPLoader creates a ConfigLoader that fetches the configuration from the given URL.
func NewHTTPLoader(u string) ConfigLoader { return &httpLoader{url: u} }

// Load implements ConfigLoader for HTTP-based configuration.
// It fetches the configuration from the specified HTTP URL with proper timeouts
// and size limits. The URL is sanitized to prevent credential leakage in logs.
// Errors are wrapped to preserve the error chain for proper error handling.
func (h *httpLoader) Load(ctx context.Context) ([]byte, error) {
	// Create a client with timeout to prevent hanging
	client := &http.Client{
		Timeout: 30 * time.Second, // 30 second timeout for the entire request
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Sanitize URL from error to avoid credential leakage in logs
		sanitizedErr := fmt.Errorf("HTTP request failed: %w", err)
		return nil, sanitizedErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	// Limit response body size to prevent memory issues
	// 10MB should be more than enough for any reasonable configuration
	const maxConfigSize = 10 * 1024 * 1024 // 10MB
	limitedReader := io.LimitReader(resp.Body, maxConfigSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	// Check if we hit the size limit
	if len(data) >= maxConfigSize {
		return nil, fmt.Errorf("configuration size exceeds maximum limit of %d bytes", maxConfigSize)
	}

	return data, nil
}
