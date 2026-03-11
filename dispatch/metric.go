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
	aggrGroups            prometheus.Gauge
	processingDuration    prometheus.Summary
	aggrGroupLimitReached prometheus.Counter
	alertsCollector       *alertStateCollector
}

// NewDispatcherMetrics returns a new registered DispatchMetrics.
func NewDispatcherMetrics(registerLimitMetrics bool, r prometheus.Registerer, ff featurecontrol.Flagger) *DispatcherMetrics {
	if r == nil {
		return nil
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
		labels: map[alert.AlertState][]string{
			alert.AlertStateActive:      {string(alert.AlertStateActive)},
			alert.AlertStateSuppressed:  {string(alert.AlertStateSuppressed)},
			alert.AlertStateUnprocessed: {string(alert.AlertStateUnprocessed)},
		},
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
		alertsCollector: collector,
	}

	return &m
}

// alertStateCollector implements prometheus.Collector to collect alert count
// metrics by state from the dispatcher's aggregation groups.
type alertStateCollector struct {
	desc           *prometheus.Desc
	enableGroupKey bool
	dispatcher     atomic.Pointer[Dispatcher]
	labels         map[alert.AlertState][]string
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
		for i := range d.routeGroupsSlice {
			d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
				ag := el.(*aggrGroup)
				active, suppressed, unprocessed := ag.countAlertsByState()
				groupKey := ag.GroupKey()
				c.emit(ch, float64(active), append(c.labels[alert.AlertStateActive], groupKey)...)
				c.emit(ch, float64(suppressed), append(c.labels[alert.AlertStateSuppressed], groupKey)...)
				c.emit(ch, float64(unprocessed), append(c.labels[alert.AlertStateUnprocessed], groupKey)...)
				return true
			})
		}
		return
	}

	// Deduplicate by fingerprint for backward compatibility.
	seen := map[model.Fingerprint]alert.AlertState{}
	for i := range d.routeGroupsSlice {
		d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
			ag := el.(*aggrGroup)
			for _, a := range ag.alerts.List() {
				fp := a.Fingerprint()
				if !a.Resolved() {
					if _, ok := seen[fp]; !ok {
						seen[fp] = ag.marker.Status(fp).State
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
	c.emit(ch, float64(active), c.labels[alert.AlertStateActive]...)
	c.emit(ch, float64(suppressed), c.labels[alert.AlertStateSuppressed]...)
	c.emit(ch, float64(unprocessed), c.labels[alert.AlertStateUnprocessed]...)
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
func (c *alertStateCollector) emit(ch chan<- prometheus.Metric, count float64, labels ...string) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, count, labels...)
}
