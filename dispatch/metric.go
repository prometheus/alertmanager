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
		aggrGroups:               prometheus.NewGauge(prometheus.GaugeOpts{}),
		processingDuration:       prometheus.NewSummary(prometheus.SummaryOpts{}),
		aggrGroupLimitReached:    prometheus.NewCounter(prometheus.CounterOpts{}),
		aggrGroupCreationRetries: prometheus.NewCounter(prometheus.CounterOpts{}),
		aggrGroupCreationGivenUp: prometheus.NewCounter(prometheus.CounterOpts{}),
	}
}

// NewDispatcherMetrics returns a new registered DispatchMetrics.
func NewDispatcherMetrics(_ bool, r prometheus.Registerer, ff featurecontrol.Flagger) *DispatcherMetrics {
	if r == nil {
		return newNoopDispatcherMetrics()
	}
	if ff == nil {
		ff = featurecontrol.NoopFlags{}
	}

	stateLabels := []string{"state"}
	suppressedLabels := []string{"reason"}
	if ff.EnableGroupKeyInMetrics() {
		stateLabels = append(stateLabels, "group_key")
		suppressedLabels = append(suppressedLabels, "group_key")
	}

	collector := &alertStateCollector{
		desc: prometheus.NewDesc(
			"alertmanager_alerts",
			"How many alerts by state.",
			stateLabels, nil,
		),
		suppressedDesc: prometheus.NewDesc(
			"alertmanager_alerts_suppressed",
			"How many alerts are currently suppressed by reason. Alerts with multiple suppression reasons are counted once for each reason.",
			suppressedLabels, nil,
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
	suppressedDesc *prometheus.Desc
	dispatcher     atomic.Pointer[Dispatcher]
	enableGroupKey bool
}

func (c *alertStateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
	ch <- c.suppressedDesc
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
				c.emitCounts(ch, ag.countAlerts(), ag.GroupKey())
				return true
			})
		}
		return
	}

	// Deduplicate by fingerprint for backward compatibility.
	// The same alert can live in multiple aggregation groups with
	// different per-group marker states. Use highest-priority state:
	// suppressed > active > unprocessed.
	seen := map[model.Fingerprint]alertStatusSnapshot{}
	for i := range d.routeGroupsSlice {
		d.routeGroupsSlice[i].groups.Range(func(_, el any) bool {
			ag := el.(*aggrGroup)
			for _, a := range ag.alerts.List() {
				fp := a.Fingerprint()
				if !a.Resolved() {
					status := ag.marker.Status(fp)
					snapshot := seen[fp]
					if _, ok := seen[fp]; !ok || status.State.Compare(snapshot.state) > 0 {
						snapshot.state = status.State
					}
					snapshot.silenced = snapshot.silenced || len(status.SilencedBy) > 0
					snapshot.inhibited = snapshot.inhibited || len(status.InhibitedBy) > 0
					seen[fp] = snapshot
				}
			}
			return true
		})
	}
	var counts alertCounts
	for _, status := range seen {
		counts.addState(status.state)
		counts.addSuppressedReasons(status.silenced, status.inhibited)
	}
	c.emitCounts(ch, counts, "")
}

type alertCounts struct {
	active                 int
	suppressed             int
	unprocessed            int
	suppressedBySilence    int
	suppressedByInhibition int
}

type alertStatusSnapshot struct {
	state     alert.AlertState
	silenced  bool
	inhibited bool
}

// countAlerts counts non-resolved alerts in the group by marker state and suppression reason.
func (ag *aggrGroup) countAlerts() alertCounts {
	var counts alertCounts
	for _, a := range ag.alerts.List() {
		if a.Resolved() {
			continue
		}

		status := ag.marker.Status(a.Fingerprint())
		counts.addState(status.State)
		counts.addSuppressedReasons(len(status.SilencedBy) > 0, len(status.InhibitedBy) > 0)
	}
	return counts
}

func (c *alertCounts) addState(state alert.AlertState) {
	switch state {
	case alert.AlertStateActive:
		c.active++
	case alert.AlertStateSuppressed:
		c.suppressed++
	default:
		c.unprocessed++
	}
}

func (c *alertCounts) addSuppressedReasons(silenced, inhibited bool) {
	if silenced {
		c.suppressedBySilence++
	}
	if inhibited {
		c.suppressedByInhibition++
	}
}

func (c *alertStateCollector) emitCounts(ch chan<- prometheus.Metric, counts alertCounts, groupKey string) {
	if c.enableGroupKey {
		c.emit(ch, c.desc, float64(counts.active), string(alert.AlertStateActive), groupKey)
		c.emit(ch, c.desc, float64(counts.suppressed), string(alert.AlertStateSuppressed), groupKey)
		c.emit(ch, c.desc, float64(counts.unprocessed), string(alert.AlertStateUnprocessed), groupKey)
		c.emit(ch, c.suppressedDesc, float64(counts.suppressedBySilence), "silence", groupKey)
		c.emit(ch, c.suppressedDesc, float64(counts.suppressedByInhibition), "inhibition", groupKey)
		return
	}

	c.emit(ch, c.desc, float64(counts.active), string(alert.AlertStateActive))
	c.emit(ch, c.desc, float64(counts.suppressed), string(alert.AlertStateSuppressed))
	c.emit(ch, c.desc, float64(counts.unprocessed), string(alert.AlertStateUnprocessed))
	c.emit(ch, c.suppressedDesc, float64(counts.suppressedBySilence), "silence")
	c.emit(ch, c.suppressedDesc, float64(counts.suppressedByInhibition), "inhibition")
}

// emit sends a gauge metric with the given count and labels.
func (c *alertStateCollector) emit(ch chan<- prometheus.Metric, desc *prometheus.Desc, count float64, labelValues ...string) {
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, count, labelValues...)
}
