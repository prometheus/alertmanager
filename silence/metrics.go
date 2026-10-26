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

package silence

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	gcDuration                            prometheus.Summary
	gcErrorsTotal                         prometheus.Counter
	snapshotDuration                      prometheus.Summary
	snapshotSize                          prometheus.Gauge
	queriesTotal                          prometheus.Counter
	queryErrorsTotal                      prometheus.Counter
	queryDuration                         prometheus.Histogram
	queryScannedTotal                     prometheus.Counter
	querySkippedTotal                     prometheus.Counter
	silencesActive                        prometheus.GaugeFunc
	silencesPending                       prometheus.GaugeFunc
	silencesExpired                       prometheus.GaugeFunc
	stateSize                             prometheus.Gauge
	matcherIndexSize                      prometheus.Gauge
	versionIndexSize                      prometheus.Gauge
	propagatedMessagesTotal               prometheus.Counter
	maintenanceTotal                      prometheus.Counter
	maintenanceErrorsTotal                prometheus.Counter
	matcherCompileIndexSilenceErrorsTotal prometheus.Counter
	matcherCompileLoadSnapshotErrorsTotal prometheus.Counter
}

func newSilenceMetricByState(r prometheus.Registerer, s *Silences, st SilenceState) prometheus.GaugeFunc {
	return promauto.With(r).NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "alertmanager_silences",
			Help:        "How many silences by state.",
			ConstLabels: prometheus.Labels{"state": string(st)},
		},
		func() float64 {
			count, err := s.CountState(context.Background(), st)
			if err != nil {
				s.logger.Error("Counting silences failed", "err", err)
			}
			return float64(count)
		},
	)
}

func newMetrics(r prometheus.Registerer, s *Silences) *metrics {
	m := &metrics{}

	m.gcDuration = promauto.With(r).NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_silences_gc_duration_seconds",
		Help:       "Duration of the last silence garbage collection cycle.",
		Objectives: map[float64]float64{},
	})
	m.gcErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_gc_errors_total",
		Help: "How many silence GC errors were encountered.",
	})
	m.snapshotDuration = promauto.With(r).NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_silences_snapshot_duration_seconds",
		Help:       "Duration of the last silence snapshot.",
		Objectives: map[float64]float64{},
	})
	m.snapshotSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_silences_snapshot_size_bytes",
		Help: "Size of the last silence snapshot in bytes.",
	})
	m.maintenanceTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_maintenance_total",
		Help: "How many maintenances were executed for silences.",
	})
	m.maintenanceErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_maintenance_errors_total",
		Help: "How many maintenances were executed for silences that failed.",
	})
	matcherCompileErrorsTotal := promauto.With(r).NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_silences_matcher_compile_errors_total",
			Help: "How many silence matcher compilations failed.",
		},
		[]string{"stage"},
	)
	m.matcherCompileIndexSilenceErrorsTotal = matcherCompileErrorsTotal.WithLabelValues("index")
	m.matcherCompileLoadSnapshotErrorsTotal = matcherCompileErrorsTotal.WithLabelValues("load_snapshot")
	m.queriesTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_queries_total",
		Help: "How many silence queries were received.",
	})
	m.queryErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_errors_total",
		Help: "How many silence received queries did not succeed.",
	})
	m.queryDuration = promauto.With(r).NewHistogram(prometheus.HistogramOpts{
		Name:                            "alertmanager_silences_query_duration_seconds",
		Help:                            "Duration of silence query evaluation.",
		Buckets:                         prometheus.DefBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})
	m.queryScannedTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_silences_scanned_total",
		Help: "How many silences were scanned during query evaluation.",
	})
	m.querySkippedTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_silences_skipped_total",
		Help: "How many silences were skipped during query evaluation using the version index.",
	})
	m.propagatedMessagesTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_gossip_messages_propagated_total",
		Help: "Number of received gossip messages that have been further gossiped.",
	})
	if s != nil {
		m.silencesActive = newSilenceMetricByState(r, s, SilenceStateActive)
		m.silencesPending = newSilenceMetricByState(r, s, SilenceStatePending)
		m.silencesExpired = newSilenceMetricByState(r, s, SilenceStateExpired)
		m.stateSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_state_size",
			Help: "The number of silences in the state map.",
		})
		m.matcherIndexSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_matcher_index_size",
			Help: "The number of entries in the matcher cache index.",
		})
		m.versionIndexSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_version_index_size",
			Help: "The number of entries in the version index.",
		})
	}

	return m
}

// updateSizeMetrics updates the size metrics for state, matcher index, and version index.
// Must be called while holding s.mtx.
func (s *Silences) updateSizeMetrics() {
	if s.metrics != nil && s.metrics.stateSize != nil {
		s.metrics.stateSize.Set(float64(len(s.st)))
		s.metrics.matcherIndexSize.Set(float64(len(s.mi)))
		s.metrics.versionIndexSize.Set(float64(len(s.vi)))
	}
}
