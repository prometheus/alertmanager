package reactApp

import (
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/go-kit/log"

	"github.com/prometheus/common/route"
	"github.com/prometheus/common/server"
)

var reactRouterPaths = []string{
	"/",
	"/status",
}

func Register(r *route.Router, logger log.Logger) {
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
