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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/prometheus/alertmanager/api"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/config/receiver"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/alertmanager/tracing"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/alertmanager/ui"
)

var (
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "alertmanager_http_request_duration_seconds",
			Help:                            "Histogram of latencies for HTTP requests.",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{"handler", "method", "code"},
	)
	responseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmanager_http_response_size_bytes",
			Help:    "Histogram of response size for HTTP requests.",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"handler", "method"},
	)
	clusterEnabled = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_cluster_enabled",
			Help: "Indicates whether the clustering is enabled or not.",
		},
	)
	configuredReceivers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_receivers",
			Help: "Number of configured receivers.",
		},
	)
	configuredIntegrations = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_integrations",
			Help: "Number of configured integrations.",
		},
	)
	configuredInhibitionRules = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_inhibition_rules",
			Help: "Number of configured inhibition rules.",
		},
	)

	promslogConfig = promslog.Config{}
)

func instrumentHandler(handlerName string, handler http.HandlerFunc) http.HandlerFunc {
	handlerLabel := prometheus.Labels{"handler": handlerName}
	return promhttp.InstrumentHandlerDuration(
		requestDuration.MustCurryWith(handlerLabel),
		promhttp.InstrumentHandlerResponseSize(
			responseSize.MustCurryWith(handlerLabel),
			handler,
		),
	)
}

const defaultClusterAddr = "0.0.0.0:9094"

func main() {
	os.Exit(run())
}

