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
	"time"

	"code.google.com/p/gorest"

	"github.com/prometheus/alert_manager/manager"
)

type Silence struct {
	CreatedBy string
	CreatedAt int64
	EndsAt    int64
	Comment   string
	Filters   map[string]string
}

func translateSilenceFromApi(sc *Silence) *manager.Suppression {
	filters := make(manager.Filters, 0, len(sc.Filters))
	for label, value := range sc.Filters {
		filters = append(filters, manager.NewFilter(label, value))
	}

	if sc.EndsAt == 0 {
		sc.EndsAt = time.Now().Add(time.Hour).Unix()
	}

	return &manager.Suppression{
		CreatedBy: sc.CreatedBy,
		CreatedAt: time.Now(),
		EndsAt:    time.Unix(sc.EndsAt, 0),
		Comment:   sc.Comment,
		Filters:   filters,
	}
}

func translateSilenceToApi(sc *manager.Suppression) *Silence {
	filters := map[string]string{}
	for _, f := range sc.Filters {
		name := f.Name.String()[1 : len(f.Name.String())-1]
		value := f.Value.String()[1 : len(f.Value.String())-1]
		filters[name] = value
	}

	return &Silence{
		CreatedBy: sc.CreatedBy,
		CreatedAt: sc.CreatedAt.Unix(),
		EndsAt:    sc.EndsAt.Unix(),
		Comment:   sc.Comment,
		Filters:   filters,
	}
}

func (s AlertManagerService) AddSilence(sc Silence) {
	// BUG: add server-side form validation.
	sup := translateSilenceFromApi(&sc)
	id := s.Suppressor.AddSuppression(sup)

	rb := s.ResponseBuilder()
	rb.SetResponseCode(http.StatusCreated)
	rb.Location(fmt.Sprintf("/api/silences/%d", id))
}

func (s AlertManagerService) GetSilence(id int) string {
	rb := s.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)
	silence, err := s.Suppressor.GetSuppression(manager.SuppressionId(id))
	if err != nil {
		log.Printf("Error getting silence: %s", err)
		rb.SetResponseCode(http.StatusNotFound)
		return err.Error()
	}

	resultBytes, err := json.Marshal(translateSilenceToApi(silence))
	if err != nil {
		log.Printf("Error marshalling silences: %s", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}

func (s AlertManagerService) UpdateSilence(sc Silence, id int) {
	// BUG: add server-side form validation.
	sup := translateSilenceFromApi(&sc)
	sup.Id = manager.SuppressionId(id)
	if err := s.Suppressor.UpdateSuppression(sup); err != nil {
		log.Printf("Error updating silence: %s", err)
		rb := s.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
	}
}

func (s AlertManagerService) DelSilence(id int) {
	if err := s.Suppressor.DelSuppression(manager.SuppressionId(id)); err != nil {
		log.Printf("Error deleting silence: %s", err)
		rb := s.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
	}
}

func (s AlertManagerService) SilenceSummary() string {
	rb := s.ResponseBuilder()
	rb.SetContentType(gorest.Application_Json)
	silenceSummary := s.Suppressor.SuppressionSummary()

	silences := []*Silence{}
	for _, s := range silenceSummary {
		silences = append(silences, translateSilenceToApi(s))
	}

	resultBytes, err := json.Marshal(silences)
	if err != nil {
		log.Printf("Error marshalling silences: %s", err)
		rb.SetResponseCode(http.StatusInternalServerError)
		return err.Error()
	}
	return string(resultBytes)
}
