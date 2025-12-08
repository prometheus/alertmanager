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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

// getMetricValue retrieves a specific metric value from the registry.
func getMetricValue(t *testing.T, reg *prometheus.Registry, metricName string, labels map[string]string) (float64, uint64, bool) {
	t.Helper()
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range metricFamilies {
		if mf.GetName() != metricName {
			continue
		}
		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric, labels) {
				if mf.GetType() == io_prometheus_client.MetricType_GAUGE {
					return metric.GetGauge().GetValue(), 0, true
				}
				if mf.GetType() == io_prometheus_client.MetricType_SUMMARY {
					return 0, metric.GetSummary().GetSampleCount(), true
				}
			}
		}
	}
	return 0, 0, false
}

func labelsMatch(metric *io_prometheus_client.Metric, wantLabels map[string]string) bool {
	for wantKey, wantVal := range wantLabels {
		found := false
		for _, labelPair := range metric.GetLabel() {
			if labelPair.GetName() == wantKey && labelPair.GetValue() == wantVal {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestInhibitorMetrics_RuleMatchesDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "test-rule",
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Test case 1: Target matches (should record matched="true")
	targetAlert := model.LabelSet{
		"severity": "warning",
		"instance": "server1",
	}
	inhibitor.Mutes(targetAlert)

	_, count, found := getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "test-rule", "matched": "true"})
	require.True(t, found, "Should find matched=true metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for matched=true")

	// Test case 2: Target doesn't match (should record matched="false")
	nonMatchingAlert := model.LabelSet{
		"severity": "info",
		"instance": "server2",
	}
	inhibitor.Mutes(nonMatchingAlert)

	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "test-rule", "matched": "false"})
	require.True(t, found, "Should find matched=false metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for matched=false")
}

func TestInhibitorMetrics_RuleMutesDuration_Muted(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "test-rule",
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Add a source alert that will inhibit
	sourceAlert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"severity": "critical",
				"instance": "server1",
			},
			StartsAt: time.Now().Add(-time.Minute),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}
	inhibitor.rules[0].Sources[0].scache.Set(sourceAlert)
	inhibitor.rules[0].Sources[0].updateIndex(sourceAlert)

	// Test that target alert is muted
	targetAlert := model.LabelSet{
		"severity": "warning",
		"instance": "server1",
	}
	muted := inhibitor.Mutes(targetAlert)
	require.True(t, muted, "Alert should be muted")

	// Verify per-rule muted="true" metric was recorded
	_, count, found := getMetricValue(t, reg, "alertmanager_inhibit_rule_mutes_duration_seconds",
		map[string]string{"rule": "test-rule", "muted": "true"})
	require.True(t, found, "Should find per-rule muted=true metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for per-rule muted=true")

	// Verify global muted="true" metric was recorded
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibitor_mutes_duration_seconds",
		map[string]string{"muted": "true"})
	require.True(t, found, "Should find global muted=true metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for global muted=true")
}

func TestInhibitorMetrics_RuleMutesDuration_NotMuted(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "test-rule",
			Sources: []config.InhibitRuleSource{
				{
					SrcMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "critical"}},
				},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Add a source alert with different instance
	sourceAlert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"severity": "critical",
				"instance": "server1",
			},
			StartsAt: time.Now().Add(-time.Minute),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}
	inhibitor.rules[0].Sources[0].scache.Set(sourceAlert)

	// Test that target alert with different instance is NOT muted
	targetAlert := model.LabelSet{
		"severity": "warning",
		"instance": "server2",
	}
	muted := inhibitor.Mutes(targetAlert)
	require.False(t, muted, "Alert should not be muted")

	// Verify per-rule muted="false" metric was recorded
	_, count, found := getMetricValue(t, reg, "alertmanager_inhibit_rule_mutes_duration_seconds",
		map[string]string{"rule": "test-rule", "muted": "false"})
	require.True(t, found, "Should find per-rule muted=false metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for per-rule muted=false")

	// Verify global muted="false" metric was recorded
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibitor_mutes_duration_seconds",
		map[string]string{"muted": "false"})
	require.True(t, found, "Should find global muted=false metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for global muted=false")
}

