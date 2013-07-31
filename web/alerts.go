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

package web

import (
	"net/http"

	"github.com/prometheus/alert_manager/manager"
)

type AlertStatus struct {
	AlertAggregates []*manager.AggregationInstance
	SilenceForEvent func(*manager.Event) *manager.Silence
}

type AlertsHandler struct {
	Aggregator              *manager.Aggregator
	IsInhibitedInterrogator manager.IsInhibitedInterrogator
}

func (h *AlertsHandler) silenceForEvent(e *manager.Event) *manager.Silence {
	_, silence := h.IsInhibitedInterrogator.IsInhibited(e)
	return silence
}

func (h *AlertsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	alertStatus := &AlertStatus{
		AlertAggregates: h.Aggregator.AlertAggregates(),
		SilenceForEvent: h.silenceForEvent,
	}
	executeTemplate(w, "alerts", alertStatus)
}
