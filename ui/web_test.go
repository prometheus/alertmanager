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

package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

func TestSelectEncoding(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   encoding
	}{
		{"", encNone},
		{"gzip", encGzip},
		{"br", encBrotli},
		{"gzip, br", encBrotli},
		{"br, gzip", encBrotli},
		{"br;q=0", encNone},
		{"br;q=0.0", encNone},
		{"br;q=0.000", encNone},
		{"br;q=0, gzip", encGzip},
		{"gzip;q=0", encNone},
		{"gzip;q=0, br", encBrotli},
		{"br;q=0.1", encBrotli},
		{"gzip ; q=0", encNone},
		{"compress, gzip", encGzip},
		{"*", encBrotli},
		{"br;q=0, *", encGzip},
		{"br;q=0, gzip;q=0, *", encNone},
		{"*;q=0", encNone},
		{"compress;q=0.5, gzip;q=1.0", encGzip},
		{"gzip;q=1.0, identity; q=0.5, *;q=0", encGzip},
		{"gzip, deflate, br, zstd", encBrotli}, // Chrome
	} {
		t.Run(tt.header, func(t *testing.T) {
			require.Equal(t, tt.want, selectEncoding(tt.header))
		})
	}
}

func fetchIndexAndAssets(t *testing.T, router *route.Router, urlPath string) {
	t.Helper()
	res := fetchWithEncoding(router, urlPath, encNone)
	require.Equal(t, http.StatusOK, res.Code)

	re := regexp.MustCompile(`(?:src|href)="([^"]+)"`)
	matches := re.FindAllStringSubmatch(res.Body.String(), -1)
	require.NotEmpty(t, matches, "No assets (src/href) found in index.html. Is the build empty?")

	for _, match := range matches {
		assetPath := path.Join(urlPath, match[1])
		t.Run(assetPath, func(t *testing.T) {
			res := fetchWithEncoding(router, assetPath, encNone)
			require.Equal(t, http.StatusOK, res.Code)
		})
	}
}

func TestIndexAssetsAreServed(t *testing.T) {
	router := route.New()
	Register(router)
	fetchIndexAndAssets(t, router, "/")
}

// TestIndexAssetsAreServedPrefix tests the --web.route-prefix feature.
func TestIndexAssetsAreServedPrefix(t *testing.T) {
	router := route.New().WithPrefix("/alertmanager")
	Register(router)
	fetchIndexAndAssets(t, router, "/alertmanager/")
}

// walkEmbeddedFiles returns a map of URL to encodings for every file in
// app/dist.
func walkEmbeddedFiles(t *testing.T) map[string][]encoding {
	t.Helper()
	appFS, err := fs.Sub(asset, "app/dist")
	require.NoError(t, err)

	count := make(map[string]int)

	err = fs.WalkDir(appFS, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := strings.TrimSuffix(strings.TrimSuffix(filePath, ".gz"), ".br")
		count[base]++
		return nil
	})
	require.NoError(t, err)

	files := make(map[string][]encoding)
	for base, n := range count {
		url := "/" + base
		if base == "index.html" {
			url = "/"
		}
		ext := path.Ext(base)
		fileType, known := fileTypes[ext]
		require.True(t, known, "unknown extension %q for url %q", ext, url)
		if fileType.varyEncoding {
			require.Equal(t, 2, n, "expected .gz and .br for %q, got %d variant(s)", url, n)
			files[url] = []encoding{encGzip, encBrotli, encNone}
		} else {
			require.Equal(t, 1, n, "expected single file for uncompressed url %q, got %d", url, n)
			files[url] = []encoding{encNone}
		}
	}

	return files
}

func fetchWithEncoding(router *route.Router, urlPath string, encoding encoding) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, urlPath, nil)
	switch encoding {
	case encGzip:
		req.Header.Set("Accept-Encoding", "gzip")
	case encBrotli:
		req.Header.Set("Accept-Encoding", "br")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// A server SHOULD send Last-Modified only when the modification date can be
// reasonably and consistently determined.
// (RFC 9110 §8.8.2, https://www.rfc-editor.org/rfc/rfc9110#section-8.8.2)
// We omit it here because no such date is available for our embedded files.
// Both caching policies make this absence a non-issue in practice:
//   - "public, max-age=31536000, immutable": clients SHOULD skip conditional
//     revalidation during the freshness window. After max-age expires the
//     immutable directive no longer applies, but revalidation without a
//     Last-Modified simply results in a normal 200.
//     (RFC 8246 §2, https://www.rfc-editor.org/rfc/rfc8246#section-2)
//   - "no-cache, no-store, must-revalidate": this applies only to files for
//     which we always intend to send a fresh copy anyway. Returning a 304
//     would be a mistake.
//     (RFC 9111 §5.2, https://www.rfc-editor.org/rfc/rfc9111#section-5.2)
func checkCachingHeaders(t *testing.T, res *httptest.ResponseRecorder) {
	t.Helper()
	require.Empty(t, res.Header().Get("Last-Modified"))
	require.Contains(t, []string{
		"public, max-age=31536000, immutable",
		"no-cache, no-store, must-revalidate",
	}, res.Header().Get("Cache-Control"))
}

// TestAssetsNotFoundCacheHeader verifies that a 404 response from the
// /assets/* route does not carry a Cache-Control header. This could
// cause the error to be cached for a year.
func TestAssetsNotFoundCacheHeader(t *testing.T) {
	router := route.New()
	Register(router)

	res := fetchWithEncoding(router, "/assets/nonexistent.js", encNone)

	require.Equal(t, http.StatusNotFound, res.Code)
	require.Empty(t, res.Header().Get("Cache-Control"))
}

// TestWebRoutes walks the embedded FS and issues an HTTP request for every
// file, using the appropriate Accept-Encoding for compressed variants.
func TestWebRoutes(t *testing.T) {
	router := route.New()
	Register(router)

	files := walkEmbeddedFiles(t)

	require.Contains(t, files, "/")
	require.Contains(t, files, "/favicon.ico")

	for url, encodings := range files {
		t.Run(url, func(t *testing.T) {
			for _, encoding := range encodings {
				switch encoding {
				case encGzip:
					t.Run("gzip", func(t *testing.T) {
						res := fetchWithEncoding(router, url, encoding)
						require.Equal(t, http.StatusOK, res.Code)
						require.Equal(t, "gzip", res.Header().Get("Content-Encoding"))
						checkCachingHeaders(t, res)
					})
				case encBrotli:
					t.Run("br", func(t *testing.T) {
						res := fetchWithEncoding(router, url, encoding)
						require.Equal(t, http.StatusOK, res.Code)
						require.Equal(t, "br", res.Header().Get("Content-Encoding"))
						checkCachingHeaders(t, res)
					})
				case encNone:
					t.Run("none", func(t *testing.T) {
						res := fetchWithEncoding(router, url, encoding)
						require.Equal(t, http.StatusOK, res.Code)
						require.Empty(t, res.Header().Get("Content-Encoding"))
						checkCachingHeaders(t, res)
					})
				default:
					t.Fatalf("unhandled encoding %d", encoding)
				}
			}
		})
	}
}
