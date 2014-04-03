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
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"

	"github.com/prometheus/alertmanager/manager"
)

type Silence struct {
	CreatedBy        string
	CreatedAtSeconds int64
	EndsAtSeconds    int64
	Comment          string
	Filters          map[string]string
}

func (s AlertManagerService) Silences(w http.ResponseWriter, r *http.Request) {
	log.Printf("path: %s, root: %s", r.URL.Path, silencesPath)
	path := strings.TrimLeft(r.URL.Path[len(silencesPath):], "/")
	if path == "" {
		switch {
		case r.Method == "POST":
			decoder := json.NewDecoder(r.Body)
			sc := manager.Silence{}
			if err := decoder.Decode(&sc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.AddSilence(w, sc)
		case r.Method == "GET":
			s.SilenceSummary(w)
		default:
			http.Error(w, "", 404)
		}
	} else {
		id, err := strconv.Atoi(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid id '%s': %s", path, err), 500)
			return
		}
		switch {
		case r.Method == "GET":
			s.GetSilence(w, id)

		case r.Method == "POST":
			decoder := json.NewDecoder(r.Body)
			sc := manager.Silence{}
			if err := decoder.Decode(&sc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.UpdateSilence(w, sc, id)
		case r.Method == "DELETE":
			s.DelSilence(w, id)

		default:
			http.Error(w, "", http.StatusNotFound)
		}
	}
}

func (s AlertManagerService) AddSilence(w http.ResponseWriter, sc manager.Silence) {
	// BUG: add server-side form validation.
	id := s.Silencer.AddSilence(&sc)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Location", fmt.Sprintf("/api/silences/%d", id))
}

func (s AlertManagerService) GetSilence(w http.ResponseWriter, id int) {
	w.Header().Set("Content-Type", "application/json")
	silence, err := s.Silencer.GetSilence(manager.SilenceId(id))
	if err != nil {
		glog.Error("Error getting silence: ", err)
		http.Error(w, "Error getting silence: "+err.Error(), http.StatusNotFound)
	}

	resultBytes, err := json.Marshal(&silence)
	if err != nil {
		glog.Error("Error marshalling silence: ", err)
		http.Error(w, "Error marshalling silence: "+err.Error(), http.StatusInternalServerError)
	}
	fmt.Fprintf(w, string(resultBytes))
}

func (s AlertManagerService) UpdateSilence(w http.ResponseWriter, sc manager.Silence, id int) {
	// BUG: add server-side form validation.
	sc.Id = manager.SilenceId(id)
	if err := s.Silencer.UpdateSilence(&sc); err != nil {
		glog.Error("Error updating silence: ", err)
		http.Error(w, "Error updating silence: "+err.Error(), http.StatusNotFound)
	}
}

func (s AlertManagerService) DelSilence(w http.ResponseWriter, id int) {
	if err := s.Silencer.DelSilence(manager.SilenceId(id)); err != nil {
		glog.Error("Error deleting silence: ", err)
		http.Error(w, "Error deleting silence: "+err.Error(), http.StatusNotFound)
	}
}

func (s AlertManagerService) SilenceSummary(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	silenceSummary := s.Silencer.SilenceSummary()

	resultBytes, err := json.Marshal(silenceSummary)
	if err != nil {
		glog.Error("Error marshalling silences: ", err)
		http.Error(w, "Error marshalling silences: "+err.Error(), http.StatusInternalServerError)
	}
	fmt.Fprintf(w, string(resultBytes))
}
