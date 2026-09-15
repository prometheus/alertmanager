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
package notify

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/alertmanager/featurecontrol"
)

type Metrics struct {
	numNotifications                   *prometheus.CounterVec
	numTotalFailedNotifications        *prometheus.CounterVec
	numNotificationRequestsTotal       *prometheus.CounterVec
	numNotificationRequestsFailedTotal *prometheus.CounterVec
	numNotificationSuppressedTotal     *prometheus.CounterVec
	notificationLatencySeconds         *prometheus.HistogramVec

	ff featurecontrol.Flagger
}

func NewMetrics(r prometheus.Registerer, ff featurecontrol.Flagger) *Metrics {
	labels := []string{"integration"}

	if ff.EnableReceiverNamesInMetrics() {
		labels = append(labels, "receiver_name")
	}

	m := &Metrics{
		numNotifications: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_total",
			Help:      "The total number of attempted notifications.",
		}, labels),
		numTotalFailedNotifications: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_failed_total",
			Help:      "The total number of failed notifications.",
		}, append(labels, "reason")),
		numNotificationRequestsTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notification_requests_total",
			Help:      "The total number of attempted notification requests.",
		}, labels),
		numNotificationRequestsFailedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notification_requests_failed_total",
			Help:      "The total number of failed notification requests.",
		}, labels),
		numNotificationSuppressedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_suppressed_total",
			Help:      "The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.",
		}, []string{"reason"}),
		notificationLatencySeconds: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "alertmanager",
			Name:                            "notification_latency_seconds",
			Help:                            "The latency of notifications in seconds.",
			Buckets:                         []float64{1, 5, 10, 15, 20},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, labels),
		ff: ff,
	}

	return m
}

func (m *Metrics) InitializeFor(receiver map[string][]Integration) {
	if m.ff.EnableReceiverNamesInMetrics() {

		// Reset the vectors to take into account receiver names changing after hot reloads.
		m.numNotifications.Reset()
		m.numNotificationRequestsTotal.Reset()
		m.numNotificationRequestsFailedTotal.Reset()
		m.notificationLatencySeconds.Reset()
		m.numTotalFailedNotifications.Reset()

		for name, integrations := range receiver {
			for _, integration := range integrations {

				m.numNotifications.WithLabelValues(integration.Name(), name)
				m.numNotificationRequestsTotal.WithLabelValues(integration.Name(), name)
				m.numNotificationRequestsFailedTotal.WithLabelValues(integration.Name(), name)
				m.notificationLatencySeconds.WithLabelValues(integration.Name(), name)

				for _, reason := range possibleFailureReasonCategory {
					m.numTotalFailedNotifications.WithLabelValues(integration.Name(), name, reason)
				}
			}
		}

		return
	}

	// When the feature flag is not enabled, we just carry on registering _all_ the integrations.
	for _, integration := range []string{
		"email",
		"pagerduty",
		"wechat",
		"pushover",
		"slack",
		"opsgenie",
		"webhook",
		"victorops",
		"sns",
		"telegram",
		"discord",
		"webex",
		"msteams",
		"msteamsv2",
		"incidentio",
		"jira",
		"rocketchat",
		"mattermost",
	} {
		m.numNotifications.WithLabelValues(integration)
		m.numNotificationRequestsTotal.WithLabelValues(integration)
		m.numNotificationRequestsFailedTotal.WithLabelValues(integration)
		m.notificationLatencySeconds.WithLabelValues(integration)

		for _, reason := range possibleFailureReasonCategory {
			m.numTotalFailedNotifications.WithLabelValues(integration, reason)
		}
	}
}
