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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/api"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/alertmanager/ui"
)

var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmanager_http_request_duration_seconds",
			Help:    "Histogram of latencies for HTTP requests.",
			Buckets: []float64{.05, 0.1, .25, .5, .75, 1, 2, 5, 20, 60},
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
	promlogConfig = promlog.Config{}
)

func init() {
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(responseSize)
	prometheus.MustRegister(version.NewCollector("alertmanager"))
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
		configFile      = kingpin.Flag("config.file", "Alertmanager configuration file name.").Default("alertmanager.yml").String()
		dataDir         = kingpin.Flag("storage.path", "Base path for data storage.").Default("data/").String()
		retention       = kingpin.Flag("data.retention", "How long to keep data for.").Default("120h").Duration()
		alertGCInterval = kingpin.Flag("alerts.gc-interval", "Interval between alert GC.").Default("30m").Duration()

		externalURL    = kingpin.Flag("web.external-url", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.").String()
		routePrefix    = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").String()
		listenAddress  = kingpin.Flag("web.listen-address", "Address to listen on for the web interface and API.").Default(":9093").String()
		getConcurrency = kingpin.Flag("web.get-concurrency", "Maximum number of GET requests processed concurrently. If negative or zero, the limit is GOMAXPROC or 8, whichever is larger.").Default("0").Int()
		httpTimeout    = kingpin.Flag("web.timeout", "Timeout for HTTP requests. If negative or zero, no timeout is set.").Default("0").Duration()

		clusterBindAddr = kingpin.Flag("cluster.listen-address", "Listen address for cluster.").
				Default(defaultClusterAddr).String()
		clusterAdvertiseAddr = kingpin.Flag("cluster.advertise-address", "Explicit address to advertise in cluster.").String()
		peers                = kingpin.Flag("cluster.peer", "Initial peers (may be repeated).").Strings()
		peerTimeout          = kingpin.Flag("cluster.peer-timeout", "Time to wait between peers to send notifications.").Default("15s").Duration()
		gossipInterval       = kingpin.Flag("cluster.gossip-interval", "Interval between sending gossip messages. By lowering this value (more frequent) gossip messages are propagated across the cluster more quickly at the expense of increased bandwidth.").Default(cluster.DefaultGossipInterval.String()).Duration()
		pushPullInterval     = kingpin.Flag("cluster.pushpull-interval", "Interval for gossip state syncs. Setting this interval lower (more frequent) will increase convergence speeds across larger clusters at the expense of increased bandwidth usage.").Default(cluster.DefaultPushPullInterval.String()).Duration()
		tcpTimeout           = kingpin.Flag("cluster.tcp-timeout", "Timeout for establishing a stream connection with a remote node for a full state sync, and for stream read and write operations.").Default(cluster.DefaultTcpTimeout.String()).Duration()
		probeTimeout         = kingpin.Flag("cluster.probe-timeout", "Timeout to wait for an ack from a probed node before assuming it is unhealthy. This should be set to 99-percentile of RTT (round-trip time) on your network.").Default(cluster.DefaultProbeTimeout.String()).Duration()
		probeInterval        = kingpin.Flag("cluster.probe-interval", "Interval between random node probes. Setting this lower (more frequent) will cause the cluster to detect failed nodes more quickly at the expense of increased bandwidth usage.").Default(cluster.DefaultProbeInterval.String()).Duration()
		settleTimeout        = kingpin.Flag("cluster.settle-timeout", "Maximum time to wait for cluster connections to settle before evaluating notifications.").Default(cluster.DefaultPushPullInterval.String()).Duration()
		reconnectInterval    = kingpin.Flag("cluster.reconnect-interval", "Interval between attempting to reconnect to lost peers.").Default(cluster.DefaultReconnectInterval.String()).Duration()
		peerReconnectTimeout = kingpin.Flag("cluster.reconnect-timeout", "Length of time to attempt to reconnect to a lost peer.").Default(cluster.DefaultReconnectTimeout.String()).Duration()
	)

	promlogflag.AddFlags(kingpin.CommandLine, &promlogConfig)

	kingpin.Version(version.Print("alertmanager"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promlog.New(&promlogConfig)

	level.Info(logger).Log("msg", "Starting Alertmanager", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	err := os.MkdirAll(*dataDir, 0777)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to create data directory", "err", err)
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
		)
		if err != nil {
			level.Error(logger).Log("msg", "unable to initialize gossip mesh", "err", err)
			return 1
		}
	}

	stopc := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	notificationLogOpts := []nflog.Option{
		nflog.WithRetention(*retention),
		nflog.WithSnapshot(filepath.Join(*dataDir, "nflog")),
		nflog.WithMaintenance(15*time.Minute, stopc, wg.Done),
		nflog.WithMetrics(prometheus.DefaultRegisterer),
		nflog.WithLogger(log.With(logger, "component", "nflog")),
	}

	notificationLog, err := nflog.New(notificationLogOpts...)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}
	if peer != nil {
		c := peer.AddState("nfl", notificationLog, prometheus.DefaultRegisterer)
		notificationLog.SetBroadcast(c.Broadcast)
	}

	marker := types.NewMarker(prometheus.DefaultRegisterer)

	silenceOpts := silence.Options{
		SnapshotFile: filepath.Join(*dataDir, "silences"),
		Retention:    *retention,
		Logger:       log.With(logger, "component", "silences"),
		Metrics:      prometheus.DefaultRegisterer,
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
		silences.Maintenance(15*time.Minute, filepath.Join(*dataDir, "silences"), stopc)
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

	alerts, err := mem.NewAlerts(context.Background(), marker, *alertGCInterval, logger)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}
	defer alerts.Close()

	var disp *dispatch.Dispatcher
	defer disp.Stop()

	groupFn := func(routeFilter func(*dispatch.Route) bool, alertFilter func(*types.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string) {
		return disp.Groups(routeFilter, alertFilter)
	}

	api, err := api.New(api.Options{
		Alerts:      alerts,
		Silences:    silences,
		StatusFunc:  marker.Status,
		Peer:        peer,
		Timeout:     *httpTimeout,
		Concurrency: *getConcurrency,
		Logger:      log.With(logger, "component", "api"),
		Registry:    prometheus.DefaultRegisterer,
		GroupFunc:   groupFn,
	})

	if err != nil {
		level.Error(logger).Log("err", fmt.Errorf("failed to create API: %v", err.Error()))
		return 1
	}

	amURL, err := extURL(*listenAddress, *externalURL)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}

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
		silencer  *silence.Silencer
		tmpl      *template.Template
		pipeline  notify.Stage
	)

	configCoordinator := config.NewCoordinator(
		*configFile,
		prometheus.DefaultRegisterer,
		log.With(logger, "component", "configuration"),
	)
	configCoordinator.Subscribe(func(conf *config.Config) error {
		tmpl, err = template.FromGlobs(conf.Templates...)
		if err != nil {
			return fmt.Errorf("failed to parse templates: %v", err.Error())
		}
		tmpl.ExternalURL = amURL

		inhibitor.Stop()
		disp.Stop()

		inhibitor = inhibit.NewInhibitor(alerts, conf.InhibitRules, marker, logger)
		silencer = silence.NewSilencer(silences, marker, logger)
		pipeline = notify.BuildPipeline(
			conf.Receivers,
			tmpl,
			waitFunc,
			inhibitor,
			silencer,
			notificationLog,
			peer,
			logger,
		)

		api.Update(conf, func(labels model.LabelSet) {
			inhibitor.Mutes(labels)
			silencer.Mutes(labels)
		})

		disp = dispatch.NewDispatcher(alerts, dispatch.NewRoute(conf.Route, nil), pipeline, marker, timeoutFunc, logger)

		go disp.Run()
		go inhibitor.Run()

		return nil
	})

	if err := configCoordinator.Reload(); err != nil {
		return 1
	}

	// Make routePrefix default to externalURL path if empty string.
	if routePrefix == nil || *routePrefix == "" {
		*routePrefix = amURL.Path
	}

	*routePrefix = "/" + strings.Trim(*routePrefix, "/")

	router := route.New().WithInstrumentation(instrumentHandler)

	if *routePrefix != "/" {
		router = router.WithPrefix(*routePrefix)
	}

	webReload := make(chan chan error)

	ui.Register(router, webReload, logger)

	mux := api.Register(router, *routePrefix)

	srv := http.Server{Addr: *listenAddress, Handler: mux}
	srvc := make(chan struct{})

	go func() {
		level.Info(logger).Log("msg", "Listening", "address", *listenAddress)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
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
		hup      = make(chan os.Signal, 1)
		hupReady = make(chan bool)
		term     = make(chan os.Signal, 1)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-hupReady
		for {
			select {
			case <-hup:
				// ignore error, already logged in `reload()`
				_ = configCoordinator.Reload()
			case errc := <-webReload:
				errc <- configCoordinator.Reload()
			}
		}
	}()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	for {
		select {
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

func extURL(listen, external string) (*url.URL, error) {
	if external == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listen)
		if err != nil {
			return nil, err
		}

		external = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	u, err := url.Parse(external)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(u.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	u.Path = ppref

	return u, nil
}
