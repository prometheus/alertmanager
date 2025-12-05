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

package inhibit

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// InhibitorMetrics represents metrics associated to an inhibitor.
type InhibitorMetrics struct {
	// Inhibitor metrics
	sourceAlertsCacheItems prometheus.Gauge
	sourceAlertsIndexItems prometheus.Gauge
	mutesDuration          *prometheus.SummaryVec
	mutesDurationMuted     prometheus.Observer
	mutesDurationNotMuted  prometheus.Observer

	// Rule metrics
	ruleSourceAlertsCacheItems *prometheus.GaugeVec
	ruleSourceAlertsIndexItems *prometheus.GaugeVec
	ruleMatchesDuration        *prometheus.SummaryVec
	ruleMutesDuration          *prometheus.SummaryVec
}

// NewInhibitorMetrics returns a new InhibitorMetrics.
func NewInhibitorMetrics(reg prometheus.Registerer) *InhibitorMetrics {
	if reg == nil {
		return nil
	}
	metrics := &InhibitorMetrics{
		sourceAlertsCacheItems: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_inhibitor_source_alerts_cache_items",
				Help: "Number of source alerts cached in inhibition rules.",
			},
		),
		sourceAlertsIndexItems: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_inhibitor_source_alerts_index_items",
				Help: "Number of source alerts indexed in inhibition rules.",
			},
		),
		mutesDuration: promauto.With(reg).NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "alertmanager_inhibitor_mutes_duration_seconds",
				Help: "Summary of latencies for the muting of alerts by inhibition rules.",
			},
			[]string{"muted"},
		),

		ruleSourceAlertsCacheItems: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "alertmanager_inhibit_rule_source_alerts_cache_items",
				Help: "Number of source alerts cached in inhibition rules.",
			},
			[]string{"rule"},
		),
		ruleSourceAlertsIndexItems: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "alertmanager_inhibit_rule_source_alerts_index_items",
				Help: "Number of source alerts indexed in inhibition rules.",
			},
			[]string{"rule"},
		),
		ruleMatchesDuration: promauto.With(reg).NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "alertmanager_inhibit_rule_matches_duration_seconds",
				Help: "Summary of latencies for the matching of alerts by inhibition rules.",
			},
			[]string{"rule", "matched"},
		),
		ruleMutesDuration: promauto.With(reg).NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "alertmanager_inhibit_rule_mutes_duration_seconds",
				Help: "Summary of latencies for the muting of alerts by inhibition rules.",
			},
			[]string{"rule", "muted"},
		),
	}

	metrics.mutesDurationMuted = metrics.mutesDuration.With(prometheus.Labels{"muted": "true"})
	metrics.mutesDurationNotMuted = metrics.mutesDuration.With(prometheus.Labels{"muted": "false"})

	metrics.sourceAlertsCacheItems.Set(0)
	metrics.sourceAlertsIndexItems.Set(0)

	return metrics
}

type RuleMetrics struct {
	ruleName                  string
	matchesDurationMatched    prometheus.Observer
	matchesDurationNotMatched prometheus.Observer

	mutesDurationMuted    prometheus.Observer
	mutesDurationNotMuted prometheus.Observer

	sourceAlertsCacheItems *prometheus.GaugeVec
	sourceAlertsIndexItems *prometheus.GaugeVec
}

func NewRuleMetrics(name string, metrics *InhibitorMetrics) *RuleMetrics {
	rm := &RuleMetrics{
		ruleName:                  name,
		matchesDurationMatched:    metrics.ruleMatchesDuration.With(prometheus.Labels{"rule": name, "matched": "true"}),
		matchesDurationNotMatched: metrics.ruleMatchesDuration.With(prometheus.Labels{"rule": name, "matched": "false"}),
		mutesDurationMuted:        metrics.ruleMutesDuration.With(prometheus.Labels{"rule": name, "muted": "true"}),
		mutesDurationNotMuted:     metrics.ruleMutesDuration.With(prometheus.Labels{"rule": name, "muted": "false"}),
		sourceAlertsCacheItems:    metrics.ruleSourceAlertsCacheItems,
		sourceAlertsIndexItems:    metrics.ruleSourceAlertsIndexItems,
	}

	rm.sourceAlertsCacheItems.With(prometheus.Labels{"rule": rm.ruleName}).Set(0)
	rm.sourceAlertsIndexItems.With(prometheus.Labels{"rule": rm.ruleName}).Set(0)

	return rm
}
