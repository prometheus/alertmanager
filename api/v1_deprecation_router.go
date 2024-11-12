// Copyright 2023 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/prometheus/common/route"
)

// V1DeprecationRouter is the router to signal v1 users that the API v1 is now removed.
type V1DeprecationRouter struct {
	logger *slog.Logger
}

// NewV1DeprecationRouter returns a new V1DeprecationRouter.
func NewV1DeprecationRouter(l *slog.Logger) *V1DeprecationRouter {
	return &V1DeprecationRouter{
		logger: l,
	}
}

// Register registers all the API v1 routes with an endpoint that returns a JSON deprecation notice and a logs a warning.
func (dr *V1DeprecationRouter) Register(r *route.Router) {
	r.Get("/status", dr.deprecationHandler)
	r.Get("/receivers", dr.deprecationHandler)

	r.Get("/alerts", dr.deprecationHandler)
	r.Post("/alerts", dr.deprecationHandler)

	r.Get("/silences", dr.deprecationHandler)
	r.Post("/silences", dr.deprecationHandler)
	r.Get("/silence/:sid", dr.deprecationHandler)
	r.Del("/silence/:sid", dr.deprecationHandler)
}

func (dr *V1DeprecationRouter) deprecationHandler(w http.ResponseWriter, req *http.Request) {
	dr.logger.Warn("v1 API received a request on a removed endpoint", "path", req.URL.Path, "method", req.Method)

	resp := struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}{
		"deprecated",
		"The Alertmanager v1 API was deprecated in version 0.16.0 and is removed as of version 0.27.0 - please use the equivalent route in the v2 API",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(410)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		dr.logger.Error("failed to write response", "err", err)
	}
}
