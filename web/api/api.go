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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"

	"github.com/prometheus/alertmanager/manager"
)

type AlertManagerService struct {
	Manager    manager.AlertManager
	Silencer   *manager.Silencer
	PathPrefix string
}

func (s AlertManagerService) Handler() http.Handler {
	r := httprouter.New()
	r.GET(s.PathPrefix+"api/alerts", s.getAlerts)
	r.POST(s.PathPrefix+"api/alerts", s.addAlerts)
	r.GET(s.PathPrefix+"api/silences", s.silenceSummary)
	r.POST(s.PathPrefix+"api/silences", s.addSilence)
	r.GET(s.PathPrefix+"api/silences/:id", s.getSilence)
	r.POST(s.PathPrefix+"api/silences/:id", s.updateSilence)
	r.DELETE(s.PathPrefix+"api/silences/:id", s.deleteSilence)

	return r
}

func respondJSON(w http.ResponseWriter, v interface{}) {
	resultBytes, err := json.Marshal(v)
	if err != nil {
		http.Error(w, fmt.Sprint("Error marshalling JSON: ", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-type", "application/json")
	w.Write(resultBytes)
}

func getID(p httprouter.Params) int {
	n, _ := strconv.Atoi(p.ByName("id"))
	return n
}

func parseJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		http.Error(w, fmt.Sprint("failed to parse JSON: ", err.Error()), http.StatusBadRequest)
		return err
	}
	return nil
}
