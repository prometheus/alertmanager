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
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"

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

	routedNotifier := notify.NewRoutedNotifier(func(confs []*config.NotificationConfig) map[string]notify.Notifier {
		res := notify.Build(confs)
		for name, n := range res {
			n = &notify.LogNotifier{
				Log:      log.With("notifier", fmt.Sprintf("%T", n)),
				Notifier: n,
			}
			res[name] = &notify.LogNotifier{
				Log:      log.With("notifier", "dedup"),
				Notifier: notify.NewDedupingNotifier(notifies, n),
			}
		}
		return res
	})

	var notifier notify.Notifier
	notifier = &notify.LogNotifier{
		Log:      log.With("notifier", "routed"),
		Notifier: routedNotifier,
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

	disp := NewDispatcher(alerts, notifier)

	if !reloadConfig(*configFile, disp, routedNotifier, inhibitor) {
		os.Exit(1)
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
			reloadConfig(*configFile, disp, routedNotifier, inhibitor)
		}
	}()

	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
	os.Exit(0)
}

func reloadConfig(filename string, rls ...types.Reloadable) (success bool) {
	log.Infof("Loading configuration file %s", filename)

	conf, err := config.LoadFile(filename)
	if err != nil {
		log.Errorf("Couldn't load configuration (-config.file=%s): %v", filename, err)
		return false
	}
	success = true

	for _, rl := range rls {
		success = success && rl.ApplyConfig(conf)
	}
	return success
}
