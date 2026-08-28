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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	apiconnect "github.com/prometheus/alertmanager/api/connect"
	apiv2 "github.com/prometheus/alertmanager/api/v2"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/types"
)

// API represents all APIs of Alertmanager.
type API struct {
	v2                *apiv2.API
	connect           *apiconnect.API
	deprecationRouter *V1DeprecationRouter

	requestDuration          *prometheus.HistogramVec
	requestsInFlight         prometheus.Gauge
	concurrencyLimitExceeded prometheus.Counter
	timeout                  time.Duration
	inFlightSem              chan struct{}
}

// Options for the creation of an API object. Alerts, Silences, AlertStatusFunc
// and GroupMutedFunc are mandatory. The zero value for everything else is a safe
// default.
type Options struct {
	// Alerts to be used by the API. Mandatory.
	Alerts provider.Alerts
	// Silences to be used by the API. Mandatory.
	Silences *silence.Silences
	// GroupMutedFunc is used be the API to know if an alert is muted.
	// Mandatory.
	GroupMutedFunc func(routeID, groupKey string) ([]string, bool)
	// Peer from the gossip cluster. If nil, no clustering will be used.
	Peer cluster.ClusterPeer
	// Timeout for HTTP requests and Connect unary RPCs. The zero value (and
	// negative values) result in no timeout.
	Timeout time.Duration
	// Concurrency limit for GET requests and, independently, Connect unary
	// RPCs and streams. The zero value (and negative values) result in a
	// limit of GOMAXPROCS or 8, whichever is larger. Status code 503 is served
	// for GET requests that would exceed the concurrency limit; Connect calls
	// receive ResourceExhausted.
	Concurrency int
	// Logger is used for logging, if nil, no logging will happen.
	Logger *slog.Logger
	// Registry is used to register Prometheus metrics. If nil, no metrics
	// registration will happen.
	Registry prometheus.Registerer
	// RequestDuration is used to measure the duration of HTTP requests.
	RequestDuration *prometheus.HistogramVec
	// GroupFunc returns a list of alert groups. The alerts are grouped
	// according to the current active configuration. Alerts returned are
	// filtered by the arguments provided to the function.
	GroupFunc func(context.Context, func(*dispatch.Route) bool, func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error)
}

func (o Options) validate() error {
	if o.Alerts == nil {
		return errors.New("mandatory field Alerts not set")
	}
	if o.Silences == nil {
		return errors.New("mandatory field Silences not set")
	}
	if o.GroupMutedFunc == nil {
		return errors.New("mandatory field GroupMutedFunc not set")
	}
	if o.GroupFunc == nil {
		return errors.New("mandatory field GroupFunc not set")
	}
	return nil
}

// New creates a new API object combining all API versions. Note that an Update
// call is also needed to get the APIs into an operational state.
func New(opts Options) (*API, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid API options: %w", err)
	}
	l := opts.Logger
	if l == nil {
		l = promslog.NewNopLogger()
	}
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = max(runtime.GOMAXPROCS(0), 8)
	}

	// The Connect API is always mounted alongside API v2.
	v2, err := apiv2.NewAPI(
		opts.Alerts,
		opts.GroupFunc,
		opts.GroupMutedFunc,
		opts.Silences,
		opts.Peer,
		l.With("version", "v2"),
		opts.Registry,
	)
	if err != nil {
		return nil, err
	}
	connect := apiconnect.NewAPI(apiconnect.Options{
		Peer:              opts.Peer,
		UnaryConcurrency:  concurrency,
		StreamConcurrency: concurrency,
		UnaryTimeout:      opts.Timeout,
	})

	requestsInFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "alertmanager_http_requests_in_flight",
		Help:        "Current number of HTTP requests being processed.",
		ConstLabels: prometheus.Labels{"method": "get"},
	})
	concurrencyLimitExceeded := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "alertmanager_http_concurrency_limit_exceeded_total",
		Help:        "Total number of times an HTTP request failed because the concurrency limit was reached.",
		ConstLabels: prometheus.Labels{"method": "get"},
	})
	if opts.Registry != nil {
		if err := opts.Registry.Register(requestsInFlight); err != nil {
			return nil, err
		}
		if err := opts.Registry.Register(concurrencyLimitExceeded); err != nil {
			return nil, err
		}
	}

	return &API{
		deprecationRouter:        NewV1DeprecationRouter(l.With("version", "v1")),
		v2:                       v2,
		connect:                  connect,
		requestDuration:          opts.RequestDuration,
		requestsInFlight:         requestsInFlight,
		concurrencyLimitExceeded: concurrencyLimitExceeded,
		timeout:                  opts.Timeout,
		inFlightSem:              make(chan struct{}, concurrency),
	}, nil
}

