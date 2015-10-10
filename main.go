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
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

var (
	configFile    = flag.String("config.file", "config.yml", "The configuration file")
	dataDir       = flag.String("data.dir", "data/", "The data directory")
	listenAddress = flag.String("web.listen-address", ":9093", "Address to listen on for the web interface and API.")
)

func main() {
	flag.Parse()

	db, err := sql.Open("ql", filepath.Join(*dataDir, "am.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	alerts, err := provider.NewSQLAlerts(db)
	if err != nil {
		log.Fatal(err)
	}
	notifies, err := provider.NewSQLNotifyInfo(db)
	if err != nil {
		log.Fatal(err)
	}
	silences, err := provider.NewSQLSilences(db)
	if err != nil {
		log.Fatal(err)
	}

	inhibitor := &Inhibitor{alerts: alerts}

	routedNotifier := &notify.RoutedNotifier{}

	// Connect the pipeline of notifiers. Notifications will be sent
	// through them in inverted order.
	var notifier notify.Notifier
	notifier = &notify.LogNotifier{
		Log:      log.With("notifier", "routed"),
		Notifier: routedNotifier,
	}

	notifier = notify.NewDedupingNotifier(notifies, notifier)
	notifier = &notify.LogNotifier{
		Log:      log.With("notifier", "dedup"),
		Notifier: notifier,
	}

	notifier = &notify.MutingNotifier{
		Notifier: notifier,
		Muter:    inhibitor,
	}
	notifier = &notify.LogNotifier{
		Log:      log.With("notifier", "inhibit"),
		Notifier: notifier,
	}

	notifier = &notify.MutingNotifier{
		Notifier: notifier,
		Muter:    silences,
	}
	notifier = &notify.LogNotifier{
		Log:      log.With("notifier", "silencer"),
		Notifier: notifier,
	}

	build := func(conf *config.Config) {

		res := map[string]notify.Notifier{}

		for _, nc := range conf.NotificationConfigs {
			var all notify.Notifiers

			for _, wc := range nc.WebhookConfigs {
				all = append(all, &notify.LogNotifier{
					Log:      log.With("notifier", "webhook"),
					Notifier: notify.NewWebhook(wc),
				})
			}
			for _, ec := range nc.EmailConfigs {
				all = append(all, &notify.LogNotifier{
					Log:      log.With("notifier", "email"),
					Notifier: notify.NewEmail(ec),
				})
			}

			for i, nv := range all {
				n := nv

				n = &notify.RetryNotifier{Notifier: n}
				n = notify.NewDedupingNotifier(notifies, n)
				nn := notify.NotifyFunc(func(ctx context.Context, alerts ...*types.Alert) error {

					dest, ok := notify.Destination(ctx)
					if !ok {
						return fmt.Errorf("missing destination name")
					}
					dest = fmt.Sprintf("%s/%s/%d", dest, nc.Name, i)

					log.Debugln("destination new", dest)

					ctx = notify.WithDestination(ctx, dest)
					return n.Notify(ctx, alerts...)
				})

				all[i] = nn
			}

			res[nc.Name] = all
		}

		routedNotifier.Lock()
		log.Debugf("set notifiers for routes %#v", res)
		routedNotifier.Notifiers = res
		routedNotifier.Unlock()
	}

	disp := NewDispatcher(alerts, notifier)

	if err := reloadConfig(*configFile, disp, types.ReloadFunc(build), inhibitor); err != nil {
		log.Fatalf("Couldn't load configuration (-config.file=%s): %v", *configFile, err)
	}

	go disp.Run()
	defer disp.Stop()

	router := route.New()

	NewAPI(router.WithPrefix("/api/v1"), alerts, silences)

	go http.ListenAndServe(*listenAddress, router)

	var (
		hup  = make(chan os.Signal)
		term = make(chan os.Signal)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		for range hup {
			if err := reloadConfig(*configFile, disp, types.ReloadFunc(build), inhibitor); err != nil {
				log.Errorf("Couldn't load configuration (-config.file=%s): %v", *configFile, err)
			}
		}
	}()

	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
	os.Exit(0)
}

func reloadConfig(filename string, rls ...types.Reloadable) error {
	log.Infof("Loading configuration file %s", filename)

	conf, err := config.LoadFile(filename)
	if err != nil {
		return err
	}

	t := template.New("")
	for _, tpath := range conf.Templates {
		t, err = t.ParseGlob(tpath)
		if err != nil {
			return err
		}
	}

	notify.SetTemplate(t)

	for _, rl := range rls {
		rl.ApplyConfig(conf)
	}
	return nil
}
