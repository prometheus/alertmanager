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

// Package app contains the Alertmanager process logic extracted from
// cmd/alertmanager so that tests and other binaries can embed
// Alertmanager in-process instead of shelling out to a compiled binary.
// See https://github.com/prometheus/alertmanager/issues/406.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/common/version"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/api"
	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/httpserver"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/tracing"
	"github.com/prometheus/alertmanager/ui"
)

// Run starts an Alertmanager instance using opts and blocks until ctx is
// cancelled or an unrecoverable error occurs. It is a thin wrapper over
// New + Start + serveLoop + Stop intended for callers that don't need
// the richer lifecycle API.
//
// The deferred Stop also ensures cleanup runs on panic, matching the
// implicit panic-safety of the original defer-based implementation.
func Run(ctx context.Context, opts Options) error {
	a, err := New(opts)
	if err != nil {
		return err
	}
	// Stop applies its own shutdownTimeout, so a background context is
	// sufficient here; passing a competing deadline would only confuse
	// which timeout actually governs the drain.
	defer func() { _ = a.Stop(context.Background()) }()
	if err := a.Start(); err != nil {
		return err
	}
	return a.serveLoop(ctx)
}

// setup wires every Alertmanager subsystem and registers their teardown
// hooks on a.cleanups via a.onStop. Stop drains those hooks in LIFO order
// so the shutdown sequence matches the implicit ordering of the original
// defer-based Run implementation.
//
// The config-scoped subgraph (routes, receivers, pipeline, inhibitor and
// dispatcher) — everything rebuilt on reload — lives in the reloader type
// so its subtle swap ordering is isolated and independently testable.
//
// What remains here is the construction of the long-lived singletons.
// It is deliberately one straight-line function rather than a chain of
// helpers: nearly every step depends on locals produced by earlier ones
// (peer, eventRec, silences, alerts, groupMarker, silencer,
// notificationLog, waitFunc, timeoutFunc, ...), so splitting it up would
// only force us to thread a wide state struct between helpers or promote
// those locals to App fields, obscuring the dataflow without simplifying
// anything. The forward dependency order is already enforced by Go's
// variable scoping, and the matching teardown order by the LIFO onStop
// stack drained in Stop.
//
//nolint:gocyclo // intentional, see comment above.
func (a *App) setup() error {
	opts := a.opts
	if err := opts.validate(); err != nil {
		return err
	}

	logger := opts.Logger
	reg := opts.Registerer
	ff := opts.Flagger
	m := newMetrics(reg)

	a.logger = logger

	logger.Info("Starting Alertmanager", "version", version.Info())
	startTime := time.Now()
	logger.Info("Build context", "build_context", version.BuildContext())

	if err := os.MkdirAll(opts.DataDir, 0o777); err != nil {
		return fmt.Errorf("unable to create data directory: %w", err)
	}

	tlsTransportConfig, err := cluster.GetTLSTransportConfig(opts.TLSConfigFile)
	if err != nil {
		return fmt.Errorf("unable to initialize TLS transport configuration for gossip mesh: %w", err)
	}

	var peer *cluster.Peer
	if opts.ClusterBindAddr != "" {
		peer, err = cluster.Create(
			logger.With("component", "cluster"),
			reg,
			opts.ClusterBindAddr,
			opts.ClusterAdvertiseAddr,
			opts.Peers,
			true,
			opts.PushPullInterval,
			opts.GossipInterval,
			opts.TCPTimeout,
			opts.PeersResolveTimeout,
			opts.ProbeTimeout,
			opts.ProbeInterval,
			tlsTransportConfig,
			opts.AllowInsecureAdvertise,
			opts.Label,
			opts.ClusterPeerName,
		)
		if err != nil {
			return fmt.Errorf("unable to initialize gossip mesh: %w", err)
		}
		m.clusterEnabled.Set(1)
	}

	stopc := make(chan struct{})
	var wg sync.WaitGroup

	// Load config once for both event recorder initialization and the
	// first coordinator apply. Subsequent reloads go through
	// configCoordinator.Reload() which reads the file again.
	initialConf, err := config.LoadFile(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("error loading configuration file: %w", err)
	}

	hostname, _ := os.Hostname()
	var eventRec eventrecorder.Recorder
	if ff.EnableEventRecorder() {
		eventRec = eventrecorder.NewRecorderFromConfig(initialConf.EventRecorder, hostname, logger.With("component", "eventrecorder"), reg)
	}
	a.trackClose("event recorder", eventRec)

	recordCtx := eventrecorder.WithEventRecording(context.Background())
	eventRec.RecordEvent(recordCtx, &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertmanagerStartupEvent{
			AlertmanagerStartupEvent: &eventrecorderpb.AlertmanagerStartupEvent{
				Version:      version.Version,
				BuildContext: version.BuildContext(),
			},
		},
	})
	a.onStop("shutdown event", func() error {
		eventRec.RecordEvent(recordCtx, &eventrecorderpb.EventData{
			EventType: &eventrecorderpb.EventData_AlertmanagerShutdownEvent{
				AlertmanagerShutdownEvent: &eventrecorderpb.AlertmanagerShutdownEvent{},
			},
		})
		return nil
	})

	notificationLogOpts := nflog.Options{
		SnapshotFile: filepath.Join(opts.DataDir, "nflog"),
		Retention:    opts.Retention,
		Logger:       logger.With("component", "nflog"),
		Metrics:      reg,
	}
	notificationLog, err := nflog.New(notificationLogOpts)
	if err != nil {
		return fmt.Errorf("error creating notification log: %w", err)
	}
	if peer != nil {
		c := peer.AddState("nfl", notificationLog, reg)
		notificationLog.SetBroadcast(c.Broadcast)
	}

	wg.Go(func() {
		notificationLog.Maintenance(opts.MaintenanceInterval, filepath.Join(opts.DataDir, "nflog"), stopc, nil)
	})

	// Register the maintenance teardown as soon as the first maintenance
	// goroutine is running. Registering it later (e.g., after silence
	// setup) would leak the already-started goroutine(s) if an
	// intervening setup step returns an error before the cleanup is
	// recorded. close(stopc) stops every maintenance goroutine and
	// wg.Wait blocks until they have all exited; both the nflog and
	// (subsequently started) silence maintenance goroutines are covered.
	a.onStop("maintenance", func() error {
		close(stopc)
		wg.Wait()
		return nil
	})

	groupMarker := marker.NewGroupMarker()

	silenceOpts := silence.Options{
		SnapshotFile: filepath.Join(opts.DataDir, "silences"),
		Retention:    opts.Retention,
		Limits: silence.Limits{
			MaxSilences:         func() int { return opts.MaxSilences },
			MaxSilenceSizeBytes: func() int { return opts.MaxSilenceSizeBytes },
		},
		Logger:        logger.With("component", "silences"),
		Metrics:       reg,
		Logging:       opts.SilenceLogging,
		EventRecorder: eventRec,
	}
	silences, err := silence.New(silenceOpts)
	if err != nil {
		return fmt.Errorf("error creating silence: %w", err)
	}
	if peer != nil {
		c := peer.AddState("sil", silences, reg)
		silences.SetBroadcast(c.Broadcast)
	}

	// Start providers before the router potentially sends updates.
	wg.Go(func() {
		silences.Maintenance(opts.MaintenanceInterval, filepath.Join(opts.DataDir, "silences"), stopc, nil)
	})

	silencer := silence.NewSilencer(silences, logger, eventRec)

	// Peer state listeners have been registered, now we can join and get the initial state.
	if peer != nil {
		if err := peer.Join(opts.ReconnectInterval, opts.PeerReconnectTimeout); err != nil {
			logger.Warn("unable to join gossip mesh", "err", err)
		}
		settleCtx, settleCancel := context.WithTimeout(context.Background(), opts.SettleTimeout)
		a.onStop("cluster peer leave", func() error {
			settleCancel()
			if err := peer.Leave(10 * time.Second); err != nil {
				return fmt.Errorf("unable to leave gossip mesh: %w", err)
			}
			return nil
		})
		go peer.Settle(settleCtx, opts.GossipInterval*10)
		eventRec.SetClusterPeer(peer)
	}

	alerts, err := mem.NewAlerts(
		context.Background(),
		opts.AlertGCInterval,
		opts.PerAlertNameLimit,
		silencer,
		logger,
		eventRec,
		reg,
		ff,
	)
	if err != nil {
		return fmt.Errorf("error creating memory provider: %w", err)
	}
	a.onStop("alerts", func() error {
		alerts.Close()
		return nil
	})

	// disp and inhibitor are swapped on every config reload; they are
	// owned and torn down by the reloader (see below), but groupFn and
	// the API mutes closure need to read the current dispatcher/inhibitor
	// directly, so the pointers live here and are shared with it.
	var (
		disp      atomic.Pointer[dispatch.Dispatcher]
		inhibitor atomic.Pointer[inhibit.Inhibitor]
	)

	groupFn := func(ctx context.Context, routeFilter func(*dispatch.Route) bool, alertFilter func(*alert.Alert, time.Time) bool) (dispatch.AlertGroups, map[model.Fingerprint][]string, error) {
		return disp.Load().Groups(ctx, routeFilter, alertFilter)
	}

	// An interface value that holds a nil concrete value is non-nil.
	// Therefore we explicitly pass an empty interface, to detect if the
	// cluster is not enabled in notify.
	var clusterPeer cluster.ClusterPeer
	if peer != nil {
		clusterPeer = peer
	}

	apih, err := api.New(api.Options{
		Alerts:          alerts,
		Silences:        silences,
		GroupMutedFunc:  groupMarker.Muted,
		Peer:            clusterPeer,
		Timeout:         opts.HTTPTimeout,
		Concurrency:     opts.GetConcurrency,
		Logger:          logger.With("component", "api"),
		Registry:        reg,
		RequestDuration: m.requestDuration,
		GroupFunc:       groupFn,
	})
	if err != nil {
		return fmt.Errorf("failed to create API: %w", err)
	}

	// Bind listeners up front so that Addr/Addrs report concrete bound
	// addresses before Start runs and kernel-assigned ":0" ports can be
	// discovered by callers. Doing this here (rather than at the end of
	// setup) also lets us derive the external URL from the real bound
	// address instead of the requested one, which would otherwise carry a
	// ":0" port for callers that bind ephemeral ports.
	listeners, err := listenAll(opts.WebConfig)
	if err != nil {
		return err
	}
	a.listeners = listeners
	// Close listeners if setup fails after this point. On a successful
	// run server.Shutdown closes them first, so this is then a harmless
	// no-op (Close on an already-closed listener just returns an error we
	// ignore).
	a.onStop("listeners", func() error {
		for _, l := range a.listeners {
			_ = l.Close()
		}
		return nil
	})

	amURL, err := extURL(logger, os.Hostname, listeners[0].Addr().String(), opts.ExternalURL)
	if err != nil {
		return fmt.Errorf("failed to determine external URL: %w", err)
	}
	logger.Debug("app setup", "external_url", amURL.String())

	waitFunc := func() time.Duration { return 0 }
	if peer != nil {
		waitFunc = clusterWait(peer, opts.PeerTimeout)
	}
	timeoutFunc := func(d time.Duration) time.Duration {
		if d < notify.MinTimeout {
			d = notify.MinTimeout
		}
		return d + waitFunc()
	}

	tracingManager := tracing.NewManager(logger.With("component", "tracing"))
	a.tracingMgr = tracingManager
	a.onStop("tracing", func() error {
		tracingManager.Stop()
		return nil
	})

	configLogger := logger.With("component", "configuration")
	configCoordinator := config.NewCoordinator(
		opts.ConfigFile,
		reg,
		configLogger,
	)
	a.coordinator = configCoordinator

	// The reloader owns the config-scoped subgraph (templates, routes,
	// receivers, pipeline, inhibitor, dispatcher). It rebuilds and swaps
	// these on every config apply and stops the live inhibitor+dispatcher
	// at shutdown. The long-lived singletons above are updated in place
	// (apih.Update, eventRec/tracing ApplyConfig) rather than rebuilt.
	r := &reloader{
		logger:                      logger,
		configLogger:                configLogger,
		alerts:                      alerts,
		silencer:                    silencer,
		groupMarker:                 groupMarker,
		notificationLog:             notificationLog,
		eventRec:                    eventRec,
		apih:                        apih,
		tracingMgr:                  tracingManager,
		pipelineBuilder:             notify.NewPipelineBuilder(reg, ff, eventRec),
		dispMetrics:                 dispatch.NewDispatcherMetrics(false, reg, ff),
		metrics:                     m,
		peer:                        peer,
		waitFunc:                    waitFunc,
		timeoutFunc:                 timeoutFunc,
		externalURL:                 amURL,
		startTime:                   startTime,
		dispatchStartDelay:          opts.DispatchStartDelay,
		dispatchMaintenanceInterval: opts.DispatchMaintenanceInterval,
		retention:                   opts.Retention,
		disp:                        &disp,
		inhibitor:                   &inhibitor,
	}
	a.onStop("dispatcher+inhibitor", r.stop)

	configCoordinator.Subscribe(r.reload)

	if err := configCoordinator.ApplyConfig(initialConf); err != nil {
		return fmt.Errorf("failed to apply initial configuration: %w", err)
	}

	// Run the tracing manager exactly once. Manager.Run blocks until the
	// manager is stopped and only (re)installs the global propagator and
	// error handler; ApplyConfig (invoked on every reload above) already
	// swaps the tracer provider in place. Starting it per-reload would
	// leak a goroutine on each reload.
	go tracingManager.Run()

	// Default routePrefix to externalURL path if empty.
	routePrefix := opts.RoutePrefix
	if routePrefix == "" {
		routePrefix = amURL.Path
	}
	routePrefix = "/" + strings.Trim(routePrefix, "/")
	logger.Debug("app setup", "route_prefix", routePrefix)

	router := route.New().WithInstrumentation(m.instrumentHandler)
	if routePrefix != "/" {
		prefix := routePrefix
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, prefix, http.StatusFound)
		})
		router = router.WithPrefix(routePrefix)
	}

	ui.Register(router)
	httpserver.Register(router, a.webReload)

	mux := apih.Register(router, routePrefix)

	a.server = &http.Server{
		// Instrument all handlers with tracing.
		Handler: tracing.Middleware(mux),
	}

	return nil
}
