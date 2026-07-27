// Package config provides configuration loading utilities.
package config

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
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

func (f *fileLoader) Load(_ context.Context) ([]byte, error) {
    return os.ReadFile(f.path)
}

// httpLoader loads configuration via a simple HTTP GET request.
type httpLoader struct{ url string }

// NewHTTPLoader creates a ConfigLoader that fetches the configuration from the given URL.
func NewHTTPLoader(u string) ConfigLoader { return &httpLoader{url: u} }

func (h *httpLoader) Load(ctx context.Context) ([]byte, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
    if err != nil {
        return nil, err
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
    }
    return io.ReadAll(resp.Body)
}
