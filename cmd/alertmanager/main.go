// Copyright 2015 Prometheus Team
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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
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
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/alertmanager/ui"
	reactapp "github.com/prometheus/alertmanager/ui/react-app"
)

var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:                            "alertmanager_http_request_duration_seconds",
			Help:                            "Histogram of latencies for HTTP requests.",
			Buckets:                         []float64{.05, 0.1, .25, .5, .75, 1, 2, 5, 20, 60},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
		[]string{"handler", "method"},
	)
	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmanager_http_response_size_bytes",
			Help:    "Histogram of response size for HTTP requests.",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"handler", "method"},
	)
	clusterEnabled = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_cluster_enabled",
			Help: "Indicates whether the clustering is enabled or not.",
		},
	)
	configuredReceivers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_receivers",
			Help: "Number of configured receivers.",
		},
	)
	configuredIntegrations = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_integrations",
			Help: "Number of configured integrations.",
		},
	)
	configuredInhibitionRules = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "alertmanager_inhibition_rules",
			Help: "Number of configured inhibition rules.",
		})
	promlogConfig = promlog.Config{}
)

func init() {
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(responseSize)
	prometheus.MustRegister(clusterEnabled)
	prometheus.MustRegister(configuredReceivers)
	prometheus.MustRegister(configuredIntegrations)
	prometheus.MustRegister(configuredInhibitionRules)
	prometheus.MustRegister(collectors.NewBuildInfoCollector())
}

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
		configFile          = kingpin.Flag("config.file", "Alertmanager configuration file name.").Default("alertmanager.yml").String()
		dataDir             = kingpin.Flag("storage.path", "Base path for data storage.").Default("data/").String()
		retention           = kingpin.Flag("data.retention", "How long to keep data for.").Default("120h").Duration()
		maintenanceInterval = kingpin.Flag("data.maintenance-interval", "Interval between garbage collection and snapshotting to disk of the silences and the notification logs.").Default("15m").Duration()
		maxSilences         = kingpin.Flag("silences.max-silences", "Maximum number of silences, including expired silences. If negative or zero, no limit is set.").Default("0").Int()
		maxSilenceSizeBytes = kingpin.Flag("silences.max-silence-size-bytes", "Maximum silence size in bytes. If negative or zero, no limit is set.").Default("0").Int()
		alertGCInterval     = kingpin.Flag("alerts.gc-interval", "Interval between alert GC.").Default("30m").Duration()

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
		peers                  = kingpin.Flag("cluster.peer", "Initial peers (may be repeated).").Strings()
		peerTimeout            = kingpin.Flag("cluster.peer-timeout", "Time to wait between peers to send notifications.").Default("15s").Duration()
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
		featureFlags           = kingpin.Flag("enable-feature", fmt.Sprintf("Experimental features to enable. The flag can be repeated to enable multiple features. Valid options: %s", strings.Join(featurecontrol.AllowedFlags, ", "))).Default("").String()
	)

	promlogflag.AddFlags(kingpin.CommandLine, &promlogConfig)
	kingpin.CommandLine.UsageWriter(os.Stdout)

	kingpin.Version(version.Print("alertmanager"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promlog.New(&promlogConfig)

	level.Info(logger).Log("msg", "Starting Alertmanager", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	ff, err := featurecontrol.NewFlags(logger, *featureFlags)
	if err != nil {
		level.Error(logger).Log("msg", "error parsing the feature flag list", "err", err)
		return 1
	}
	compat.InitFromFlags(logger, ff)

	if ff.EnableAutoGOMEMLIMIT() {
		if *memlimitRatio <= 0.0 || *memlimitRatio > 1.0 {
			level.Error(logger).Log("msg", "--auto-gomemlimit.ratio must be greater than 0 and less than or equal to 1.")
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
			level.Warn(logger).Log("component", "automemlimit", "msg", "Failed to set GOMEMLIMIT automatically", "err", err)
		}
	}

	if ff.EnableAutoGOMAXPROCS() {
		l := func(format string, a ...interface{}) {
			level.Info(logger).Log("component", "automaxprocs", "msg", fmt.Sprintf(strings.TrimPrefix(format, "maxprocs: "), a...))
		}
		if _, err := maxprocs.Set(maxprocs.Logger(l)); err != nil {
			level.Warn(logger).Log("msg", "Failed to set GOMAXPROCS automatically", "err", err)
		}
	}

	err = os.MkdirAll(*dataDir, 0o777)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to create data directory", "err", err)
		return 1
	}

	tlsTransportConfig, err := cluster.GetTLSTransportConfig(*tlsConfigFile)
	if err != nil {
		level.Error(logger).Log("msg", "unable to initialize TLS transport configuration for gossip mesh", "err", err)
		return 1
	}
	var peer *cluster.Peer
	if *clusterBindAddr != "" {
		peer, err = cluster.Create(
			log.With(logger, "component", "cluster"),
			prometheus.DefaultRegisterer,
			*clusterBindAddr,
			*clusterAdvertiseAddr,
			*peers,
			true,
			*pushPullInterval,
			*gossipInterval,
			*tcpTimeout,
			*probeTimeout,
			*probeInterval,
			tlsTransportConfig,
			*allowInsecureAdvertise,
			*label,
		)
		if err != nil {
			level.Error(logger).Log("msg", "unable to initialize gossip mesh", "err", err)
			return 1
		}
		clusterEnabled.Set(1)
	}

	stopc := make(chan struct{})
	var wg sync.WaitGroup

	notificationLogOpts := nflog.Options{
		SnapshotFile: filepath.Join(*dataDir, "nflog"),
		Retention:    *retention,
		Logger:       log.With(logger, "component", "nflog"),
		Metrics:      prometheus.DefaultRegisterer,
	}

	notificationLog, err := nflog.New(notificationLogOpts)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}
	if peer != nil {
		c := peer.AddState("nfl", notificationLog, prometheus.DefaultRegisterer)
		notificationLog.SetBroadcast(c.Broadcast)
	}

	wg.Add(1)
	go func() {
		notificationLog.Maintenance(*maintenanceInterval, filepath.Join(*dataDir, "nflog"), stopc, nil)
		wg.Done()
	}()

	marker := types.NewMarker(prometheus.DefaultRegisterer)

	silenceOpts := silence.Options{
		SnapshotFile: filepath.Join(*dataDir, "silences"),
		Retention:    *retention,
		Limits: silence.Limits{
			MaxSilences:         func() int { return *maxSilences },
			MaxSilenceSizeBytes: func() int { return *maxSilenceSizeBytes },
		},
		Logger:  log.With(logger, "component", "silences"),
		Metrics: prometheus.DefaultRegisterer,
	}

	silences, err := silence.New(silenceOpts)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}
	if peer != nil {
		c := peer.AddState("sil", silences, prometheus.DefaultRegisterer)
		silences.SetBroadcast(c.Broadcast)
	}

	// Start providers before router potentially sends updates.
	wg.Add(1)
	go func() {
		silences.Maintenance(*maintenanceInterval, filepath.Join(*dataDir, "silences"), stopc, nil)
		wg.Done()
	}()

	defer func() {
		close(stopc)
		wg.Wait()
	}()

	// Peer state listeners have been registered, now we can join and get the initial state.
	if peer != nil {
		err = peer.Join(
			*reconnectInterval,
			*peerReconnectTimeout,
		)
		if err != nil {
			level.Warn(logger).Log("msg", "unable to join gossip mesh", "err", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), *settleTimeout)
		defer func() {
			cancel()
			if err := peer.Leave(10 * time.Second); err != nil {
				level.Warn(logger).Log("msg", "unable to leave gossip mesh", "err", err)
			}
		}()
		go peer.Settle(ctx, *gossipInterval*10)
	}

	alerts, err := mem.NewAlerts(context.Background(), marker, *alertGCInterval, nil, logger, prometheus.DefaultRegisterer)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}
	defer alerts.Close()

	var disp *dispatch.Dispatcher
	defer func() {
		disp.Stop()
	}()

	groupFn := func(routeFilter func(*dispatch.Route) bool, alertFilter func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string) {
		return disp.Groups(routeFilter, alertFilter)
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
		Logger:          log.With(logger, "component", "api"),
		Registry:        prometheus.DefaultRegisterer,
		GroupFunc:       groupFn,
	})
	if err != nil {
		level.Error(logger).Log("err", fmt.Errorf("failed to create API: %w", err))
		return 1
	}

	amURL, err := extURL(logger, os.Hostname, (*webConfig.WebListenAddresses)[0], *externalURL)
	if err != nil {
		level.Error(logger).Log("msg", "failed to determine external URL", "err", err)
		return 1
	}
	level.Debug(logger).Log("externalURL", amURL.String())

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

	var (
		inhibitor *inhibit.Inhibitor
		tmpl      *template.Template
	)

	dispMetrics := dispatch.NewDispatcherMetrics(false, prometheus.DefaultRegisterer)
	pipelineBuilder := notify.NewPipelineBuilder(prometheus.DefaultRegisterer, ff)
	configLogger := log.With(logger, "component", "configuration")
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
				level.Info(configLogger).Log("msg", "skipping creation of receiver not referenced by any route", "receiver", rcv.Name)
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

		inhibitor.Stop()
		disp.Stop()

		inhibitor = inhibit.NewInhibitor(alerts, conf.InhibitRules, marker, logger)
		silencer := silence.NewSilencer(silences, marker, logger)

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
			inhibitor,
			silencer,
			intervener,
			marker,
			notificationLog,
			pipelinePeer,
		)

		configuredReceivers.Set(float64(len(activeReceivers)))
		configuredIntegrations.Set(float64(integrationsNum))
		configuredInhibitionRules.Set(float64(len(conf.InhibitRules)))

		api.Update(conf, func(labels model.LabelSet) {
			inhibitor.Mutes(labels)
			silencer.Mutes(labels)
		})

		disp = dispatch.NewDispatcher(alerts, routes, pipeline, marker, timeoutFunc, nil, logger, dispMetrics)
		routes.Walk(func(r *dispatch.Route) {
			if r.RouteOpts.RepeatInterval > *retention {
				level.Warn(configLogger).Log(
					"msg",
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
				level.Warn(configLogger).Log(
					"msg",
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

		go disp.Run()
		go inhibitor.Run()

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
	level.Debug(logger).Log("routePrefix", *routePrefix)

	router := route.New().WithInstrumentation(instrumentHandler)
	if *routePrefix != "/" {
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, *routePrefix, http.StatusFound)
		})
		router = router.WithPrefix(*routePrefix)
	}

	webReload := make(chan chan error)

	ui.Register(router, webReload, logger)
	reactapp.Register(router, logger)

	mux := api.Register(router, *routePrefix)

	srv := &http.Server{Handler: mux}
	srvc := make(chan struct{})

	go func() {
		if err := web.ListenAndServe(srv, webConfig, logger); !errors.Is(err, http.ErrServerClosed) {
			level.Error(logger).Log("msg", "Listen error", "err", err)
			close(srvc)
		}
		defer func() {
			if err := srv.Close(); err != nil {
				level.Error(logger).Log("msg", "Error on closing the server", "err", err)
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
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
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

func extURL(logger log.Logger, hostnamef func() (string, error), listen, external string) (*url.URL, error) {
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
			level.Warn(logger).Log("msg", "no port found for listen address", "address", listen)
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
