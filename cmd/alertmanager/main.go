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
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/alertmanager/api"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/weaveworks/mesh"
)

var (
	configHash = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_config_hash",
		Help: "Hash of the currently loaded alertmanager configuration.",
	})
	configSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_config_last_reload_successful",
		Help: "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_config_last_reload_success_timestamp_seconds",
		Help: "Timestamp of the last successful configuration reload.",
	})
	alertsActive     prometheus.GaugeFunc
	alertsSuppressed prometheus.GaugeFunc
)

func init() {
	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)
	prometheus.MustRegister(configHash)
	prometheus.MustRegister(version.NewCollector("alertmanager"))
}

func newAlertMetricByState(marker types.Marker, st types.AlertState) prometheus.GaugeFunc {
	return prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "alertmanager_alerts",
			Help:        "How many alerts by state.",
			ConstLabels: prometheus.Labels{"state": string(st)},
		},
		func() float64 {
			return float64(marker.Count(st))
		},
	)
}

func newMarkerMetrics(marker types.Marker) {
	alertsActive = newAlertMetricByState(marker, types.AlertStateActive)
	alertsSuppressed = newAlertMetricByState(marker, types.AlertStateSuppressed)

	prometheus.MustRegister(alertsActive)
	prometheus.MustRegister(alertsSuppressed)
}

