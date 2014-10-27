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

	//"github.com/golang/glog"
	"github.com/julienschmidt/httprouter"

	"github.com/prometheus/alertmanager/manager"
)

func (s AlertManagerService) addSilence(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	sc := manager.Silence{}
	if err := parseJSON(w, r, &sc); err != nil {
		return
	}
	// BUG: add server-side form validation.
	id := s.Silencer.AddSilence(&sc)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Location", fmt.Sprintf("/api/silences/%d", id))
}

func (s AlertManagerService) getSilence(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	silence, err := s.Silencer.GetSilence(manager.SilenceID(getID(p)))
	if err != nil {
		http.Error(w, fmt.Sprint("Error getting silence: ", err), http.StatusNotFound)
		return
	}

	respondJSON(w, &silence)
}

func (s AlertManagerService) updateSilence(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	sc := manager.Silence{}
	if err := parseJSON(w, r, &sc); err != nil {
		return
	}
	// BUG: add server-side form validation.
	sc.ID = manager.SilenceID(getID(p))
	if err := s.Silencer.UpdateSilence(&sc); err != nil {
		http.Error(w, fmt.Sprint("Error updating silence: ", err), http.StatusNotFound)
		return
	}
}

func (s AlertManagerService) deleteSilence(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if err := s.Silencer.DelSilence(manager.SilenceID(getID(p))); err != nil {
		http.Error(w, fmt.Sprint("Error deleting silence: ", err), http.StatusNotFound)
		return
	}
}

func (s AlertManagerService) silenceSummary(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	respondJSON(w, s.Silencer.SilenceSummary())
}