func TestInhibitorMetrics_NoRuleMatches(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "test-rule",
			Sources: []config.InhibitRuleSource{
				{
					SrcMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "critical"}},
				},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Test with alert that doesn't match any rule's target
	nonMatchingAlert := model.LabelSet{
		"severity": "info",
		"instance": "server1",
	}
	muted := inhibitor.Mutes(nonMatchingAlert)
	require.False(t, muted, "Alert should not be muted")

	// Verify that global muted="false" metric was recorded
	_, count, found := getMetricValue(t, reg, "alertmanager_inhibitor_mutes_duration_seconds",
		map[string]string{"muted": "false"})
	require.True(t, found, "Should find global muted=false metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for global muted=false")

	// Verify per-rule matched="false" was recorded
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "test-rule", "matched": "false"})
	require.True(t, found, "Should find rule matched=false metric")
	require.Equal(t, uint64(1), count, "Should have 1 sample for rule matched=false")
}

func TestInhibitorMetrics_MultipleRules(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "rule-1",
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
		{
			Name: "rule-2",
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "team", Value: "sre"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "team", Value: "dev"},
			},
			Equal: []string{"service"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Add source alert for rule-1
	sourceAlert1 := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"severity": "critical",
				"instance": "server1",
			},
			StartsAt: time.Now().Add(-time.Minute),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}
	inhibitor.rules[0].Sources[0].scache.Set(sourceAlert1)
	inhibitor.rules[0].Sources[0].updateIndex(sourceAlert1)

	// Test alert that matches rule-1
	targetAlert1 := model.LabelSet{
		"severity": "warning",
		"instance": "server1",
	}
	muted1 := inhibitor.Mutes(targetAlert1)
	require.True(t, muted1, "Alert should be muted by rule-1")

	// Verify metrics for rule-1
	_, count, found := getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "rule-1", "matched": "true"})
	require.True(t, found, "Should find rule-1 matched=true metric")
	require.Equal(t, 1, int(count))

	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_mutes_duration_seconds",
		map[string]string{"rule": "rule-1", "muted": "true"})
	require.True(t, found, "Should find rule-1 muted=true metric")
	require.Equal(t, 1, int(count))

	// Verify global muted="true" metric
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibitor_mutes_duration_seconds",
		map[string]string{"muted": "true"})
	require.True(t, found, "Should find global muted=true metric")
	require.Equal(t, 1, int(count))

	// Test alert that matches rule-2 target but has no source
	targetAlert2 := model.LabelSet{
		"team":    "dev",
		"service": "api",
	}
	muted2 := inhibitor.Mutes(targetAlert2)
	require.False(t, muted2, "Alert should not be muted")

	// Verify metrics for rule-2 (both rules process this alert since rule-1 doesn't match target)
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "rule-1", "matched": "false"})
	require.True(t, found, "Should find rule-1 matched=false metric")
	require.Equal(t, 1, int(count))

	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_matches_duration_seconds",
		map[string]string{"rule": "rule-2", "matched": "true"})
	require.True(t, found, "Should find rule-2 matched=true metric")
	require.Equal(t, 1, int(count))

	_, count, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_mutes_duration_seconds",
		map[string]string{"rule": "rule-2", "muted": "false"})
	require.True(t, found, "Should find rule-2 muted=false metric")
	require.Equal(t, 1, int(count))

	// Verify global muted="false" metric
	_, count, found = getMetricValue(t, reg, "alertmanager_inhibitor_mutes_duration_seconds",
		map[string]string{"muted": "false"})
	require.True(t, found, "Should find global muted=false metric")
	require.Equal(t, 1, int(count), "Should have 1 samples")
}

