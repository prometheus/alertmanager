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

package config

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "alertmanager" // For Prometheus.
)

var configLoads = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: Namespace,
		Name:      "config_reloads_total",
		Help:      "The number of configuration reloads.",
	},
	[]string{"outcome"},
)

func init() {
	prometheus.MustRegister(configLoads)
}
