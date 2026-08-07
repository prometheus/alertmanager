// Copyright The Prometheus Authors
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

// Package apiconnect implements the experimental ConnectRPC-based
// Alertmanager API. It is always mounted alongside API v2, under the
// version-neutral /api/ prefix. ConnectRPC serves the Connect, gRPC, and
// gRPC-Web protocols from a single service definition. Each service is
// independently versioned (e.g. status.v3); the package itself carries no
// umbrella version.
package apiconnect

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/api/status/v3/statusv3connect"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
)

// API implements the ConnectRPC service handlers for the Connect API.
type API struct {
	mtx    sync.RWMutex
	logger *slog.Logger
	peer   cluster.ClusterPeer
	uptime time.Time

	alertmanagerConfig *config.Config
}

// NewAPI returns a new Connect API handler. Peer may be nil when clustering
// is disabled. If logger is nil, a no-op logger is used.
func NewAPI(peer cluster.ClusterPeer, logger *slog.Logger) *API {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	return &API{
		logger: logger,
		peer:   peer,
		uptime: time.Now(),
	}
}

// Update swaps in the currently loaded configuration. It is safe for
// concurrent use with the RPC handlers.
func (api *API) Update(cfg *config.Config) {
	api.mtx.Lock()
	defer api.mtx.Unlock()
	api.alertmanagerConfig = cfg
}

// Handler returns an http.Handler serving every ConnectRPC service exposed
// by the Connect API: the versioned application services, the gRPC Health
// Checking Protocol (grpc.health.v1.Health), and gRPC server reflection
// (v1 and v1alpha, for tools such as grpcurl). Procedures are
// fully-qualified, so the returned handler is mounted at a single prefix.
func (api *API) Handler(opts ...connect.HandlerOption) http.Handler {
	// serviceNames lists the fully-qualified service names advertised via
	// health checking and reflection.
	serviceNames := []string{
		statusv3connect.StatusServiceName,
	}

	mux := http.NewServeMux()

	mux.Handle(statusv3connect.NewStatusServiceHandler(api, opts...))

	mux.Handle(grpchealth.NewHandler(grpchealth.NewStaticChecker(serviceNames...), opts...))

	reflector := grpcreflect.NewStaticReflector(serviceNames...)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, opts...))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, opts...))

	return mux
}
