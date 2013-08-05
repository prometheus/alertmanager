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

package main

import (
	"flag"
	"log"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/manager"
	"github.com/prometheus/alertmanager/web"
	"github.com/prometheus/alertmanager/web/api"
)

var (
	configFile   = flag.String("configFile", "alertmanager.conf", "Alert Manager configuration file name.")
	silencesFile = flag.String("silencesFile", "silences.json", "Silence storage file name.")
)

func main() {
	flag.Parse()

	conf := config.MustLoadFromFile(*configFile)

	silencer := manager.NewSilencer()
	defer silencer.Close()

	err := silencer.LoadFromFile(*silencesFile)
	if err != nil {
		log.Println("Couldn't load silences, starting up with empty silence list:", err)
	}
	saveSilencesTicker := time.NewTicker(10 * time.Second)
	go func() {
		for _ = range saveSilencesTicker.C {
			if err := silencer.SaveToFile(*silencesFile); err != nil {
				log.Println("Error saving silences to file:", err)
			}
		}
	}()
	defer saveSilencesTicker.Stop()

	notifier := manager.NewNotifier(conf.NotificationConfig)
	defer notifier.Close()

	aggregator := manager.NewAggregator(notifier)
	defer aggregator.Close()

	webService := &web.WebService{
		// REST API Service.
		AlertManagerService: &api.AlertManagerService{
			Aggregator: aggregator,
			Silencer:   silencer,
		},

		// Template-based page handlers.
		AlertsHandler: &web.AlertsHandler{
			Aggregator:              aggregator,
			IsInhibitedInterrogator: silencer,
		},
		SilencesHandler: &web.SilencesHandler{
			Silencer: silencer,
		},
	}
	go webService.ServeForever()

	aggregator.SetRules(conf.AggregationRules())

	watcher := config.NewFileWatcher(*configFile)
	go watcher.Watch(func(conf *config.Config) {
		notifier.SetNotificationConfigs(conf.NotificationConfig)
		aggregator.SetRules(conf.AggregationRules())
	})

	log.Println("Running summary dispatcher...")
	notifier.Dispatch(silencer)
}