func TestInhibitorMetrics_CacheAndIndexItems(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	rules := []config.InhibitRule{
		{
			Name: "named-rule",
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
		{
			SourceMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "critical"},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"cluster"},
		},
	}

	marker := types.NewMarker(reg)
	provider, err := mem.NewAlerts(t.Context(), marker, 15*time.Minute, nil, nopLogger, reg)
	require.NoError(t, err)
	inhibitor := NewInhibitor(provider, rules, marker, nopLogger, metrics)
	go inhibitor.Run()

	// Add multiple source alerts
	for i := 1; i <= 3; i++ {
		sourceAlert := &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"severity": "critical",
					"instance": model.LabelValue("server" + string(rune('0'+i))),
					"cluster":  model.LabelValue("cluster" + string(rune('0'+i))),
				},
				StartsAt: time.Now().Add(-time.Minute),
				EndsAt:   time.Now().Add(time.Hour),
			},
		}
		require.NoError(t, provider.Put(sourceAlert))
	}

	// Wait for the inhibitor to process alerts and update metrics
	// The Run() goroutine processes alerts asynchronously
	require.Eventually(t, func() bool {
		value, _, found := getMetricValue(t, reg, "alertmanager_inhibitor_source_alerts_cache_items",
			map[string]string{})
		return found && value == 6
	}, 2*time.Second, 50*time.Millisecond, "Cache items metric should reach 6")

	// Stop the inhibitor
	inhibitor.Stop()

	// Global metrics (no labels) show the sum across all rules
	value, _, found := getMetricValue(t, reg, "alertmanager_inhibitor_source_alerts_cache_items",
		map[string]string{})
	require.True(t, found, "Should find global cache items metric")
	require.Equal(t, float64(6), value, "Global cache should contain 6 alerts total")

	value, _, found = getMetricValue(t, reg, "alertmanager_inhibitor_source_alerts_index_items",
		map[string]string{})
	require.True(t, found, "Should find global index items metric")
	require.Equal(t, float64(6), value, "Global index should contain 6 entries total")

	// Per-rule metrics show individual rule values
	value, _, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_source_alerts_cache_items",
		map[string]string{"rule": "named-rule"})
	require.True(t, found, "Should find per-rule cache items metric")
	require.Equal(t, float64(3), value, "Named rule cache should contain 3 alerts")

	value, _, found = getMetricValue(t, reg, "alertmanager_inhibit_rule_source_alerts_index_items",
		map[string]string{"rule": "named-rule"})
	require.True(t, found, "Should find per-rule index items metric")
	require.Equal(t, float64(3), value, "Named rule index should contain 3 entries")
}

func TestInhibitorMetrics_Registration(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewInhibitorMetrics(reg)

	require.NotNil(t, metrics, "Metrics should be created")

	// Create a rule and use the metrics so they appear in Gather() output
	rules := []config.InhibitRule{
		{
			Name: "test-rule",
			Sources: []config.InhibitRuleSource{
				{
					SrcMatchers: config.Matchers{&labels.Matcher{Type: labels.MatchEqual, Name: "severity", Value: "critical"}},
				},
			},
			TargetMatchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: "severity", Value: "warning"},
			},
			Equal: []string{"instance"},
		},
	}

	marker := types.NewMarker(reg)
	inhibitor := NewInhibitor(nil, rules, marker, nopLogger, metrics)

	// Use the metrics to ensure they show up in Gather()
	testAlert := model.LabelSet{
		"severity": "warning",
		"instance": "server1",
	}
	inhibitor.Mutes(testAlert)

	// Verify all metrics are registered and have data
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	registeredMetrics := map[string]bool{
		"alertmanager_inhibitor_source_alerts_cache_items":    false,
		"alertmanager_inhibitor_source_alerts_index_items":    false,
		"alertmanager_inhibitor_mutes_duration_seconds":       false,
		"alertmanager_inhibit_rule_source_alerts_cache_items": false,
		"alertmanager_inhibit_rule_source_alerts_index_items": false,
		"alertmanager_inhibit_rule_matches_duration_seconds":  false,
		"alertmanager_inhibit_rule_mutes_duration_seconds":    false,
	}

	for _, mf := range metricFamilies {
		if _, exists := registeredMetrics[mf.GetName()]; exists {
			registeredMetrics[mf.GetName()] = true
		}
	}

	for metricName, registered := range registeredMetrics {
		require.True(t, registered, "Metric %s should be registered", metricName)
	}
}
