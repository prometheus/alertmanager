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

	"code.google.com/p/gorest"

	"github.com/prometheus/alert_manager/manager"
)

type Silence struct {
	CreatedBy        string
	CreatedAtSeconds int64
	EndsAtSeconds    int64
	Comment          string
	Filters          map[string]string
}

func (s AlertManagerService) AddSilence(sc manager.Silence) {
	// BUG: add server-side form validation.
	id := s.Silencer.AddSilence(&sc)

	rb := s.ResponseBuilder()
	rb.SetResponseCode(http.StatusCreated)
	rb.Location(fmt.Sprintf("/api/silences/%d", id))
}

func (s AlertManagerService) GetSilence(id int) string {
	rb := s.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)
	silence, err := s.Silencer.GetSilence(manager.SilenceId(id))
	if err != nil {
		log.Printf("Error getting silence: %s", err)
		rb.SetResponseCode(http.StatusNotFound)
		return err.Error()
	}

	resultBytes, err := json.Marshal(&silence)
	if err != nil {
		log.Printf("Error marshalling silence: %s", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}

func (s AlertManagerService) UpdateSilence(sc manager.Silence, id int) {
	// BUG: add server-side form validation.
	sc.Id = manager.SilenceId(id)
	if err := s.Silencer.UpdateSilence(&sc); err != nil {
		log.Printf("Error updating silence: %s", err)
		rb := s.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
	}
}

func (s AlertManagerService) DelSilence(id int) {
	if err := s.Silencer.DelSilence(manager.SilenceId(id)); err != nil {
		log.Printf("Error deleting silence: %s", err)
		rb := s.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
	}
}

func (s AlertManagerService) SilenceSummary() string {
	rb := s.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)
	silenceSummary := s.Silencer.SilenceSummary()

	resultBytes, err := json.Marshal(silenceSummary)
	if err != nil {
		log.Printf("Error marshalling silences: %s", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}
