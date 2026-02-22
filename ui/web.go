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
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"

	"github.com/prometheus/alertmanager/asset"
)

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router, reloadCh chan<- chan error, logger *slog.Logger) {
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	fs := http.FileServer(asset.Assets)
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/"
		fs.ServeHTTP(w, req)
	})

	r.Get("/script.js", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/script.js"
		fs.ServeHTTP(w, req)
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/favicon.ico"
		fs.ServeHTTP(w, req)
	})

	r.Get("/lib/*path", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = path.Join("/static/lib", route.Param(req.Context(), "path"))
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
