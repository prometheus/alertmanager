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
	"fmt"
	"net/http"
	"runtime"
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

// Register all APIs. It registers APIv1 with the provided router directly. As
// APIv2 works on the http.Handler level, this method also creates a new
// http.ServeMux and then uses it to register both the provided router (to
// handle "/") and APIv2 (to handle "<routePrefix>/api/v2"). The method returns
// the newly created http.ServeMux.
//
// If the provided value for timeout is positive, it is enforced for all HTTP
// requests. (Negative or zero results in no timeout at all.)
//
// If the provided value for concurrency is positive, it limits the number of
// concurrently processed GET requests. Otherwise, the number of concurrently
// processed GET requests is limited to GOMAXPROCS or 8, whatever is
// larger. Status code 503 is served for GET requests that would exceed the
// concurrency limit.
func (api *API) Register(
	r *route.Router, routePrefix string,
	timeout time.Duration, concurrency int,
) *http.ServeMux {
	limiter := makeLimiter(timeout, concurrency)

	api.v1.Register(r.WithPrefix("/api/v1"))

	mux := http.NewServeMux()
	mux.Handle("/", limiter(r))

	apiPrefix := ""
	if routePrefix != "/" {
		apiPrefix = routePrefix
	}
	mux.Handle(
		apiPrefix+"/api/v2/",
		limiter(http.StripPrefix(apiPrefix+"/api/v2", api.v2.Handler)),
	)

	return mux
}

// Update config and resolve timeout of each API.
func (api *API) Update(cfg *config.Config, resolveTimeout time.Duration) error {
	if err := api.v1.Update(cfg, resolveTimeout); err != nil {
		return err
	}

	return api.v2.Update(cfg, resolveTimeout)
}

// makeLimiter returns an HTTP middleware that sets a timeout for HTTP requests
// and also limits the number of concurrently processed GET requests to the
// given number.
//
// If timeout is < 1, no timeout is enforced.
//
// If concurrency is < 1, GOMAXPROCS is used as the concurrency limit but at least 8.
//
// The returned middleware serves http.StatusServiceUnavailable (503) for requests that
// would exceed the number.
func makeLimiter(timeout time.Duration, concurrency int) func(http.Handler) http.Handler {
	if concurrency < 1 {
		concurrency = runtime.GOMAXPROCS(0)
		if concurrency < 8 {
			concurrency = 8
		}
	}
	inFlightSem := make(chan struct{}, concurrency)

	return func(h http.Handler) http.Handler {
		concLimiter := http.HandlerFunc(func(rsp http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodGet { // Only limit concurrency of GETs.
				select {
				case inFlightSem <- struct{}{}: // All good, carry on.
					defer func() { <-inFlightSem }()
				default:
					http.Error(rsp, fmt.Sprintf(
						"Limit of concurrent GET requests reached (%d), try again later.\n", concurrency,
					), http.StatusServiceUnavailable)
					return
				}
			}
			h.ServeHTTP(rsp, req)
		})
		if timeout <= 0 {
			return concLimiter
		}
		return http.TimeoutHandler(concLimiter, timeout, fmt.Sprintf(
			"Exceeded configured timeout of %v.\n", timeout,
		))
	}
}