func main() {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	logLevel := &promlog.AllowedLevel{}
	if err := logLevel.Set("info"); err != nil {
		panic(err)
	}
	var (
		configFile      = kingpin.Flag("config.file", "Alertmanager configuration file name.").Default("alertmanager.yml").String()
		dataDir         = kingpin.Flag("storage.path", "Base path for data storage.").Default("data/").String()
		retention       = kingpin.Flag("data.retention", "How long to keep data for.").Default("120h").Duration()
		alertGCInterval = kingpin.Flag("alerts.gc-interval", "Interval between alert GC.").Default("30m").Duration()
		logLevelString  = kingpin.Flag("log.level", "Only log messages with the given severity or above.").Default("info").Enum("debug", "info", "warn", "error")

		externalURL   = kingpin.Flag("web.external-url", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.").String()
		routePrefix   = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").String()
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for the web interface and API.").Default(":9093").String()

		meshListen = kingpin.Flag("mesh.listen-address", "mesh listen address. Pass an empty string to disable.").Default(net.JoinHostPort("0.0.0.0", strconv.Itoa(mesh.Port))).String()
		hwaddr     = kingpin.Flag("mesh.peer-id", "mesh peer ID").Default(mustHardwareAddr()).String()
		nickname   = kingpin.Flag("mesh.nickname", "mesh peer nickname").Default(mustHostname()).String()
		password   = kingpin.Flag("mesh.password", "password to join the peer network (empty password disables encryption)").Default("").String()
		peers      = kingpin.Flag("mesh.peer", "initial peers (may be repeated)").Strings()
	)

	kingpin.Version(version.Print("alertmanager"))
	kingpin.CommandLine.GetFlag("help").Short('h')
	kingpin.Parse()

	logLevel.Set(*logLevelString)
	logger := promlog.New(*logLevel)

	level.Info(logger).Log("msg", "Starting Alertmanager", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	err := os.MkdirAll(*dataDir, 0777)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to create data directory", "err", err)
		os.Exit(1)
	}

	var mrouter *mesh.Router
	if *meshListen != "" {
		mrouter, err = initMesh(*meshListen, *hwaddr, *nickname, *password, log.With(logger, "component", "mesh"))
		if err != nil {
			level.Error(logger).Log("msg", "Unable to initialize gossip mesh", "err", err)
			os.Exit(1)
		}
		prometheus.MustRegister(NewMeshCollector(mrouter))
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
	if *meshListen != "" {
		notificationLogOpts = append(notificationLogOpts, nflog.WithMesh(func(g mesh.Gossiper) mesh.Gossip {
			res, err := mrouter.NewGossip("nflog", g)
			if err != nil {
				level.Error(logger).Log("err", err)
				os.Exit(1)
			}
			return res
		}))
	}
	notificationLog, err := nflog.New(notificationLogOpts...)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}

	marker := types.NewMarker()
	newMarkerMetrics(marker)

	silenceOpts := silence.Options{
		SnapshotFile: filepath.Join(*dataDir, "silences"),
		Retention:    *retention,
		Logger:       log.With(logger, "component", "silences"),
		Metrics:      prometheus.DefaultRegisterer,
	}
	if *meshListen != "" {
		silenceOpts.Gossip = func(g mesh.Gossiper) mesh.Gossip {
			res, err := mrouter.NewGossip("silences", g)
			if err != nil {
				level.Error(logger).Log("err", err)
				os.Exit(1)
			}
			return res
		}
	}
	silences, err := silence.New(silenceOpts)

	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}

	// Start providers before router potentially sends updates.
	wg.Add(1)
	go func() {
		silences.Maintenance(15*time.Minute, filepath.Join(*dataDir, "silences"), stopc)
		wg.Done()
	}()

	// Disable mesh if empty string passed for mesh.listen-address flag.
	if *meshListen != "" {
		mrouter.Start()
		mrouter.ConnectionMaker.InitiateConnections(*peers, true)
	}

	defer func() {
		close(stopc)
		if *meshListen != "" {
			// Stop receiving updates from router before shutting down.
			mrouter.Stop()
		}
		wg.Wait()
	}()

	alerts, err := mem.NewAlerts(marker, *alertGCInterval)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	defer alerts.Close()

	var (
		inhibitor *inhibit.Inhibitor
		tmpl      *template.Template
		pipeline  notify.Stage
		disp      *dispatch.Dispatcher
	)
	defer disp.Stop()

	apiv := api.New(
		alerts,
		silences,
		func(matchers []*labels.Matcher) dispatch.AlertOverview {
			return disp.Groups(matchers)
		},
		marker.Status,
		mrouter,
		logger,
	)

	amURL, err := extURL(*listenAddress, *externalURL)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}

	waitFunc := func() time.Duration { return 0 }
	if *meshListen != "" {
		waitFunc = meshWait(mrouter, 5*time.Second)
	}
	timeoutFunc := func(d time.Duration) time.Duration {
		if d < notify.MinTimeout {
			d = notify.MinTimeout
		}
		return d + waitFunc()
	}

	var hash float64
	reload := func() (err error) {
		level.Info(logger).Log("msg", "Loading configuration file", "file", *configFile)
		defer func() {
			if err != nil {
				level.Error(logger).Log("msg", "Loading configuration file failed", "file", *configFile, "err", err)
				configSuccess.Set(0)
			} else {
				configSuccess.Set(1)
				configSuccessTime.Set(float64(time.Now().Unix()))
				configHash.Set(hash)
			}
		}()

		conf, plainCfg, err := config.LoadFile(*configFile)
		if err != nil {
			return err
		}

		hash = md5HashAsMetricValue(plainCfg)

		err = apiv.Update(conf, time.Duration(conf.Global.ResolveTimeout))
		if err != nil {
			return err
		}

		tmpl, err = template.FromGlobs(conf.Templates...)
		if err != nil {
			return err
		}
		tmpl.ExternalURL = amURL

		inhibitor.Stop()
		disp.Stop()

		inhibitor = inhibit.NewInhibitor(alerts, conf.InhibitRules, marker, logger)
		pipeline = notify.BuildPipeline(
			conf.Receivers,
			tmpl,
			waitFunc,
			inhibitor,
			silences,
			notificationLog,
			marker,
			logger,
		)
		disp = dispatch.NewDispatcher(alerts, dispatch.NewRoute(conf.Route, nil), pipeline, marker, timeoutFunc, logger)

		go disp.Run()
		go inhibitor.Run()

		return nil
	}

	if err := reload(); err != nil {
		os.Exit(1)
	}

	// Make routePrefix default to externalURL path if empty string.
	if routePrefix == nil || *routePrefix == "" {
		*routePrefix = amURL.Path
	}

	*routePrefix = "/" + strings.Trim(*routePrefix, "/")

	router := route.New()

	if *routePrefix != "/" {
		router = router.WithPrefix(*routePrefix)
	}

	webReload := make(chan chan error)

	ui.Register(router, webReload, logger)

	apiv.Register(router.WithPrefix("/api"))

	level.Info(logger).Log("msg", "Listening", "address", *listenAddress)
	go listen(*listenAddress, router, logger)

	var (
		hup      = make(chan os.Signal)
		hupReady = make(chan bool)
		term     = make(chan os.Signal)
	)
	signal.Notify(hup, syscall.SIGHUP)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-hupReady
		for {
			select {
			case <-hup:
				reload()
			case errc := <-webReload:
				errc <- reload()
			}
		}
	}()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	<-term

	level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
}

type meshCollector struct {
	router   *mesh.Router
	connDesc *prometheus.Desc
	posDesc  *prometheus.Desc
	termDesc *prometheus.Desc
}

