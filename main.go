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

package main

import (
	"bytes"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	template_text "text/template"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/alertmanager/version"
)

var (
	showVersion = flag.Bool("version", false, "Print version information.")

	configFile = flag.String("config.file", "alertmanager.yml", "The configuration file")
	dataDir    = flag.String("storage.path", "data/", "The data directory")

	externalURL   = flag.String("web.external-url", "", "The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.")
	listenAddress = flag.String("web.listen-address", ":9093", "Address to listen on for the web interface and API.")
)

func main() {
	flag.Parse()

	printVersion()
	if *showVersion {
		os.Exit(0)
	}

	err := os.MkdirAll(*dataDir, 0777)
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("ql", filepath.Join(*dataDir, "am.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	marker := types.NewMarker()

	alerts, err := provider.NewSQLAlerts(db)
	if err != nil {
		log.Fatal(err)
	}
	notifies, err := provider.NewSQLNotifyInfo(db)
	if err != nil {
		log.Fatal(err)
	}
	silences, err := provider.NewSQLSilences(db, marker)
	if err != nil {
		log.Fatal(err)
	}

	var (
		inhibitor *Inhibitor
		tmpl      *template.Template
		disp      *Dispatcher
	)
	defer disp.Stop()

	api := NewAPI(alerts, silences, func() AlertOverview {
		return disp.Groups()
	})

	build := func(rcvs []*config.Receiver) notify.Notifier {
		var (
			router  = notify.Router{}
			fanouts = notify.Build(rcvs, tmpl)
		)
		for name, fo := range fanouts {
			for i, n := range fo {
				n = notify.Retry(n)
				n = notify.Log(n, log.With("step", "retry"))
				n = notify.Dedup(notifies, n)
				n = notify.Log(n, log.With("step", "dedup"))

				fo[i] = n
			}
			router[name] = fo
		}
		var n notify.Notifier = router

		n = notify.Log(n, log.With("step", "route"))
		n = notify.Mute(silences, n)
		n = notify.Log(n, log.With("step", "silence"))
		n = notify.Mute(inhibitor, n)
		n = notify.Log(n, log.With("step", "inhibit"))

		return n
	}

	reload := func() (err error) {
		log.With("file", *configFile).Infof("Loading configuration file")
		defer func() {
			if err != nil {
				log.With("file", *configFile).Errorf("Loading configuration file failed: %s", err)
			}
		}()

		conf, err := config.LoadFile(*configFile)
		if err != nil {
			return err
		}

		api.Update(conf.String())

		tmpl, err = template.FromGlobs(conf.Templates...)
		if err != nil {
			return err
		}

		disp.Stop()

		inhibitor = NewInhibitor(alerts, conf.InhibitRules, marker)
		disp = NewDispatcher(alerts, NewRoute(conf.Route, nil), build(conf.Receivers), marker)

		go disp.Run()

		return nil
	}

	if err := reload(); err != nil {
		os.Exit(1)
	}

	router := route.New()

	RegisterWeb(router)
	api.Register(router.WithPrefix("/api"))

	go http.ListenAndServe(*listenAddress, router)

	var (
		hup  = make(chan os.Signal)
		term = make(chan os.Signal)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		for range hup {
			reload()
		}
	}()

	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
}

var versionInfoTmpl = `
alertmanager, version {{.version}} (branch: {{.branch}}, revision: {{.revision}})
  build user:       {{.buildUser}}
  build date:       {{.buildDate}}
  go version:       {{.goVersion}}
`

func printVersion() {
	t := template_text.Must(template_text.New("version").Parse(versionInfoTmpl))

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "version", version.Map); err != nil {
		panic(err)
	}
	fmt.Fprintln(os.Stdout, strings.TrimSpace(buf.String()))
}
