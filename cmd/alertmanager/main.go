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
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/internal/app"
	"github.com/prometheus/alertmanager/matcher/compat"
)

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
		silenceLogging              = kingpin.Flag("log.silences", "Enable logging silences. If it is enabled, the status change of silence will be logged").Bool()
		alertGCInterval             = kingpin.Flag("alerts.gc-interval", "Interval between alert GC.").Default("30m").Duration()
		perAlertNameLimit           = kingpin.Flag("alerts.per-alertname-limit", "Maximum number of alerts per alertname. If negative or zero, no limit is set.").Default("0").Int()
		dispatchMaintenanceInterval = kingpin.Flag("dispatch.maintenance-interval", "Interval between maintenance of aggregation groups in the dispatcher.").Default("30s").Duration()
		dispatchStartDelay          = kingpin.Flag("dispatch.start-delay", "Minimum amount of time to wait before dispatching alerts. This option should be synced with value of --rules.alert.resend-delay on Prometheus.").Default("0s").Duration()

		webConfig      = webflag.AddFlags(kingpin.CommandLine, ":9093")
		externalURL    = kingpin.Flag("web.external-url", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.").String()
		routePrefix    = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").String()
		getConcurrency = kingpin.Flag("web.get-concurrency", "Maximum number of GET requests processed concurrently. If negative or zero, the limit is GOMAXPROC or 8, whichever is larger.").Default("0").Int()
		httpTimeout    = kingpin.Flag("web.timeout", "Timeout for HTTP requests. If negative or zero, no timeout is set.").Default("0").Duration()

		memlimitRatio = kingpin.Flag("auto-gomemlimit.ratio", "The ratio of reserved GOMEMLIMIT memory to the detected maximum container or system memory. The value must be greater than 0 and less than or equal to 1.").
				Default("0.9").Float64()

		clusterBindAddr = kingpin.Flag("cluster.listen-address", "Listen address for cluster. Set to empty string to disable HA mode.").
				Default(app.DefaultClusterAddr).String()
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

	promslogConfig := promslog.Config{}
	promslogflag.AddFlags(kingpin.CommandLine, &promslogConfig)
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.Version(version.Print("alertmanager"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logger := promslog.New(&promslogConfig)

	prometheus.MustRegister(versioncollector.NewCollector("alertmanager"))

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

	// Translate OS signals into context cancellation (SIGINT/SIGTERM) and
	// reload events (SIGHUP). The app package no longer touches signals
	// directly so that it can be embedded in tests.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	reload := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-hup:
				select {
				case reload <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	opts := app.Options{
		ConfigFile:                  *configFile,
		DataDir:                     *dataDir,
		Retention:                   *retention,
		MaintenanceInterval:         *maintenanceInterval,
		MaxSilences:                 *maxSilences,
		MaxSilenceSizeBytes:         *maxSilenceSizeBytes,
		SilenceLogging:              *silenceLogging,
		AlertGCInterval:             *alertGCInterval,
		PerAlertNameLimit:           *perAlertNameLimit,
		DispatchMaintenanceInterval: *dispatchMaintenanceInterval,
		DispatchStartDelay:          *dispatchStartDelay,

		WebConfig:      webConfig,
		ExternalURL:    *externalURL,
		RoutePrefix:    *routePrefix,
		GetConcurrency: *getConcurrency,
		HTTPTimeout:    *httpTimeout,

		ClusterBindAddr:        *clusterBindAddr,
		ClusterAdvertiseAddr:   *clusterAdvertiseAddr,
		ClusterPeerName:        *clusterPeerName,
		Peers:                  *peers,
		PeerTimeout:            *peerTimeout,
		PeersResolveTimeout:    *peersResolveTimeout,
		GossipInterval:         *gossipInterval,
		PushPullInterval:       *pushPullInterval,
		TCPTimeout:             *tcpTimeout,
		ProbeTimeout:           *probeTimeout,
		ProbeInterval:          *probeInterval,
		SettleTimeout:          *settleTimeout,
		ReconnectInterval:      *reconnectInterval,
		PeerReconnectTimeout:   *peerReconnectTimeout,
		TLSConfigFile:          *tlsConfigFile,
		AllowInsecureAdvertise: *allowInsecureAdvertise,
		Label:                  *label,

		Logger:     logger,
		Registerer: prometheus.DefaultRegisterer,
		Flagger:    ff,
		Reload:     reload,
	}

	if err := app.Run(ctx, opts); err != nil {
		logger.Error("alertmanager exited with error", "err", err)
		return 1
	}
	logger.Info("Received shutdown signal, exiting gracefully...")
	return 0
}
