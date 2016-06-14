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
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/boltmem"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var (
	showVersion = flag.Bool("version", false, "Print version information.")

	configFile = flag.String("config.file", "alertmanager.yml", "Alertmanager configuration file name.")
	dataDir    = flag.String("storage.path", "data/", "Base path for data storage.")

	externalURL   = flag.String("web.external-url", "", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.")
	listenAddress = flag.String("web.listen-address", ":9093", "Address to listen on for the web interface and API.")
)

var (
	configSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "alertmanager",
		Name:      "config_last_reload_successful",
		Help:      "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "alertmanager",
		Name:      "config_last_reload_success_timestamp_seconds",
		Help:      "Timestamp of the last successful configuration reload.",
	})
)

func init() {
	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)
	prometheus.MustRegister(version.NewCollector("alertmanager"))
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("alertmanager"))
		os.Exit(0)
	}

	log.Infoln("Starting alertmanager", version.Info())
	log.Infoln("Build context", version.BuildContext())

	err := os.MkdirAll(*dataDir, 0777)
	if err != nil {
		log.Fatal(err)
	}

	marker := types.NewMarker()

	alerts, err := boltmem.NewAlerts(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer alerts.Close()

	notifies, err := boltmem.NewNotificationInfo(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer notifies.Close()

	silences, err := boltmem.NewSilences(*dataDir, marker)
	if err != nil {
		log.Fatal(err)
	}
	defer silences.Close()

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
		n := notify.Notifier(router)

		n = notify.Log(n, log.With("step", "route"))
		n = notify.Silence(silences, n, marker)
		n = notify.Log(n, log.With("step", "silence"))
		n = notify.Inhibit(inhibitor, n, marker)
		n = notify.Log(n, log.With("step", "inhibit"))

		return n
	}

	amURL, err := extURL(*externalURL)
	if err != nil {
		log.Fatal(err)
	}

	reload := func() (err error) {
		log.With("file", *configFile).Infof("Loading configuration file")
		defer func() {
			if err != nil {
				log.With("file", *configFile).Errorf("Loading configuration file failed: %s", err)
				configSuccess.Set(0)
			} else {
				configSuccess.Set(1)
				configSuccessTime.Set(float64(time.Now().Unix()))
			}
		}()

		conf, err := config.LoadFile(*configFile)
		if err != nil {
			return err
		}

		api.Update(conf.String(), time.Duration(conf.Global.ResolveTimeout))

		tmpl, err = template.FromGlobs(conf.Templates...)
		if err != nil {
			return err
		}
		tmpl.ExternalURL = amURL

		inhibitor.Stop()
		disp.Stop()

		inhibitor = NewInhibitor(alerts, conf.InhibitRules, marker)
		disp = NewDispatcher(alerts, NewRoute(conf.Route, nil), build(conf.Receivers), marker)

		go disp.Run()
		go inhibitor.Run()

		return nil
	}

	if err := reload(); err != nil {
		os.Exit(1)
	}

	router := route.New()

	webReload := make(chan struct{})
	RegisterWeb(router.WithPrefix(amURL.Path), webReload)
	api.Register(router.WithPrefix(path.Join(amURL.Path, "/api")))

	log.Infoln("Listening on", *listenAddress)
	go listen(router)

	var (
		hup      = make(chan os.Signal)
		hupReady = make(chan bool)
		term     = make(chan os.Signal)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-hupReady
		for {
			select {
			case <-hup:
			case <-webReload:
			}
			reload()
		}
	}()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
}

func extURL(s string) (*url.URL, error) {
	if s == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(*listenAddress)
		if err != nil {
			return nil, err
		}

		s = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(u.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	u.Path = ppref

	return u, nil
}

func listen(router *route.Router) {
	if err := http.ListenAndServe(*listenAddress, router); err != nil {
		log.Fatal(err)
	}
}
