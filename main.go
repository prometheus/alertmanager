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
	"os"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/manager"
	"github.com/prometheus/alertmanager/web"
	"github.com/prometheus/alertmanager/web/api"
)

var (
	configFile       = flag.String("configFile", "alertmanager.conf", "Alert Manager configuration file name.")
	silencesFile     = flag.String("silencesFile", "silences.json", "Silence storage file name.")
	minRefreshPeriod = flag.Duration("minRefreshPeriod", 5*time.Minute, "Minimum required alert refresh period before an alert is purged.")
)

func main() {
	flag.Parse()

	versionInfoTmpl.Execute(os.Stdout, BuildInfo)

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

	inhibitor := manager.NewInhibitor(conf.InhibitRules())

	alertManager := manager.NewMemoryAlertManager(*minRefreshPeriod, inhibitor, silencer, notifier)
	alertManager.SetAggregationRules(conf.AggregationRules())
	go alertManager.Run()

	// Web initialization.
	flags := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	statusHandler := &web.StatusHandler{
		Config:    conf.String(),
		Flags:     flags,
		BuildInfo: BuildInfo,
		Birth:     time.Now(),
	}

	webService := &web.WebService{
		// REST API Service.
		AlertManagerService: &api.AlertManagerService{
			Manager:  alertManager,
			Silencer: silencer,
		},

		// Template-based page handlers.
		AlertsHandler: &web.AlertsHandler{
			Manager:                alertManager,
			IsSilencedInterrogator: silencer,
		},
		SilencesHandler: &web.SilencesHandler{
			Silencer: silencer,
		},
		StatusHandler: statusHandler,
	}
	go webService.ServeForever()

	// React to configuration changes.
	watcher := config.NewFileWatcher(*configFile)
	go watcher.Watch(func(conf *config.Config) {
		inhibitor.SetInhibitRules(conf.InhibitRules())
		notifier.SetNotificationConfigs(conf.NotificationConfig)
		alertManager.SetAggregationRules(conf.AggregationRules())
		statusHandler.UpdateConfig(conf.String())
	})

	log.Println("Running notification dispatcher...")
	notifier.Dispatch()
}
