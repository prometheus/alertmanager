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
	"io"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/server"

	"github.com/prometheus/alertmanager/asset"
)

var newUIReactRouterPaths = []string{
	"/alerts",
	"/config",
	"/silences",
	"/status",
}

// Register registers handlers to serve files for the web interface.
func Register(r *route.Router, reloadCh chan<- chan error, logger *slog.Logger, enableUIV2 bool) {
	if enableUIV2 {
		RegisterUIV2(r, logger)
	} else {
		RegisterUIV1(r, logger)
	}

	r.Get("/metrics", promhttp.Handler().ServeHTTP)

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

func RegisterUIV1(r *route.Router, logger *slog.Logger) {
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
}

func RegisterUIV2(router *route.Router, logger *slog.Logger) {
	homePage := "/alerts"
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, homePage, http.StatusFound)
	})

	reactAssetsRoot := "/static/mantine-ui"

	serveReactApp := func(w http.ResponseWriter, _ *http.Request) {
		indexPath := reactAssetsRoot + "/index.html"
		f, err := Assets.Open(indexPath)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error opening React index.html: %v", err)
			return
		}
		defer func() { _ = f.Close() }()
		idx, err := io.ReadAll(f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error reading React index.html: %v", err)
			return
		}
		// replacedIdx := bytes.ReplaceAll(idx, []byte("CONSOLES_LINK_PLACEHOLDER"), []byte(h.consolesPath()))
		// replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("TITLE_PLACEHOLDER"), []byte(h.options.PageTitle))
		// replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("AGENT_MODE_PLACEHOLDER"), []byte(strconv.FormatBool(h.options.IsAgent)))
		// replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("READY_PLACEHOLDER"), []byte(strconv.FormatBool(h.isReady())))
		// replacedIdx = bytes.ReplaceAll(replacedIdx, []byte("LOOKBACKDELTA_PLACEHOLDER"), []byte(model.Duration(h.options.LookbackDelta).String()))
		w.Write(idx)
	}

	// Serve the React app.
	reactRouterPaths := newUIReactRouterPaths

	for _, p := range reactRouterPaths {
		router.Get(p, serveReactApp)
	}

	reactStaticAssetsDir := "/assets"
	// Static files required by the React app.
	router.Get(reactStaticAssetsDir+"/*filepath", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = path.Join(reactAssetsRoot+reactStaticAssetsDir, route.Param(r.Context(), "filepath"))
		fs := server.StaticFileServer(Assets)
		fs.ServeHTTP(w, r)
	})
}

func disableCaching(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0") // Prevent proxies from caching.
}
