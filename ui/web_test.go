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
	"testing"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"
)

func fetch(router *route.Router, urlPath string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, urlPath, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func fetchIndexAndAssets(t *testing.T, router *route.Router, urlPath string) {
	t.Helper()
	res := fetch(router, urlPath)
	require.Equal(t, http.StatusOK, res.Code)

	re := regexp.MustCompile(`(?:src|href)="([^"]+)"`)
	matches := re.FindAllStringSubmatch(res.Body.String(), -1)
	require.NotEmpty(t, matches, "No assets (src/href) found in index.html. Is the build empty?")

	for _, match := range matches {
		assetPath := path.Join(urlPath, match[1])
		t.Run(assetPath, func(t *testing.T) {
			res := fetch(router, assetPath)
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

// walkEmbeddedFiles returns a map of URL to file path for every file in the
// app/dist embed.FS. Each URL is guaranteed to be unique. The test fails if
// any file has no known route.
func walkEmbeddedFiles(t *testing.T) map[string]string {
	t.Helper()
	appFS, err := fs.Sub(asset, "app/dist")
	require.NoError(t, err)
	files := make(map[string]string)
	err = fs.WalkDir(appFS, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		url := "/" + filePath
		if filePath == "index.html" {
			url = "/"
		}
		require.NotContains(t, files, url, "duplicate URL %q in embedded FS", url)
		files[url] = filePath
		return nil
	})
	require.NoError(t, err)
	return files
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

	res := fetch(router, "/assets/nonexistent.js")

	require.Equal(t, http.StatusNotFound, res.Code)
	require.Empty(t, res.Header().Get("Cache-Control"))
}

// TestWebRoutes walks the embedded FS and issues an HTTP request for every file.
func TestWebRoutes(t *testing.T) {
	router := route.New()
	Register(router)

	files := walkEmbeddedFiles(t)

	require.Contains(t, files, "/")
	require.Contains(t, files, "/favicon.ico")

	for url := range files {
		t.Run(url, func(t *testing.T) {
			res := fetch(router, url)
			require.Equal(t, http.StatusOK, res.Code)
			checkCachingHeaders(t, res)
		})
	}
}
