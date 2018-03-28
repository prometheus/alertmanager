// Copyright 2015 Prometheus Team
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
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"
)

func serveAsset(w http.ResponseWriter, req *http.Request, fp string, logger log.Logger) {
	info, err := AssetInfo(fp)
	if err != nil {
		level.Warn(logger).Log("msg", "Could not get file", "err", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	file, err := Asset(fp)
	if err != nil {
		if err != io.EOF {
			level.Warn(logger).Log("msg", "Could not get file", "file", fp, "err", err)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.ServeContent(w, req, info.Name(), info.ModTime(), bytes.NewReader(file))
}

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router, reloadCh chan<- chan error, logger log.Logger) {
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		serveAsset(w, req, "ui/app/index.html", logger)
	}))

	r.Get("/script.js", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		serveAsset(w, req, "ui/app/script.js", logger)
	}))

	r.Get("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		serveAsset(w, req, "ui/app/favicon.ico", logger)
	}))

	r.Get("/lib/*filepath", http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			fp := route.Param(req.Context(), "filepath")
			serveAsset(w, req, filepath.Join("ui/app/lib", fp), logger)
		},
	))

	r.Post("/-/reload", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		errc := make(chan error)
		defer close(errc)

		reloadCh <- errc
		if err := <-errc; err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
		}
	}))

	r.Get("/-/healthy", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	}))
	r.Get("/-/ready", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	}))

	r.Get("/debug/*subpath", http.DefaultServeMux.ServeHTTP)
	r.Post("/debug/*subpath", http.DefaultServeMux.ServeHTTP)
}