func NewMeshCollector(router *mesh.Router) *meshCollector {
	return &meshCollector{
		router: router,
		connDesc: prometheus.NewDesc(
			"alertmanager_peer_connection",
			"State of the connection between the Alertmanager instance and a peer.",
			[]string{"peer"},
			prometheus.Labels{},
		),
		posDesc: prometheus.NewDesc(
			"alertmanager_peer_position",
			"Position the Alertmanager instance believes it's in. The position determines a peer's behavior in the cluster.",
			[]string{},
			prometheus.Labels{},
		),
		termDesc: prometheus.NewDesc(
			"alertmanager_peer_terminations_total",
			"Total number of terminated connections between the AlertManager and its peers.",
			[]string{},
			prometheus.Labels{},
		),
	}
}

// Describe implements the prometheus.Collector interface
func (c *meshCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connDesc
	ch <- c.posDesc
	ch <- c.termDesc
}

// Collect implements the prometheus.Collector interface
func (c *meshCollector) Collect(ch chan<- prometheus.Metric) {
	status := mesh.NewStatus(c.router)
	for _, peer := range status.Peers {
		// collect only metrics for the local peer
		if status.Name != peer.Name {
			continue
		}
		for _, conn := range peer.Connections {
			var v float64
			if conn.Established {
				v = 1
			}
			ch <- prometheus.MustNewConstMetric(
				c.connDesc,
				prometheus.GaugeValue,
				v,
				conn.Name,
			)
		}
	}
	ch <- prometheus.MustNewConstMetric(
		c.posDesc,
		prometheus.GaugeValue,
		float64(meshPeerPosition(c.router)),
	)
	ch <- prometheus.MustNewConstMetric(
		c.termDesc,
		prometheus.GaugeValue,
		float64(status.TerminationCount),
	)
}

type peerDescSlice []mesh.PeerDescription

func (s peerDescSlice) Len() int           { return len(s) }
func (s peerDescSlice) Less(i, j int) bool { return s[i].UID < s[j].UID }
func (s peerDescSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// meshPeerPosition returns the position of the local peer in the mesh.
func meshPeerPosition(r *mesh.Router) int {
	var peers peerDescSlice
	for _, desc := range r.Peers.Descriptions() {
		peers = append(peers, desc)
	}
	sort.Sort(peers)

	k := 0
	for _, desc := range peers {
		if desc.Self {
			break
		}
		k++
	}
	return k
}

// meshWait returns a function that inspects the current peer state and returns
// a duration of one base timeout for each peer with a higher ID than ourselves.
func meshWait(r *mesh.Router, timeout time.Duration) func() time.Duration {
	return func() time.Duration {
		return time.Duration(meshPeerPosition(r)) * timeout
	}
}

func initMesh(addr, hwaddr, nickname, pw string, logger log.Logger) (*mesh.Router, error) {
	host, portStr, err := net.SplitHostPort(addr)

	if err != nil {
		level.Error(logger).Log("msg", "Invalid mesh address", "address", addr, "err", err)
		os.Exit(1)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		level.Error(logger).Log("msg", "Invalid mesh address", "address", addr, "err", err)
		os.Exit(1)
	}

	name, err := mesh.PeerNameFromString(hwaddr)
	if err != nil {
		level.Error(logger).Log("msg", "Invalid hardware address", "address", hwaddr, "err", err)
		os.Exit(1)
	}

	password := []byte(pw)
	if len(password) == 0 {
		// Emtpy password is used to disable secure communication. Using a nil
		// password disables encryption in mesh.
		password = nil
	}

	return mesh.NewRouter(mesh.Config{
		Host:               host,
		Port:               port,
		ProtocolMinVersion: mesh.ProtocolMinVersion,
		Password:           password,
		ConnLimit:          64,
		PeerDiscovery:      true,
		TrustedSubnets:     []*net.IPNet{},
	}, name, nickname, mesh.NullOverlay{}, printfLogger{logger})
}

type printfLogger struct {
	log.Logger
}

func (l printfLogger) Printf(f string, args ...interface{}) {
	level.Debug(l).Log("msg", fmt.Sprintf(f, args...))
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

func listen(listen string, router *route.Router, logger log.Logger) {
	if err := http.ListenAndServe(listen, router); err != nil {
		level.Error(logger).Log("msg", "Listen error", "err", err)
		os.Exit(1)
	}
}

func mustHardwareAddr() string {
	// TODO(fabxc): consider a safe-guard against colliding MAC addresses.
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, iface := range ifaces {
		if s := iface.HardwareAddr.String(); s != "" {
			return s
		}
	}
	panic("no valid network interfaces")
}

func mustHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}

func md5HashAsMetricValue(data []byte) float64 {
	sum := md5.Sum(data)
	// We only want 48 bits as a float64 only has a 53 bit mantissa.
	smallSum := sum[0:6]
	var bytes = make([]byte, 8)
	copy(bytes, smallSum)
	return float64(binary.LittleEndian.Uint64(bytes))
}
