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
	"io/fs"
	"net/http"

	"github.com/prometheus/common/route"
)

//go:embed app/script.js app/index.html app/favicon.ico app/lib
var asset embed.FS

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router) {
	appFS, err := fs.Sub(asset, "app")
	if err != nil {
		panic(err) // During build step, we did not embed a directory named `app`.
	}
	fs := http.FileServerFS(appFS)
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)
		fs.ServeHTTP(w, req)
	})

	r.Get("/script.js", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)
		fs.ServeHTTP(w, req)
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)
		fs.ServeHTTP(w, req)
	})

	r.Get("/lib/*path", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)
		fs.ServeHTTP(w, req)
	})
}

func disableCaching(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0") // Prevent proxies from caching.
}
