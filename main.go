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

	"github.com/prometheus/alert_manager/config"
	"github.com/prometheus/alert_manager/manager"
	"github.com/prometheus/alert_manager/web"
	"github.com/prometheus/alert_manager/web/api"
)

var (
	configFile = flag.String("configFile", "alertmanager.conf", "Alert Manager configuration file name.")
)

func main() {
	flag.Parse()

	conf, err := config.LoadFromFile(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration from %s: %s", *configFile, err)
	}

	suppressor := manager.NewSuppressor()
	defer suppressor.Close()

	summarizer := manager.NewSummaryDispatcher()

	aggregator := manager.NewAggregator(summarizer)
	defer aggregator.Close()

	webService := &web.WebService{
		// REST API Service.
		AlertManagerService: &api.AlertManagerService{
			Aggregator: aggregator,
			Suppressor: suppressor,
		},

		// Template-based page handlers.
		AlertsHandler: &web.AlertsHandler{
			Aggregator:              aggregator,
			IsInhibitedInterrogator: suppressor,
		},
		SilencesHandler: &web.SilencesHandler{
			Suppressor: suppressor,
		},
	}
	go webService.ServeForever()

  aggregator.SetRules(conf.AggregationRules())

	log.Println("Running summary dispatcher...")
	summarizer.Dispatch(suppressor)
}
