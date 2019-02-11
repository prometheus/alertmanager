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
	"net/http"
	"time"

	apiv1 "github.com/prometheus/alertmanager/api/v1"
	apiv2 "github.com/prometheus/alertmanager/api/v2"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/go-kit/kit/log"
)

// API represents all APIs of Alertmanager.
type API struct {
	v1 *apiv1.API
	v2 *apiv2.API
}

// New creates a new API object combining all API versions.
func New(
	alerts provider.Alerts,
	silences *silence.Silences,
	sf func(model.Fingerprint) types.AlertStatus,
	peer *cluster.Peer,
	l log.Logger,
) (*API, error) {
	v1 := apiv1.New(
		alerts,
		silences,
		sf,
		peer,
		log.With(l, "version", "v1"),
	)

	v2, err := apiv2.NewAPI(
		alerts,
		sf,
		silences,
		peer,
		log.With(l, "version", "v2"),
	)

	if err != nil {
		return nil, err
	}

	return &API{
		v1: v1,
		v2: v2,
	}, nil
}

// Register all APIs with the given router and return a mux.
func (api *API) Register(r *route.Router, routePrefix string) *http.ServeMux {
	api.v1.Register(r.WithPrefix("/api/v1"))

	mux := http.NewServeMux()
	mux.Handle("/", r)

	apiPrefix := ""
	if routePrefix != "/" {
		apiPrefix = routePrefix
	}
	mux.Handle(apiPrefix+"/api/v2/", http.StripPrefix(apiPrefix+"/api/v2", api.v2.Handler))

	return mux
}

// Update config and resolve timeout of each API.
func (api *API) Update(cfg *config.Config, resolveTimeout time.Duration, setAlertStatus func(model.LabelSet) error) error {
	if err := api.v1.Update(cfg, resolveTimeout); err != nil {
		return err
	}

	return api.v2.Update(cfg, resolveTimeout, setAlertStatus)
}