func run() int {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	var (
		configFile                  = kingpin.Flag("config.file", "Alertmanager configuration file name.").Default("alertmanager.yml").String()
		dataDir                     = kingpin.Flag("storage.path", "Base path for data storage.").Default("data/").String()
		retention                   = kingpin.Flag("data.retention", "How long to keep data for.").Default("120h").Duration()
		maintenanceInterval         = kingpin.Flag("data.maintenance-interval", "Interval between garbage collection and snapshotting to disk of the silences and the notification logs.").Default("15m").Duration()
		maxSilences                 = kingpin.Flag("silences.max-silences", "Maximum number of silences, including expired silences. If negative or zero, no limit is set.").Default("0").Int()
		maxSilenceSizeBytes         = kingpin.Flag("silences.max-silence-size-bytes", "Maximum silence size in bytes. If negative or zero, no limit is set.").Default("0").Int()
		alertGCInterval             = kingpin.Flag("alerts.gc-interval", "Interval between alert GC.").Default("30m").Duration()
		perAlertNameLimit           = kingpin.Flag("alerts.per-alertname-limit", "Maximum number of alerts per alertname. If negative or zero, no limit is set.").Default("0").Int()
		dispatchMaintenanceInterval = kingpin.Flag("dispatch.maintenance-interval", "Interval between maintenance of aggregation groups in the dispatcher.").Default("30s").Duration()
		DispatchStartDelay          = kingpin.Flag("dispatch.start-delay", "Minimum amount of time to wait before dispatching alerts. This option should be synced with value of --rules.alert.resend-delay on Prometheus.").Default("0s").Duration()

		webConfig      = webflag.AddFlags(kingpin.CommandLine, ":9093")
		externalURL    = kingpin.Flag("web.external-url", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.").String()
		routePrefix    = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").String()
		getConcurrency = kingpin.Flag("web.get-concurrency", "Maximum number of GET requests processed concurrently. If negative or zero, the limit is GOMAXPROC or 8, whichever is larger.").Default("0").Int()
		httpTimeout    = kingpin.Flag("web.timeout", "Timeout for HTTP requests. If negative or zero, no timeout is set.").Default("0").Duration()

		memlimitRatio = kingpin.Flag("auto-gomemlimit.ratio", "The ratio of reserved GOMEMLIMIT memory to the detected maximum container or system memory. The value must be greater than 0 and less than or equal to 1.").
				Default("0.9").Float64()

		clusterBindAddr = kingpin.Flag("cluster.listen-address", "Listen address for cluster. Set to empty string to disable HA mode.").
				Default(defaultClusterAddr).String()
		clusterAdvertiseAddr   = kingpin.Flag("cluster.advertise-address", "Explicit address to advertise in cluster.").String()
		clusterPeerName        = kingpin.Flag("cluster.peer-name", "Explicit name of the peer, rather than generating a random one").Default("").String()
		peers                  = kingpin.Flag("cluster.peer", "Initial peers (may be repeated).").Strings()
		peerTimeout            = kingpin.Flag("cluster.peer-timeout", "Time to wait between peers to send notifications.").Default("15s").Duration()
		peersResolveTimeout    = kingpin.Flag("cluster.peers-resolve-timeout", "Time to resolve peers.").Default(cluster.DefaultResolvePeersTimeout.String()).Duration()
		gossipInterval         = kingpin.Flag("cluster.gossip-interval", "Interval between sending gossip messages. By lowering this value (more frequent) gossip messages are propagated across the cluster more quickly at the expense of increased bandwidth.").Default(cluster.DefaultGossipInterval.String()).Duration()
		pushPullInterval       = kingpin.Flag("cluster.pushpull-interval", "Interval for gossip state syncs. Setting this interval lower (more frequent) will increase convergence speeds across larger clusters at the expense of increased bandwidth usage.").Default(cluster.DefaultPushPullInterval.String()).Duration()
		tcpTimeout             = kingpin.Flag("cluster.tcp-timeout", "Timeout for establishing a stream connection with a remote node for a full state sync, and for stream read and write operations.").Default(cluster.DefaultTCPTimeout.String()).Duration()
		probeTimeout           = kingpin.Flag("cluster.probe-timeout", "Timeout to wait for an ack from a probed node before assuming it is unhealthy. This should be set to 99-percentile of RTT (round-trip time) on your network.").Default(cluster.DefaultProbeTimeout.String()).Duration()
		probeInterval          = kingpin.Flag("cluster.probe-interval", "Interval between random node probes. Setting this lower (more frequent) will cause the cluster to detect failed nodes more quickly at the expense of increased bandwidth usage.").Default(cluster.DefaultProbeInterval.String()).Duration()
		settleTimeout          = kingpin.Flag("cluster.settle-timeout", "Maximum time to wait for cluster connections to settle before evaluating notifications.").Default(cluster.DefaultPushPullInterval.String()).Duration()
		reconnectInterval      = kingpin.Flag("cluster.reconnect-interval", "Interval between attempting to reconnect to lost peers.").Default(cluster.DefaultReconnectInterval.String()).Duration()
		peerReconnectTimeout   = kingpin.Flag("cluster.reconnect-timeout", "Length of time to attempt to reconnect to a lost peer.").Default(cluster.DefaultReconnectTimeout.String()).Duration()
		tlsConfigFile          = kingpin.Flag("cluster.tls-config", "[EXPERIMENTAL] Path to config yaml file that can enable mutual TLS within the gossip protocol.").Default("").String()
		allowInsecureAdvertise = kingpin.Flag("cluster.allow-insecure-public-advertise-address-discovery", "[EXPERIMENTAL] Allow alertmanager to discover and listen on a public IP address.").Bool()
		label                  = kingpin.Flag("cluster.label", "The cluster label is an optional string to include on each packet and stream. It uniquely identifies the cluster and prevents cross-communication issues when sending gossip messages.").Default("").String()
		featureFlags           = kingpin.Flag("enable-feature", fmt.Sprintf("Comma-separated experimental features to enable. Valid options: %s", strings.Join(featurecontrol.AllowedFlags, ", "))).Default("").String()
	)

	prometheus.MustRegister(versioncollector.NewCollector("alertmanager"))

	promslogflag.AddFlags(kingpin.CommandLine, &promslogConfig)
	kingpin.CommandLine.UsageWriter(os.Stdout)

	kingpin.Version(version.Print("alertmanager"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promslog.New(&promslogConfig)

	logger.Info("Starting Alertmanager", "version", version.Info())
	startTime := time.Now()

	logger.Info("Build context", "build_context", version.BuildContext())

	ff, err := featurecontrol.NewFlags(logger, *featureFlags)
	if err != nil {
		logger.Error("error parsing the feature flag list", "err", err)
		return 1
	}
	compat.InitFromFlags(logger, ff)

	if ff.EnableAutoGOMEMLIMIT() {
		if *memlimitRatio <= 0.0 || *memlimitRatio > 1.0 {
			logger.Error("--auto-gomemlimit.ratio must be greater than 0 and less than or equal to 1.")
			return 1
		}

		if _, err := memlimit.SetGoMemLimitWithOpts(
			memlimit.WithRatio(*memlimitRatio),
			memlimit.WithProvider(
				memlimit.ApplyFallback(
					memlimit.FromCgroup,
					memlimit.FromSystem,
				),
			),
		); err != nil {
			logger.Warn("automemlimit", "msg", "Failed to set GOMEMLIMIT automatically", "err", err)
		}
	}

	if ff.EnableAutoGOMAXPROCS() {
		l := func(format string, a ...any) {
			logger.Info("automaxprocs", "msg", fmt.Sprintf(strings.TrimPrefix(format, "maxprocs: "), a...))
		}
		if _, err := maxprocs.Set(maxprocs.Logger(l)); err != nil {
			logger.Warn("Failed to set GOMAXPROCS automatically", "err", err)
		}
	}

	err = os.MkdirAll(*dataDir, 0o777)
	if err != nil {
		logger.Error("Unable to create data directory", "err", err)
		return 1
	}

	tlsTransportConfig, err := cluster.GetTLSTransportConfig(*tlsConfigFile)
	if err != nil {
		logger.Error("unable to initialize TLS transport configuration for gossip mesh", "err", err)
		return 1
	}
	var peer *cluster.Peer
	if *clusterBindAddr != "" {
		peer, err = cluster.Create(
			logger.With("component", "cluster"),
			prometheus.DefaultRegisterer,
			*clusterBindAddr,
			*clusterAdvertiseAddr,
			*peers,
			true,
			*pushPullInterval,
			*gossipInterval,
			*tcpTimeout,
			*peersResolveTimeout,
			*probeTimeout,
			*probeInterval,
			tlsTransportConfig,
			*allowInsecureAdvertise,
			*label,
			*clusterPeerName,
		)
		if err != nil {
			logger.Error("unable to initialize gossip mesh", "err", err)
			return 1
		}
		clusterEnabled.Set(1)
	}

	stopc := make(chan struct{})
	var wg sync.WaitGroup

	notificationLogOpts := nflog.Options{
		SnapshotFile: filepath.Join(*dataDir, "nflog"),
		Retention:    *retention,
		Logger:       logger.With("component", "nflog"),
		Metrics:      prometheus.DefaultRegisterer,
	}

	notificationLog, err := nflog.New(notificationLogOpts)
	if err != nil {
		logger.Error("error creating notification log", "err", err)
		return 1
	}
	if peer != nil {
		c := peer.AddState("nfl", notificationLog, prometheus.DefaultRegisterer)
		notificationLog.SetBroadcast(c.Broadcast)
	}

	wg.Go(func() {
		notificationLog.Maintenance(*maintenanceInterval, filepath.Join(*dataDir, "nflog"), stopc, nil)
	})

	marker := types.NewMarker(prometheus.DefaultRegisterer)

	silenceOpts := silence.Options{
		SnapshotFile: filepath.Join(*dataDir, "silences"),
		Retention:    *retention,
		Limits: silence.Limits{
			MaxSilences:         func() int { return *maxSilences },
			MaxSilenceSizeBytes: func() int { return *maxSilenceSizeBytes },
		},
		Logger:  logger.With("component", "silences"),
		Metrics: prometheus.DefaultRegisterer,
	}

	silences, err := silence.New(silenceOpts)
	if err != nil {
		logger.Error("error creating silence", "err", err)
		return 1
	}
	if peer != nil {
		c := peer.AddState("sil", silences, prometheus.DefaultRegisterer)
		silences.SetBroadcast(c.Broadcast)
	}

	// Start providers before router potentially sends updates.
	wg.Go(func() {
		silences.Maintenance(*maintenanceInterval, filepath.Join(*dataDir, "silences"), stopc, nil)
	})

	defer func() {
		close(stopc)
		wg.Wait()
	}()

	silencer := silence.NewSilencer(silences, marker, logger)

	// Peer state listeners have been registered, now we can join and get the initial state.
	if peer != nil {
		err = peer.Join(
			*reconnectInterval,
			*peerReconnectTimeout,
		)
		if err != nil {
			logger.Warn("unable to join gossip mesh", "err", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), *settleTimeout)
		defer func() {
			cancel()
			if err := peer.Leave(10 * time.Second); err != nil {
				logger.Warn("unable to leave gossip mesh", "err", err)
			}
		}()
		go peer.Settle(ctx, *gossipInterval*10)
	}

	alerts, err := mem.NewAlerts(
		context.Background(),
		marker,
		*alertGCInterval,
		*perAlertNameLimit,
		silencer,
		logger,
		prometheus.DefaultRegisterer,
		ff,
	)
	if err != nil {
		logger.Error("error creating memory provider", "err", err)
		return 1
	}
	defer alerts.Close()

	var disp atomic.Pointer[dispatch.Dispatcher]
	defer func() {
		disp.Load().Stop()
	}()

	groupFn := func(ctx context.Context, routeFilter func(*dispatch.Route) bool, alertFilter func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error) {
		return disp.Load().Groups(ctx, routeFilter, alertFilter)
	}

	// An interface value that holds a nil concrete value is non-nil.
	// Therefore we explicly pass an empty interface, to detect if the
	// cluster is not enabled in notify.
	var clusterPeer cluster.ClusterPeer
	if peer != nil {
		clusterPeer = peer
	}

	api, err := api.New(api.Options{
		Alerts:          alerts,
		Silences:        silences,
		AlertStatusFunc: marker.Status,
		GroupMutedFunc:  marker.Muted,
		Peer:            clusterPeer,
		Timeout:         *httpTimeout,
		Concurrency:     *getConcurrency,
		Logger:          logger.With("component", "api"),
		Registry:        prometheus.DefaultRegisterer,
		RequestDuration: requestDuration,
		GroupFunc:       groupFn,
	})
	if err != nil {
		logger.Error("failed to create API", "err", err)
		return 1
	}

	amURL, err := extURL(logger, os.Hostname, (*webConfig.WebListenAddresses)[0], *externalURL)
	if err != nil {
		logger.Error("failed to determine external URL", "err", err)
		return 1
	}
	logger.Debug("external url", "externalUrl", amURL.String())

	waitFunc := func() time.Duration { return 0 }
	if peer != nil {
		waitFunc = clusterWait(peer, *peerTimeout)
	}
	timeoutFunc := func(d time.Duration) time.Duration {
		if d < notify.MinTimeout {
			d = notify.MinTimeout
		}
		return d + waitFunc()
	}

	tracingManager := tracing.NewManager(logger.With("component", "tracing"))

	var (
		inhibitor atomic.Pointer[inhibit.Inhibitor]
		tmpl      *template.Template
	)

	dispMetrics := dispatch.NewDispatcherMetrics(false, prometheus.DefaultRegisterer)
	pipelineBuilder := notify.NewPipelineBuilder(prometheus.DefaultRegisterer, ff)
	configLogger := logger.With("component", "configuration")
	configCoordinator := config.NewCoordinator(
		*configFile,
		prometheus.DefaultRegisterer,
		configLogger,
	)
	configCoordinator.Subscribe(func(conf *config.Config) error {
		tmpl, err = template.FromGlobs(conf.Templates)
		if err != nil {
			return fmt.Errorf("failed to parse templates: %w", err)
		}
		tmpl.ExternalURL = amURL

		// Build the routing tree and record which receivers are used.
		routes := dispatch.NewRoute(conf.Route, nil)
		activeReceivers := make(map[string]struct{})
		routes.Walk(func(r *dispatch.Route) {
			activeReceivers[r.RouteOpts.Receiver] = struct{}{}
		})

		// Build the map of receiver to integrations.
		receivers := make(map[string][]notify.Integration, len(activeReceivers))
		var integrationsNum int
		for _, rcv := range conf.Receivers {
			if _, found := activeReceivers[rcv.Name]; !found {
				// No need to build a receiver if no route is using it.
				configLogger.Info("skipping creation of receiver not referenced by any route", "receiver", rcv.Name)
				continue
			}
			integrations, err := receiver.BuildReceiverIntegrations(rcv, tmpl, logger)
			if err != nil {
				return err
			}
			// rcv.Name is guaranteed to be unique across all receivers.
			receivers[rcv.Name] = integrations
			integrationsNum += len(integrations)
		}

		// Build the map of time interval names to time interval definitions.
		timeIntervals := make(map[string][]timeinterval.TimeInterval, len(conf.MuteTimeIntervals)+len(conf.TimeIntervals))
		for _, ti := range conf.MuteTimeIntervals {
			timeIntervals[ti.Name] = ti.TimeIntervals
		}

		for _, ti := range conf.TimeIntervals {
			timeIntervals[ti.Name] = ti.TimeIntervals
		}

		intervener := timeinterval.NewIntervener(timeIntervals)

		inhibitor.Load().Stop()
		disp.Load().Stop()

		newInhibitor := inhibit.NewInhibitor(alerts, conf.InhibitRules, marker, logger)
		inhibitor.Store(newInhibitor)

		// An interface value that holds a nil concrete value is non-nil.
		// Therefore we explicly pass an empty interface, to detect if the
		// cluster is not enabled in notify.
		var pipelinePeer notify.Peer
		if peer != nil {
			pipelinePeer = peer
		}

		pipeline := pipelineBuilder.New(
			receivers,
			waitFunc,
			newInhibitor,
			silencer,
			intervener,
			marker,
			notificationLog,
			pipelinePeer,
		)

		configuredReceivers.Set(float64(len(activeReceivers)))
		configuredIntegrations.Set(float64(integrationsNum))
		configuredInhibitionRules.Set(float64(len(conf.InhibitRules)))

		api.Update(conf, func(ctx context.Context, labels model.LabelSet) {
			inhibitor.Load().Mutes(ctx, labels)
			silencer.Mutes(ctx, labels)
		})

		newDisp := dispatch.NewDispatcher(
			alerts,
			routes,
			pipeline,
			marker,
			timeoutFunc,
			*dispatchMaintenanceInterval,
			nil,
			logger,
			dispMetrics,
		)
		routes.Walk(func(r *dispatch.Route) {
			if r.RouteOpts.RepeatInterval > *retention {
				configLogger.Warn(
					"repeat_interval is greater than the data retention period. It can lead to notifications being repeated more often than expected.",
					"repeat_interval",
					r.RouteOpts.RepeatInterval,
					"retention",
					*retention,
					"route",
					r.Key(),
				)
			}

			if r.RouteOpts.RepeatInterval < r.RouteOpts.GroupInterval {
				configLogger.Warn(
					"repeat_interval is less than group_interval. Notifications will not repeat until the next group_interval.",
					"repeat_interval",
					r.RouteOpts.RepeatInterval,
					"group_interval",
					r.RouteOpts.GroupInterval,
					"route",
					r.Key(),
				)
			}
		})

		// first, start the inhibitor so the inhibition cache can populate
		// wait for this to load alerts before starting the dispatcher so
		// we don't accidentially notify for an alert that will be inhibited
		go newInhibitor.Run()
		newInhibitor.WaitForLoading()

		// next, start the dispatcher and wait for it to load before swapping the disp pointer.
		// This ensures that the API doesn't see the new dispatcher before it finishes populating
		// the aggrGroups
		go newDisp.Run(startTime.Add(*DispatchStartDelay))
		newDisp.WaitForLoading()
		disp.Store(newDisp)

		err = tracingManager.ApplyConfig(conf.TracingConfig)
		if err != nil {
			return fmt.Errorf("failed to apply tracing config: %w", err)
		}

		go tracingManager.Run()

		return nil
	})

	if err := configCoordinator.Reload(); err != nil {
		return 1
	}

	// Make routePrefix default to externalURL path if empty string.
	if *routePrefix == "" {
		*routePrefix = amURL.Path
	}
	*routePrefix = "/" + strings.Trim(*routePrefix, "/")
	logger.Debug("route prefix", "routePrefix", *routePrefix)

	router := route.New().WithInstrumentation(instrumentHandler)
	if *routePrefix != "/" {
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, *routePrefix, http.StatusFound)
		})
		router = router.WithPrefix(*routePrefix)
	}

	webReload := make(chan chan error)

	ui.Register(router, webReload, logger)

	mux := api.Register(router, *routePrefix)

	srv := &http.Server{
		// instrument all handlers with tracing
		Handler: tracing.Middleware(mux),
	}
	srvc := make(chan struct{})

	go func() {
		if err := web.ListenAndServe(srv, webConfig, logger); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Listen error", "err", err)
			close(srvc)
		}
		defer func() {
			if err := srv.Close(); err != nil {
				logger.Error("Error on closing the server", "err", err)
			}
		}()
	}()

	var (
		hup  = make(chan os.Signal, 1)
		term = make(chan os.Signal, 1)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-hup:
			// ignore error, already logged in `reload()`
			_ = configCoordinator.Reload()
		case errc := <-webReload:
			errc <- configCoordinator.Reload()
		case <-term:
			logger.Info("Received SIGTERM, exiting gracefully...")

			// shut down the tracing manager to flush any remaining spans.
			// this blocks for up to 5s
			tracingManager.Stop()

			return 0
		case <-srvc:
			return 1
		}
	}
}

// clusterWait returns a function that inspects the current peer state and returns
// a duration of one base timeout for each peer with a higher ID than ourselves.
func clusterWait(p *cluster.Peer, timeout time.Duration) func() time.Duration {
	return func() time.Duration {
		return time.Duration(p.Position()) * timeout
	}
}

func extURL(logger *slog.Logger, hostnamef func() (string, error), listen, external string) (*url.URL, error) {
	if external == "" {
		hostname, err := hostnamef()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listen)
		if err != nil {
			return nil, err
		}
		if port == "" {
			logger.Warn("no port found for listen address", "address", listen)
		}

		external = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	u, err := url.Parse(external)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%q: invalid %q scheme, only 'http' and 'https' are supported", u.String(), u.Scheme)
	}

	ppref := strings.TrimRight(u.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	u.Path = ppref

	return u, nil
}
