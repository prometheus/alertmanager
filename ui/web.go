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
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/route"
)

//go:embed app/dist
var asset embed.FS

var fileTypes = map[string]struct {
	contentType  string // https://www.iana.org/assignments/media-types/
	varyEncoding bool   // Must match build configuration in vite.config.mjs.
}{
	".css":   {"text/css; charset=utf-8", true},
	".eot":   {"application/vnd.ms-fontobject", true},
	".html":  {"text/html; charset=utf-8", true},
	".ico":   {"image/vnd.microsoft.icon", true},
	".js":    {"text/javascript; charset=utf-8", true},
	".svg":   {"image/svg+xml", true},
	".ttf":   {"font/ttf", true},
	".woff":  {"font/woff", false},
	".woff2": {"font/woff2", false},
}

type encoding int

const (
	encNone encoding = iota
	encGzip
	encBrotli
)

type tokenEffect int

const (
	effectUnseen tokenEffect = iota
	effectReject
	effectAccept
)

// selectEncoding parses the Accept-Encoding header and returns the preferred
// encoding. For simplicity, non-zero q-values are not ranked.
func selectEncoding(header string) encoding {
	brotli, gzip, wildcard := effectUnseen, effectUnseen, effectUnseen
	for part := range strings.SplitSeq(header, ",") {
		encAndQ := strings.SplitN(strings.TrimSpace(part), ";", 2)

		effect := effectAccept
		if len(encAndQ) > 1 {
			// https://www.rfc-editor.org/rfc/rfc9110.html#section-12.4.2
			if q, ok := strings.CutPrefix(strings.TrimSpace(encAndQ[1]), "q="); ok {
				if weight, err := strconv.ParseFloat(q, 64); err == nil && weight <= 0 {
					effect = effectReject
				}
			}
		}

		switch strings.TrimSpace(encAndQ[0]) {
		case "br":
			brotli = effect
		case "gzip":
			gzip = effect
		case "*":
			wildcard = effect
		}
	}

	if brotli == effectAccept || (wildcard == effectAccept && brotli == effectUnseen) {
		return encBrotli
	} else if gzip == effectAccept || (wildcard == effectAccept && gzip == effectUnseen) {
		return encGzip
	}
	return encNone
}

// decompressToReader decompresses f into memory. For simplicity the entire
// file is buffered; this is acceptable given the small size of the assets.
func decompressToReader(f fs.File) (*bytes.Reader, error) {
	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router) {
	appFS, err := fs.Sub(asset, "app/dist")
	if err != nil {
		panic(err) // During build step, we did not embed a directory named `app/dist`.
	}
	serve := func(w http.ResponseWriter, req *http.Request, filePath string, immutable bool) {
		ext := strings.ToLower(path.Ext(filePath))
		fileType, ok := fileTypes[ext]
		if !ok {
			http.NotFound(w, req)
			return
		}

		if fileType.varyEncoding {
			switch selectEncoding(req.Header.Get("Accept-Encoding")) {
			case encBrotli:
				if f, err := appFS.Open(filePath + ".br"); err == nil {
					defer f.Close()
					setCachePolicy(w, immutable)
					w.Header().Set("Content-Type", fileType.contentType)
					w.Header().Set("Content-Encoding", "br")
					w.Header().Add("Vary", "Accept-Encoding")
					http.ServeContent(w, req, filePath, time.Time{}, f.(io.ReadSeeker))
					return
				}
			case encGzip:
				if f, err := appFS.Open(filePath + ".gz"); err == nil {
					defer f.Close()
					setCachePolicy(w, immutable)
					w.Header().Set("Content-Type", fileType.contentType)
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Add("Vary", "Accept-Encoding")
					http.ServeContent(w, req, filePath, time.Time{}, f.(io.ReadSeeker))
					return
				}
			case encNone:
				if f, err := appFS.Open(filePath + ".gz"); err == nil {
					defer f.Close()
					uncompressedBytes, err := decompressToReader(f)
					if err != nil {
						http.Error(w, "failed to decompress file", http.StatusInternalServerError)
						return
					}
					setCachePolicy(w, immutable)
					w.Header().Set("Content-Type", fileType.contentType)
					w.Header().Add("Vary", "Accept-Encoding")
					http.ServeContent(w, req, filePath, time.Time{}, uncompressedBytes)
					return
				}
			}
		} else {
			if f, err := appFS.Open(filePath); err == nil {
				defer f.Close()
				setCachePolicy(w, immutable)
				w.Header().Set("Content-Type", fileType.contentType)
				http.ServeContent(w, req, filePath, time.Time{}, f.(io.ReadSeeker))
				return
			}
		}
		http.NotFound(w, req)
	}

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		serve(w, req, "index.html", false)
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		serve(w, req, "favicon.ico", false)
	})

	r.Get("/assets/*path", func(w http.ResponseWriter, req *http.Request) {
		serve(w, req, path.Join("assets", route.Param(req.Context(), "path")), true)
	})
}

func setCachePolicy(w http.ResponseWriter, immutable bool) {
	if immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0") // Prevent proxies from caching.
	}
}
