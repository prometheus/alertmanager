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

	"github.com/prometheus/alertmanager/manager"
)

type AlertStatus struct {
	AlertAggregates manager.AlertAggregates
	SilenceForAlert func(*manager.Alert) *manager.Silence
}

type AlertsHandler struct {
	Manager                manager.AlertManager
	IsSilencedInterrogator manager.IsSilencedInterrogator
}

func (h *AlertsHandler) silenceForAlert(a *manager.Alert) *manager.Silence {
	_, silence := h.IsSilencedInterrogator.IsSilenced(a.Labels)
	return silence
}

func (h *AlertsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	alertStatus := &AlertStatus{
		AlertAggregates: h.Manager.GetAll(nil),
		SilenceForAlert: h.silenceForAlert,
	}
	executeTemplate(w, "alerts", alertStatus)
}
