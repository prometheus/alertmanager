// Copyright 2013 Prometheus Team
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

package web

import (
	"flag"
	"fmt"
	"html/template"
	"net/http"
	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/route"
	"github.com/prometheus/log"

	"github.com/prometheus/alertmanager/web/api"
	"github.com/prometheus/alertmanager/web/blob"
)

// Commandline flags.
var (
	useLocalAssets = flag.Bool("web.use-local-assets", false, "Serve assets and templates from local files instead of from the binary.")
)

type WebService struct {
	AlertManagerService *api.AlertManagerService
	AlertsHandler       *AlertsHandler
	SilencesHandler     *SilencesHandler
	StatusHandler       *StatusHandler
}

func (w WebService) ServeForever(addr string, pathPrefix string) error {
	router := route.New()

	if pathPrefix != "" {
		// If the prefix is missing for the root path, prepend it.
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, pathPrefix, http.StatusFound)
		})
		router = router.WithPrefix(pathPrefix)
	}

	instrf := prometheus.InstrumentHandlerFunc
	instrh := prometheus.InstrumentHandler

	router.Get("/alerts", instrh("alerts", w.AlertsHandler))
	router.Get("/silences", instrh("silences", w.SilencesHandler))
	router.Get("/status", instrh("status", w.StatusHandler))

	router.Get("/metrics", prometheus.Handler().ServeHTTP)

	if *useLocalAssets {
		router.Get("/static/*filepath", instrf("static", route.FileServe("web/blob/static")))
	} else {
		router.Get("/static/*filepath", instrh("static", blob.Handler{}))
	}

	w.AlertManagerService.Register(router.WithPrefix("/api"))

	log.Info("listening on ", addr)

	return http.ListenAndServe(addr, nil)
}

func getLocalTemplate(name string, pathPrefix string) (*template.Template, error) {
	t := template.New("_base.html")
	t.Funcs(webHelpers)
	t.Funcs(template.FuncMap{"pathPrefix": func() string { return pathPrefix }})

	return t.ParseFiles(
		"web/templates/_base.html",
		fmt.Sprintf("web/templates/%s.html", name),
	)
}

func getEmbeddedTemplate(name string, pathPrefix string) (*template.Template, error) {
	t := template.New("_base.html")
	t.Funcs(webHelpers)
	t.Funcs(template.FuncMap{"pathPrefix": func() string { return pathPrefix }})

	file, err := blob.GetFile(blob.TemplateFiles, "_base.html")
	if err != nil {
		log.Error("Could not read base template: ", err)
		return nil, err
	}
	t.Parse(string(file))

	file, err = blob.GetFile(blob.TemplateFiles, name+".html")
	if err != nil {
		log.Errorf("Could not read %s template: %s", name, err)
		return nil, err
	}
	t.Parse(string(file))

	return t, nil
}

func getTemplate(name string, pathPrefix string) (t *template.Template, err error) {
	if *useLocalAssets {
		t, err = getLocalTemplate(name, pathPrefix)
	} else {
		t, err = getEmbeddedTemplate(name, pathPrefix)
	}

	if err != nil {
		return nil, err
	}

	return t, nil
}

func executeTemplate(w http.ResponseWriter, name string, data interface{}, pathPrefix string) {
	tpl, err := getTemplate(name, pathPrefix)
	if err != nil {
		log.Error("Error preparing layout template: ", err)
		return
	}
	err = tpl.Execute(w, data)
	if err != nil {
		log.Error("Error executing template: ", err)
	}
}
