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

package api

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/manager"
)

// Return all currently firing alerts as JSON.
func (s AlertManagerService) getAlerts(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	aggs := s.Manager.GetAll(nil)
	respondJSON(w, aggs)
}

func (s AlertManagerService) addAlerts(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	alerts := []*manager.Alert{}
	if err := parseJSON(w, r, &alerts); err != nil {
		return
	}
	for i, a := range alerts {
		if a.Summary == "" || a.Description == "" {
			http.Error(w, fmt.Sprintf("Missing field in alert %d: %s", i, a), http.StatusBadRequest)
			return
		}
		if _, ok := a.Labels[model.AlertNameLabel]; !ok {
			http.Error(w, fmt.Sprintf("Missing alert name label in alert %d: %s", i, a), http.StatusBadRequest)
			return
		}
	}

	s.Manager.Receive(alerts)
}
