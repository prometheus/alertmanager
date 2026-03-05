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
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"
)

//go:embed app/script.js app/index.html app/favicon.ico app/lib
var asset embed.FS

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router, reloadCh chan<- chan error, logger *slog.Logger) {
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

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

	r.Post("/-/reload", func(w http.ResponseWriter, req *http.Request) {
		errc := make(chan error)
		defer close(errc)

		reloadCh <- errc
		if err := <-errc; err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
		}
	})

	r.Get("/-/healthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	r.Head("/-/healthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/-/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	r.Head("/-/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	debugHandlerFunc := func(w http.ResponseWriter, req *http.Request) {
		subpath := route.Param(req.Context(), "subpath")
		req.URL.Path = path.Join("/debug", subpath)
		// path.Join removes trailing slashes, but some pprof handlers expect them.
		if strings.HasSuffix(subpath, "/") && !strings.HasSuffix(req.URL.Path, "/") {
			req.URL.Path += "/"
		}
		http.DefaultServeMux.ServeHTTP(w, req)
	}
	r.Get("/debug/*subpath", debugHandlerFunc)
	r.Post("/debug/*subpath", debugHandlerFunc)
}

func disableCaching(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0") // Prevent proxies from caching.
}
