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
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"
	"github.com/weaveworks/mesh"
)

var (
	configSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "alertmanager",
		Name:      "config_last_reload_successful",
		Help:      "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "alertmanager",
		Name:      "config_last_reload_success_timestamp_seconds",
		Help:      "Timestamp of the last successful configuration reload.",
	})
)

func init() {
	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)
	prometheus.MustRegister(version.NewCollector("alertmanager"))
}

func main() {
	peers := &stringset{}
	var (
		showVersion = flag.Bool("version", false, "Print version information.")

		configFile = flag.String("config.file", "alertmanager.yml", "Alertmanager configuration file name.")
		dataDir    = flag.String("storage.path", "data/", "Base path for data storage.")
		retention  = flag.Duration("data.retention", 5*24*time.Hour, "How long to keep data for.")

		externalURL   = flag.String("web.external-url", "", "The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.")
		listenAddress = flag.String("web.listen-address", ":9093", "Address to listen on for the web interface and API.")

		meshListen = flag.String("mesh.listen-address", net.JoinHostPort("0.0.0.0", strconv.Itoa(mesh.Port)), "mesh listen address")
		hwaddr     = flag.String("mesh.hardware-address", mustHardwareAddr(), "MAC address, i.e. mesh peer ID")
		nickname   = flag.String("mesh.nickname", mustHostname(), "peer nickname")
		password   = flag.String("mesh.password", "", "password to join the peer network (empty password disables encryption)")
	)
	flag.Var(peers, "mesh.peer", "initial peers (may be repeated)")
	flag.Parse()

	if len(flag.Args()) > 0 {
		log.Fatalln("Received unexpected and unparsed arguments: ", strings.Join(flag.Args(), ", "))
	}

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("alertmanager"))
		os.Exit(0)
	}

	log.Infoln("Starting alertmanager", version.Info())
	log.Infoln("Build context", version.BuildContext())

	err := os.MkdirAll(*dataDir, 0777)
	if err != nil {
		log.Fatal(err)
	}

	logger := log.NewLogger(os.Stderr)
	mrouter := initMesh(*meshListen, *hwaddr, *nickname, *password)

	stopc := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	notificationLog, err := nflog.New(
		nflog.WithMesh(func(g mesh.Gossiper) mesh.Gossip {
			return mrouter.NewGossip("nflog", g)
		}),
		nflog.WithRetention(*retention),
		nflog.WithSnapshot(filepath.Join(*dataDir, "nflog")),
		nflog.WithMaintenance(15*time.Minute, stopc, wg.Done),
		nflog.WithLogger(logger.With("component", "nflog")),
	)
	if err != nil {
		log.Fatal(err)
	}

	marker := types.NewMarker()

	silences, err := silence.New(silence.Options{
		SnapshotFile: filepath.Join(*dataDir, "silences"),
		Retention:    *retention,
		Logger:       logger.With("component", "silences"),
		Metrics:      prometheus.DefaultRegisterer,
		Gossip: func(g mesh.Gossiper) mesh.Gossip {
			return mrouter.NewGossip("silences", g)
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Start providers before router potentially sends updates.
	wg.Add(1)
	go func() {
		silences.Maintenance(15*time.Minute, filepath.Join(*dataDir, "silences"), stopc)
		wg.Done()
	}()

	mrouter.Start()

	defer func() {
		close(stopc)
		// Stop receiving updates from router before shutting down.
		mrouter.Stop()
		wg.Wait()
	}()

	mrouter.ConnectionMaker.InitiateConnections(peers.slice(), true)

	alerts, err := mem.NewAlerts(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer alerts.Close()

	var (
		inhibitor *inhibit.Inhibitor
		tmpl      *template.Template
		pipeline  notify.Stage
		disp      *dispatch.Dispatcher
	)
	defer disp.Stop()

	apiv := api.New(alerts, silences, func() dispatch.AlertOverview {
		return disp.Groups()
	})

	amURL, err := extURL(*listenAddress, *externalURL)
	if err != nil {
		log.Fatal(err)
	}

	waitFunc := meshWait(mrouter, 5*time.Second)
	timeoutFunc := func(d time.Duration) time.Duration {
		if d < notify.MinTimeout {
			d = notify.MinTimeout
		}
		return d + waitFunc()
	}

	reload := func() (err error) {
		log.With("file", *configFile).Infof("Loading configuration file")
		defer func() {
			if err != nil {
				log.With("file", *configFile).Errorf("Loading configuration file failed: %s", err)
				configSuccess.Set(0)
			} else {
				configSuccess.Set(1)
				configSuccessTime.Set(float64(time.Now().Unix()))
			}
		}()

		conf, err := config.LoadFile(*configFile)
		if err != nil {
			return err
		}

		apiv.Update(conf.String(), time.Duration(conf.Global.ResolveTimeout))

		tmpl, err = template.FromGlobs(conf.Templates...)
		if err != nil {
			return err
		}
		tmpl.ExternalURL = amURL

		inhibitor.Stop()
		disp.Stop()

		inhibitor = inhibit.NewInhibitor(alerts, conf.InhibitRules, marker)
		pipeline = notify.BuildPipeline(
			conf.Receivers,
			tmpl,
			waitFunc,
			inhibitor,
			silences,
			notificationLog,
			marker,
		)
		disp = dispatch.NewDispatcher(alerts, dispatch.NewRoute(conf.Route, nil), pipeline, marker, timeoutFunc)

		go disp.Run()
		go inhibitor.Run()

		return nil
	}

	if err := reload(); err != nil {
		os.Exit(1)
	}

	router := route.New()

	webReload := make(chan struct{})
	ui.Register(router.WithPrefix(amURL.Path), webReload)
	apiv.Register(router.WithPrefix(path.Join(amURL.Path, "/api")))

	log.Infoln("Listening on", *listenAddress)
	go listen(*listenAddress, router)

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
			case <-webReload:
			}
			reload()
		}
	}()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	<-term

	log.Infoln("Received SIGTERM, exiting gracefully...")
}

type peerDescSlice []mesh.PeerDescription

func (s peerDescSlice) Len() int           { return len(s) }
func (s peerDescSlice) Less(i, j int) bool { return s[i].UID < s[j].UID }
func (s peerDescSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// meshWait returns a function that inspects the current peer state and returns
// a duration of one base timeout for each peer with a higher ID than ourselves.
func meshWait(r *mesh.Router, timeout time.Duration) func() time.Duration {
	return func() time.Duration {
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
		// TODO(fabxc): add metric exposing the "position" from AM's own view.
		return time.Duration(k) * timeout
	}
}

func initMesh(addr, hwaddr, nickname, pw string) *mesh.Router {
	host, portStr, err := net.SplitHostPort(addr)

	if err != nil {
		log.Fatalf("mesh address: %s: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("mesh address: %s: %v", addr, err)
	}

	name, err := mesh.PeerNameFromString(hwaddr)
	if err != nil {
		log.Fatalf("invalid hardware address %q: %v", hwaddr, err)
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
	}, name, nickname, mesh.NullOverlay{}, stdlog.New(ioutil.Discard, "", 0))

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

func listen(listen string, router *route.Router) {
	if err := http.ListenAndServe(listen, router); err != nil {
		log.Fatal(err)
	}
}

type stringset map[string]struct{}

func (ss stringset) Set(value string) error {
	ss[value] = struct{}{}
	return nil
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
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
