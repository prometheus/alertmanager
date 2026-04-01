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
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/prometheus/common/route"
)

//go:embed app/dist
var asset embed.FS

// https://www.iana.org/assignments/media-types/
var fileTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".eot":   "application/vnd.ms-fontobject",
	".html":  "text/html; charset=utf-8",
	".ico":   "image/vnd.microsoft.icon",
	".js":    "text/javascript; charset=utf-8",
	".svg":   "image/svg+xml",
	".ttf":   "font/ttf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router) {
	appFS, err := fs.Sub(asset, "app/dist")
	if err != nil {
		panic(err) // During build step, we did not embed a directory named `app/dist`.
	}
	serve := func(w http.ResponseWriter, req *http.Request, filePath string, immutable bool) {
		ext := strings.ToLower(path.Ext(filePath))
		contentType, ok := fileTypes[ext]
		if !ok {
			http.NotFound(w, req)
			return
		}

		f, err := appFS.Open(filePath)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		defer f.Close()

		setCachePolicy(w, immutable)
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, req, filePath, time.Time{}, f.(io.ReadSeeker))
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
