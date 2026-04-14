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

package dispatch

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/featurecontrol"
)

// DispatcherMetrics represents metrics associated to a dispatcher.
type DispatcherMetrics struct {
	aggrGroups               prometheus.Gauge
	processingDuration       prometheus.Summary
	aggrGroupLimitReached    prometheus.Counter
	aggrGroupCreationRetries prometheus.Counter
	aggrGroupCreationGivenUp prometheus.Counter
	alertsCollector          *alertStateCollector
}

// newNoopDispatcherMetrics returns a DispatcherMetrics whose counters,
// gauges, and summaries silently discard observations. It is used when
// no prometheus.Registerer is provided.
func newNoopDispatcherMetrics() *DispatcherMetrics {
	return &DispatcherMetrics{
		aggrGroups:            prometheus.NewGauge(prometheus.GaugeOpts{}),
		processingDuration:    prometheus.NewSummary(prometheus.SummaryOpts{}),
		aggrGroupLimitReached: prometheus.NewCounter(prometheus.CounterOpts{}),
	}
}

// NewDispatcherMetrics returns a new registered DispatchMetrics.
func NewDispatcherMetrics(registerLimitMetrics bool, r prometheus.Registerer, ff featurecontrol.Flagger) *DispatcherMetrics {
	if r == nil {
		return newNoopDispatcherMetrics()
	}
	if ff == nil {
		ff = featurecontrol.NoopFlags{}
	}

	labels := []string{"state"}
	if ff.EnableGroupKeyInMetrics() {
		labels = append(labels, "group_key")
	}

	collector := &alertStateCollector{
		desc: prometheus.NewDesc(
			"alertmanager_alerts",
			"How many alerts by state.",
			labels, nil,
		),
		enableGroupKey: ff.EnableGroupKeyInMetrics(),
	}
	r.MustRegister(collector)

	m := DispatcherMetrics{
		aggrGroups: promauto.With(r).NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_dispatcher_aggregation_groups",
				Help: "Number of active aggregation groups",
			},
		),
		processingDuration: promauto.With(r).NewSummary(
			prometheus.SummaryOpts{
				Name: "alertmanager_dispatcher_alert_processing_duration_seconds",
				Help: "Summary of latencies for the processing of alerts.",
			},
		),
		aggrGroupLimitReached: promauto.With(r).NewCounter(
			prometheus.CounterOpts{
				Name: "alertmanager_dispatcher_aggregation_group_limit_reached_total",
				Help: "Number of times when dispatcher failed to create new aggregation group due to limit.",
			},
		),
		aggrGroupCreationRetries: promauto.With(r).NewCounter(
			prometheus.CounterOpts{
				Name: "alertmanager_dispatcher_aggregation_group_creation_retries_total",
				Help: "Number of CAS retries while creating aggregation groups under contention.",
			},
		),
		aggrGroupCreationGivenUp: promauto.With(r).NewCounter(
			prometheus.CounterOpts{
				Name: "alertmanager_dispatcher_aggregation_group_creation_given_up_total",
				Help: "Number of alerts dropped because aggregation group creation exceeded the retry limit.",
			},
		),
		alertsCollector: collector,
	}

	return &m
}

// alertStateCollector implements prometheus.Collector to collect alert count
// metrics by state from the dispatcher's aggregation groups.
type alertStateCollector struct {
	desc           *prometheus.Desc
	dispatcher     atomic.Pointer[Dispatcher]
	enableGroupKey bool
}

func (c *alertStateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *alertStateCollector) Collect(ch chan<- prometheus.Metric) {
	d := c.dispatcher.Load()
	if d == nil {
		return
	}
	if d.state.Load() != DispatcherStateRunning {
		return
	}

	if c.enableGroupKey {
		labelValues := make([]string, 2)
		for i := range d.routeGroupsSlice {
			d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
				ag := el.(*aggrGroup)
				active, suppressed, unprocessed := ag.countAlertsByState()
				labelValues[1] = ag.GroupKey()
				labelValues[0] = string(alert.AlertStateActive)
				c.emit(ch, float64(active), labelValues...)
				labelValues[0] = string(alert.AlertStateSuppressed)
				c.emit(ch, float64(suppressed), labelValues...)
				labelValues[0] = string(alert.AlertStateUnprocessed)
				c.emit(ch, float64(unprocessed), labelValues...)
				return true
			})
		}
		return
	}

	// Deduplicate by fingerprint for backward compatibility.
	// The same alert can live in multiple aggregation groups with
	// different per-group marker states. Use highest-priority state:
	// suppressed > active > unprocessed.
	seen := map[model.Fingerprint]alert.AlertState{}
	for i := range d.routeGroupsSlice {
		d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
			ag := el.(*aggrGroup)
			for _, a := range ag.alerts.List() {
				fp := a.Fingerprint()
				if !a.Resolved() {
					state := ag.marker.Status(fp).State
					if prev, ok := seen[fp]; !ok || state.Compare(prev) > 0 {
						seen[fp] = state
					}
				}
			}
			return true
		})
	}
	var active, suppressed, unprocessed int
	for _, state := range seen {
		switch state {
		case alert.AlertStateActive:
			active++
		case alert.AlertStateSuppressed:
			suppressed++
		default:
			unprocessed++
		}
	}
	c.emit(ch, float64(active), string(alert.AlertStateActive))
	c.emit(ch, float64(suppressed), string(alert.AlertStateSuppressed))
	c.emit(ch, float64(unprocessed), string(alert.AlertStateUnprocessed))
}

// countAlertsByState counts non-resolved alerts in the group by their marker state.
func (ag *aggrGroup) countAlertsByState() (active, suppressed, unprocessed int) {
	for _, a := range ag.alerts.List() {
		if a.Resolved() {
			continue
		}
		switch ag.marker.Status(a.Fingerprint()).State {
		case alert.AlertStateActive:
			active++
		case alert.AlertStateSuppressed:
			suppressed++
		default:
			unprocessed++
		}
	}
	return active, suppressed, unprocessed
}

// emit sends a gauge metric with the given count and labels.
func (c *alertStateCollector) emit(ch chan<- prometheus.Metric, count float64, labelValues ...string) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, count, labelValues...)
}
