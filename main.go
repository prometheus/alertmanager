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
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
)

var (
	configFile = flag.String("config.file", "config.yml", "The configuration file")
)

func main() {
	flag.Parse()

	conf, err := config.LoadFile(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	data := provider.NewMemData()

	alerts := provider.NewMemAlerts(data)
	notifies := provider.NewMemNotifies(data)
	silences := provider.NewMemSilences()

	inhibitor := &Inhibitor{alerts: alerts}
	inhibitor.ApplyConfig(conf)

	routedNotifier := newRoutedNotifier(func(conf *config.Config) map[string]Notifier {
		res := map[string]Notifier{}
		for _, cn := range conf.NotificationConfigs {
			res[cn.Name] = newDedupingNotifier(notifies, &LogNotifier{name: cn.Name})
		}
		return res
	})
	routedNotifier.ApplyConfig(conf)

	var notifier Notifier
	notifier = routedNotifier
	notifier = &mutingNotifier{
		notifier: notifier,
		Muter:    inhibitor,
	}
	notifier = &mutingNotifier{
		notifier: notifier,
		Muter:    silences,
	}

	disp := NewDispatcher(alerts, notifier)

	disp.ApplyConfig(conf)
	go disp.Run()
	defer disp.Stop()

	router := route.New()

	NewAPI(router.WithPrefix("/api"), alerts, silences)

	go http.ListenAndServe(":9091", router)

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
	os.Exit(0)
}
