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
	"strings"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/alertmanager/web/api"
	"github.com/prometheus/alertmanager/web/blob"
)

// Commandline flags.
var (
	listenAddress  = flag.String("web.listen-address", ":9093", "Address to listen on for the web interface and API.")
	useLocalAssets = flag.Bool("web.use-local-assets", false, "Serve assets and templates from local files instead of from the binary.")
)

type WebService struct {
	AlertManagerService *api.AlertManagerService
	AlertsHandler       *AlertsHandler
	SilencesHandler     *SilencesHandler
	StatusHandler       *StatusHandler
}

func (w WebService) ServeForever(pathPrefix string) error {

	http.Handle(pathPrefix+"favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", 404)
	}))

	http.HandleFunc("/", prometheus.InstrumentHandlerFunc("index", func(rw http.ResponseWriter, req *http.Request) {
		// The "/" pattern matches everything, so we need to check
		// that we're at the root here.
		if req.URL.Path == pathPrefix {
			w.AlertsHandler.ServeHTTP(rw, req)
		} else if req.URL.Path == strings.TrimRight(pathPrefix, "/") {
			http.Redirect(rw, req, pathPrefix, http.StatusFound)
		} else if !strings.HasPrefix(req.URL.Path, pathPrefix) {
			// We're running under a prefix but the user requested something
			// outside of it. Let's see if this page exists under the prefix.
			http.Redirect(rw, req, pathPrefix+strings.TrimLeft(req.URL.Path, "/"), http.StatusFound)
		} else {
			http.NotFound(rw, req)
		}
	}))

	http.Handle(pathPrefix+"alerts", prometheus.InstrumentHandler("alerts", w.AlertsHandler))
	http.Handle(pathPrefix+"silences", prometheus.InstrumentHandler("silences", w.SilencesHandler))
	http.Handle(pathPrefix+"status", prometheus.InstrumentHandler("status", w.StatusHandler))

	http.Handle(pathPrefix+"metrics", prometheus.Handler())
	if *useLocalAssets {
		http.Handle(pathPrefix+"static/", http.StripPrefix(pathPrefix+"static/", http.FileServer(http.Dir("web/static"))))
	} else {
		http.Handle(pathPrefix+"static/", http.StripPrefix(pathPrefix+"static/", new(blob.Handler)))
	}
	http.Handle(pathPrefix+"api/", w.AlertManagerService.Handler())

	glog.Info("listening on ", *listenAddress)

	return http.ListenAndServe(*listenAddress, nil)
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
		glog.Error("Could not read base template: ", err)
		return nil, err
	}
	t.Parse(string(file))

	file, err = blob.GetFile(blob.TemplateFiles, name+".html")
	if err != nil {
		glog.Errorf("Could not read %s template: %s", name, err)
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
		glog.Error("Error preparing layout template: ", err)
		return
	}
	err = tpl.Execute(w, data)
	if err != nil {
		glog.Error("Error executing template: ", err)
	}
}
