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
	"github.com/prometheus/alertmanager/config/receiver"
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
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/timeinterval"
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
// The body is deliberately one long function rather than a chain of helpers.
// Nearly every step depends on locals produced by earlier steps (peer,
// eventRec, silences, alerts, groupMarker, silencer, notificationLog,
// disp, inhibitor, tmpl, configCoordinator, waitFunc, timeoutFunc, ...),
// and the configCoordinator.Subscribe callback closes over most of them.
// Splitting setup into helpers would force us to either thread a wide
// state struct between them or promote those locals to App fields; both
// obscure the dataflow without simplifying anything.
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
	a.onStop(func() { eventRec.Close() })

	recordCtx := eventrecorder.WithEventRecording(context.Background())
	eventRec.RecordEvent(recordCtx, &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertmanagerStartupEvent{
			AlertmanagerStartupEvent: &eventrecorderpb.AlertmanagerStartupEvent{
				Version:      version.Version,
				BuildContext: version.BuildContext(),
			},
		},
	})
	a.onStop(func() {
		eventRec.RecordEvent(recordCtx, &eventrecorderpb.EventData{
			EventType: &eventrecorderpb.EventData_AlertmanagerShutdownEvent{
				AlertmanagerShutdownEvent: &eventrecorderpb.AlertmanagerShutdownEvent{},
			},
		})
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
	a.onStop(func() {
		close(stopc)
		wg.Wait()
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
		a.onStop(func() {
			settleCancel()
			if err := peer.Leave(10 * time.Second); err != nil {
				logger.Warn("unable to leave gossip mesh", "err", err)
			}
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
	a.onStop(alerts.Close)

	var disp atomic.Pointer[dispatch.Dispatcher]
	a.onStop(func() {
		if d := disp.Load(); d != nil {
			d.Stop()
		}
	})

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
	a.onStop(func() {
		for _, l := range a.listeners {
			_ = l.Close()
		}
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
	a.onStop(tracingManager.Stop)

	var inhibitor atomic.Pointer[inhibit.Inhibitor]

	// Stop the current inhibitor at shutdown. The reload callback swaps
	// in a fresh inhibitor on every config apply (stopping the previous
	// one), so without this the most recently installed inhibitor's
	// goroutine would leak when the App is stopped.
	a.onStop(func() {
		if i := inhibitor.Load(); i != nil {
			i.Stop()
		}
	})

	dispMetrics := dispatch.NewDispatcherMetrics(false, reg, ff)
	pipelineBuilder := notify.NewPipelineBuilder(reg, ff, eventRec)
	configLogger := logger.With("component", "configuration")
	configCoordinator := config.NewCoordinator(
		opts.ConfigFile,
		reg,
		configLogger,
	)
	a.coordinator = configCoordinator

	configCoordinator.Subscribe(func(conf *config.Config) error {
		// Reload event recorder outputs first so events emitted during
		// the rest of this callback (e.g., by stopping the old
		// dispatcher) go to the new outputs.
		eventRec.ApplyConfig(conf.EventRecorder)

		tmpl, err := template.FromGlobs(conf.Templates)
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

		if old := inhibitor.Load(); old != nil {
			old.Stop()
		}
		if old := disp.Load(); old != nil {
			old.Stop()
		}

		newInhibitor := inhibit.NewInhibitor(alerts, conf.InhibitRules, logger, eventRec)
		inhibitor.Store(newInhibitor)

		// An interface value that holds a nil concrete value is non-nil.
		// Therefore we explicitly pass an empty interface, to detect if the
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
			groupMarker,
			notificationLog,
			pipelinePeer,
		)

		m.configuredReceivers.Set(float64(len(activeReceivers)))
		m.configuredIntegrations.Set(float64(integrationsNum))
		m.configuredInhibitionRules.Set(float64(len(conf.InhibitRules)))

		apih.Update(conf, func(ctx context.Context, labels model.LabelSet) {
			inhibitor.Load().Mutes(ctx, labels)
			silencer.Mutes(ctx, labels)
		})

		newDisp := dispatch.NewDispatcher(
			alerts,
			routes,
			pipeline,
			groupMarker,
			timeoutFunc,
			opts.DispatchMaintenanceInterval,
			nil,
			logger,
			eventRec,
			dispMetrics,
		)
		routes.Walk(func(r *dispatch.Route) {
			if r.RouteOpts.RepeatInterval > opts.Retention {
				configLogger.Warn(
					"repeat_interval is greater than the data retention period. It can lead to notifications being repeated more often than expected.",
					"repeat_interval", r.RouteOpts.RepeatInterval,
					"retention", opts.Retention,
					"route", r.Key(),
				)
			}
			if r.RouteOpts.RepeatInterval < r.RouteOpts.GroupInterval {
				configLogger.Warn(
					"repeat_interval is less than group_interval. Notifications will not repeat until the next group_interval.",
					"repeat_interval", r.RouteOpts.RepeatInterval,
					"group_interval", r.RouteOpts.GroupInterval,
					"route", r.Key(),
				)
			}
		})

		// First, start the inhibitor so the inhibition cache can populate.
		// Wait for this to load alerts before starting the dispatcher so
		// we don't accidentally notify for an alert that will be inhibited.
		go newInhibitor.Run()
		newInhibitor.WaitForLoading()

		// Next, start the dispatcher and wait for it to load before swapping
		// the disp pointer. This ensures that the API doesn't see the new
		// dispatcher before it finishes populating the aggrGroups.
		go newDisp.Run(startTime.Add(opts.DispatchStartDelay))
		newDisp.WaitForLoading()
		disp.Store(newDisp)

		if err := tracingManager.ApplyConfig(conf.TracingConfig); err != nil {
			return fmt.Errorf("failed to apply tracing config: %w", err)
		}

		return nil
	})

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
