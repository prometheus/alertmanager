// Copyright The Prometheus Authors
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

package app

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics bundles the process-level Prometheus metrics owned by the app
// package. They used to live as package-level variables in
// cmd/alertmanager/main.go and were registered against
// prometheus.DefaultRegisterer at init time. They are now constructed per
// app.Run invocation against the registerer supplied via Options so that
// multiple instances can coexist within a single process (e.g. tests).
type metrics struct {
	requestDuration           *prometheus.HistogramVec
	responseSize              *prometheus.HistogramVec
	clusterEnabled            prometheus.Gauge
	configuredReceivers       prometheus.Gauge
	configuredIntegrations    prometheus.Gauge
	configuredInhibitionRules prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	f := promauto.With(reg)
	return &metrics{
		requestDuration: f.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:                            "alertmanager_http_request_duration_seconds",
				Help:                            "Histogram of latencies for HTTP requests.",
				Buckets:                         prometheus.DefBuckets,
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  100,
				NativeHistogramMinResetDuration: 1 * time.Hour,
			},
			[]string{"handler", "method", "code"},
		),
		responseSize: f.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_http_response_size_bytes",
				Help:    "Histogram of response size for HTTP requests.",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"handler", "method"},
		),
		clusterEnabled: f.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_cluster_enabled",
				Help: "Indicates whether the clustering is enabled or not.",
			},
		),
		configuredReceivers: f.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_receivers",
				Help: "Number of configured receivers.",
			},
		),
		configuredIntegrations: f.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_integrations",
				Help: "Number of configured integrations.",
			},
		),
		configuredInhibitionRules: f.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_inhibition_rules",
				Help: "Number of configured inhibition rules.",
			},
		),
	}
}

func (m *metrics) instrumentHandler(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
	handlerLabel := prometheus.Labels{"handler": handlerName}
	return promhttp.InstrumentHandlerDuration(
		m.requestDuration.MustCurryWith(handlerLabel),
		promhttp.InstrumentHandlerResponseSize(
			m.responseSize.MustCurryWith(handlerLabel),
			handler,
		),
	)
}
