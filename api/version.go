// Copyright 2019 Prometheus Team
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
	"log"
	"net/http"

	"github.com/prometheus/common/version"
)

type versionResponse struct {
	Version string `json:"version"`
}

// VersionHandler returns Alertmanager version
func VersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	b, err := json.Marshal(&versionResponse{
		Version: version.Version,
	})
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		return
	}

	_, err = w.Write(b)
	if err != nil {
		log.Printf("Failed to write data to connection: %s", err)
	}
}
