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
	"sync"
	"time"
)

type StatusHandler struct {
	mu sync.Mutex

	BuildInfo  map[string]string
	Config     string
	Flags      map[string]string
	Birth      time.Time
	PathPrefix string
}

func (h *StatusHandler) UpdateConfig(c string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.Config = c
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	executeTemplate(w, "status", h, h.PathPrefix)
}
