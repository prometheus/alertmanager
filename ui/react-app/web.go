// Copyright 2023 The Prometheus Authors
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

package reactapp

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"

	"github.com/prometheus/common/route"
	"github.com/prometheus/common/server"
)

var reactRouterPaths = []string{
	"/",
	"/status",
}

func Register(r *route.Router, logger *slog.Logger) {
	serveReactApp := func(w http.ResponseWriter, r *http.Request) {
		f, err := Assets.Open("/dist/index.html")
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
		w.Write(idx)
	}

	// Static files required by the React app.
	r.Get("/react-app/*filepath", func(w http.ResponseWriter, r *http.Request) {
		for _, rt := range reactRouterPaths {
			if r.URL.Path != "/react-app"+rt {
				continue
			}
			serveReactApp(w, r)
			return
		}
		r.URL.Path = path.Join("/dist", route.Param(r.Context(), "filepath"))
		fs := server.StaticFileServer(Assets)
		fs.ServeHTTP(w, r)
	})
}