// Register API. As APIv2 works on the http.Handler level, this method also creates a new
// http.ServeMux and then uses it to register both the provided router (to
// handle "/") and APIv2 (to handle "<routePrefix>/api/v2"). The method returns
// the newly created http.ServeMux. Configured timeouts apply to regular HTTP
// requests and Connect unary RPCs. Regular HTTP GETs and Connect RPCs use
// independent concurrency limits; streams also have a separate limit.
func (api *API) Register(r *route.Router, routePrefix string) *http.ServeMux {
	// TODO(gotjosh) API V1 was removed as of version 0.27, when we reach 1.0.0 we should removed these deprecation warnings.
	api.deprecationRouter.Register(r.WithPrefix("/api/v1"))

	mux := http.NewServeMux()
	connectHandler := api.connect.Handler()
	// ConnectRPC procedure paths are the only bounded label values on the
	// Connect/gRPC surface; any other path yields a 404 and must not be
	// recorded verbatim, or a client could inflate metric/trace cardinality.
	servicePrefixes := api.connect.ServicePrefixes()
	// Native gRPC is served at the server root, so match against the raw
	// request path (no mount prefix).
	grpcHandler := api.instrumentConnectHandler("", servicePrefixes, connectHandler)
	webHandler := api.limitHandler(r)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGRPCRequest(r) {
			grpcHandler.ServeHTTP(w, r)
			return
		}
		webHandler.ServeHTTP(w, r)
	}))

	apiPrefix := ""
	if routePrefix != "/" {
		apiPrefix = routePrefix
	}

	// The v1 deprecation routes live on the base router r (mounted at "/").
	// Re-register them under the more specific /api/v1/ prefix so the
	// version-neutral Connect /api/ catch-all below does not shadow them.
	mux.Handle(apiPrefix+"/api/v1/", api.limitHandler(r))

	mux.Handle(
		apiPrefix+"/api/v2/",
		api.instrumentHandler(
			apiPrefix,
			api.limitHandler(
				http.StripPrefix(
					apiPrefix,
					api.v2.Handler,
				),
			),
		),
	)

	// Connect and gRPC-Web procedures are fully-qualified and carry their own
	// service version (e.g. /status.v3alpha.StatusService/GetStatus), so mount them
	// behind a version-neutral /api/ prefix. Native gRPC remains at the root so
	// standard clients do not need path-prefix support. The more specific
	// /api/v1/ and /api/v2/ patterns above win via longest-prefix matching.
	mux.Handle(
		apiPrefix+"/api/",
		api.instrumentConnectHandler(
			apiPrefix+"/api",
			servicePrefixes,
			http.StripPrefix(apiPrefix+"/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if isGRPCRequest(r) {
					http.NotFound(w, r)
					return
				}
				connectHandler.ServeHTTP(w, r)
			})),
		),
	)

	return mux
}

func isGRPCRequest(r *http.Request) bool {
	if r.ProtoMajor != 2 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && (mediaType == "application/grpc" || strings.HasPrefix(mediaType, "application/grpc+"))
}

// Update config and resolve timeout of each API. APIv2 also needs
// setAlertStatus to be updated.
func (api *API) Update(cfg *config.Config, setAlertStatus func(ctx context.Context, labels model.LabelSet)) {
	if api.v2 != nil {
		api.v2.Update(cfg, setAlertStatus)
	}
	if api.connect != nil {
		api.connect.Update(cfg)
	}
}

func (api *API) limitHandler(h http.Handler) http.Handler {
	limited := api.concurrencyLimitHandler(h)
	if api.timeout <= 0 {
		return limited
	}
	return http.TimeoutHandler(limited, api.timeout, fmt.Sprintf(
		"Exceeded configured timeout of %v.\n", api.timeout,
	))
}

func (api *API) concurrencyLimitHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rsp http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet { // Only limit concurrency of GETs.
			select {
			case api.inFlightSem <- struct{}{}: // All good, carry on.
				api.requestsInFlight.Inc()
				defer func() {
					<-api.inFlightSem
					api.requestsInFlight.Dec()
				}()
			default:
				api.concurrencyLimitExceeded.Inc()
				http.Error(rsp, fmt.Sprintf(
					"Limit of concurrent GET requests reached (%d), try again later.\n", cap(api.inFlightSem),
				), http.StatusServiceUnavailable)
				return
			}
		}
		h.ServeHTTP(rsp, req)
	})
}

func (api *API) instrumentHandler(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := strings.CutPrefix(r.URL.Path, prefix)
		// avoid high cardinality label values by replacing the actual silence IDs with a placeholder
		if strings.HasPrefix(path, "/api/v2/silence/") {
			path = "/api/v2/silence/{silenceID}"
		}
		promhttp.InstrumentHandlerDuration(
			api.requestDuration.MustCurryWith(prometheus.Labels{"handler": path}),
			otelhttp.NewHandler(h, path),
		).ServeHTTP(w, r)
	})
}

// unmatchedRPCLabel is the placeholder handler label and trace span name used
// for Connect/gRPC requests whose path does not correspond to a registered
// service. Collapsing these to a single value keeps clients from inflating
// metric and trace cardinality by hitting arbitrary paths.
const unmatchedRPCLabel = "unmatched"

// instrumentConnectHandler is like instrumentHandler but bounds label and
// span cardinality for the Connect/gRPC surface. Requests whose path (after
// stripping mountPrefix) matches a registered service are recorded under that
// service's prefix; the trailing method segment is intentionally dropped so a
// client cannot inflate cardinality by appending arbitrary (and 404-ing)
// method names. Everything else collapses to unmatchedRPCLabel.
func (api *API) instrumentConnectHandler(mountPrefix string, servicePrefixes []string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		procedure, _ := strings.CutPrefix(r.URL.Path, mountPrefix)
		label := unmatchedRPCLabel
		for _, p := range servicePrefixes {
			if strings.HasPrefix(procedure, p) {
				label = p
				break
			}
		}
		promhttp.InstrumentHandlerDuration(
			api.requestDuration.MustCurryWith(prometheus.Labels{"handler": label}),
			otelhttp.NewHandler(h, label),
		).ServeHTTP(w, r)
	})
}
