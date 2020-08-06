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
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"os"
	"path"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"

	"github.com/prometheus/alertmanager/asset"
	"github.com/prometheus/alertmanager/config"
)

const (
	CONF_HTTP_MIME_TYPE = "text/vnd.yaml"
)

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router, curConf func() config.Config, confPath string, reloadCh chan<- chan error, logger log.Logger) {
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/"
		fs := http.FileServer(asset.Assets)
		fs.ServeHTTP(w, req)
	})

	r.Get("/script.js", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/script.js"
		fs := http.FileServer(asset.Assets)
		fs.ServeHTTP(w, req)
	})

	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = "/static/favicon.ico"
		fs := http.FileServer(asset.Assets)
		fs.ServeHTTP(w, req)
	})

	r.Get("/lib/*path", func(w http.ResponseWriter, req *http.Request) {
		disableCaching(w)

		req.URL.Path = path.Join("/static/lib", route.Param(req.Context(), "path"))
		fs := http.FileServer(asset.Assets)
		fs.ServeHTTP(w, req)
	})

	r.Post("/-/reload", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		errc := make(chan error)
		defer close(errc)

		reloadCh <- errc
		if err := <-errc; err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
		}
	}))

	r.Post("/-/config", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		errc := make(chan error)
		defer close(errc)

		if CONF_HTTP_MIME_TYPE != req.Header.Get("Content-Type") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		configFile, err := os.OpenFile(confPath, os.O_RDWR|os.O_CREATE, 0644)
		defer configFile.Close()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to open old config: %s", err), http.StatusInternalServerError)
		}

		if _, err := io.Copy(configFile, req.Body); err != nil {
			http.Error(w, fmt.Sprintf("failed to sync config: %s", err), http.StatusInternalServerError)
		}

		reloadCh <- errc
		if err := <-errc; err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
		}
	}))

	r.Get("/-/config", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", CONF_HTTP_MIME_TYPE)
		w.Write([]byte(curConf().String()))
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

func disableCaching(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0") // Prevent proxies from caching.
}
