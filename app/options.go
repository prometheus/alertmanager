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

package app

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/exporter-toolkit/web"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/featurecontrol"
)

// DefaultClusterAddr is the default listen address used when the operator
// does not pass --cluster.listen-address.
const DefaultClusterAddr = "0.0.0.0:9094"

// Default storage and lifecycle values, mirroring the kingpin flag
// defaults in cmd/alertmanager/main.go so embedders that start from
// DefaultOptions behave like the binary.
const (
	DefaultConfigFile                  = "alertmanager.yml"
	DefaultDataDir                     = "data/"
	DefaultRetention                   = 120 * time.Hour
	DefaultMaintenanceInterval         = 15 * time.Minute
	DefaultAlertGCInterval             = 30 * time.Minute
	DefaultDispatchMaintenanceInterval = 30 * time.Second
)

// Options carries the resolved configuration for a single Alertmanager
// instance. Field names follow the kingpin flags in cmd/alertmanager/main.go
// so that mapping between the two is straightforward.
//
// Logger, Registerer and Flagger are required dependencies; the remaining
// fields default to their zero value (which generally matches the kingpin
// flag default).
type Options struct {
	// Storage and lifecycle.
	ConfigFile                  string
	DataDir                     string
	Retention                   time.Duration
	MaintenanceInterval         time.Duration
	MaxSilences                 int
	MaxSilenceSizeBytes         int
	SilenceLogging              bool
	AlertGCInterval             time.Duration
	PerAlertNameLimit           int
	DispatchMaintenanceInterval time.Duration
	DispatchStartDelay          time.Duration

	// Web server.
	WebConfig      *web.FlagConfig
	ExternalURL    string
	RoutePrefix    string
	GetConcurrency int
	HTTPTimeout    time.Duration

	// Cluster.
	ClusterBindAddr        string
	ClusterAdvertiseAddr   string
	ClusterPeerName        string
	Peers                  []string
	PeerTimeout            time.Duration
	PeersResolveTimeout    time.Duration
	GossipInterval         time.Duration
	PushPullInterval       time.Duration
	TCPTimeout             time.Duration
	ProbeTimeout           time.Duration
	ProbeInterval          time.Duration
	SettleTimeout          time.Duration
	ReconnectInterval      time.Duration
	PeerReconnectTimeout   time.Duration
	TLSConfigFile          string
	AllowInsecureAdvertise bool
	Label                  string

	// Injected dependencies.
	Logger     *slog.Logger
	Registerer prometheus.Registerer
	Flagger    featurecontrol.Flagger

	// Reload triggers a configuration reload each time it receives a
	// value. The binary translates SIGHUP into sends on this channel;
	// callers can also drive reloads programmatically. A nil channel
	// disables external reloads (the /-/reload HTTP endpoint still works).
	Reload <-chan struct{}
}

// DefaultOptions returns an Options value pre-populated with the same
// defaults as the cmd/alertmanager kingpin flags. Clustering is disabled
// (ClusterBindAddr empty) because enabling a gossip listener by default
// would surprise embedders; the cluster timeouts are still seeded so that
// setting ClusterBindAddr is all that's needed to enable HA.
//
// Callers must still supply the required dependencies (Logger, Registerer,
// Flagger) and a WebConfig before passing the result to New or Run.
func DefaultOptions() Options {
	return Options{
		ConfigFile:                  DefaultConfigFile,
		DataDir:                     DefaultDataDir,
		Retention:                   DefaultRetention,
		MaintenanceInterval:         DefaultMaintenanceInterval,
		AlertGCInterval:             DefaultAlertGCInterval,
		DispatchMaintenanceInterval: DefaultDispatchMaintenanceInterval,

		PeerTimeout:          15 * time.Second,
		PeersResolveTimeout:  cluster.DefaultResolvePeersTimeout,
		GossipInterval:       cluster.DefaultGossipInterval,
		PushPullInterval:     cluster.DefaultPushPullInterval,
		TCPTimeout:           cluster.DefaultTCPTimeout,
		ProbeTimeout:         cluster.DefaultProbeTimeout,
		ProbeInterval:        cluster.DefaultProbeInterval,
		SettleTimeout:        cluster.DefaultPushPullInterval,
		ReconnectInterval:    cluster.DefaultReconnectInterval,
		PeerReconnectTimeout: cluster.DefaultReconnectTimeout,
	}
}

// usingSystemdSocket reports whether the web server is configured to take
// its listener(s) from systemd socket activation, in which case explicit
// listen addresses are not required.
func (o *Options) usingSystemdSocket() bool {
	return o.WebConfig != nil &&
		o.WebConfig.WebSystemdSocket != nil &&
		*o.WebConfig.WebSystemdSocket
}

// validate checks that the Options are internally consistent and that no
// field carries a zero value that would later panic (e.g. a zero interval
// handed to time.NewTicker) or silently misbehave. It is intended to turn
// embedder misconfiguration into a clear error at New time rather than an
// obscure failure deep inside a subsystem.
func (o *Options) validate() error {
	// Required injected dependencies.
	if o.Logger == nil {
		return errors.New("alertmanager/app: Options.Logger is required")
	}
	if o.Registerer == nil {
		return errors.New("alertmanager/app: Options.Registerer is required")
	}
	if o.Flagger == nil {
		return errors.New("alertmanager/app: Options.Flagger is required")
	}

	// Storage and config paths.
	if o.ConfigFile == "" {
		return errors.New("alertmanager/app: Options.ConfigFile is required")
	}
	if o.DataDir == "" {
		return errors.New("alertmanager/app: Options.DataDir is required")
	}

	// Intervals that drive time.NewTicker panic on non-positive values,
	// so reject them up front with a clear message.
	for _, f := range []struct {
		name string
		val  time.Duration
	}{
		{"Retention", o.Retention},
		{"MaintenanceInterval", o.MaintenanceInterval},
		{"AlertGCInterval", o.AlertGCInterval},
		{"DispatchMaintenanceInterval", o.DispatchMaintenanceInterval},
	} {
		if f.val <= 0 {
			return fmt.Errorf("alertmanager/app: Options.%s must be positive", f.name)
		}
	}

	// Web server.
	if o.WebConfig == nil {
		return errors.New("alertmanager/app: Options.WebConfig is required")
	}
	// With systemd socket activation the listeners come from the
	// activation file descriptors, so explicit listen addresses are
	// optional; otherwise at least one is required.
	if !o.usingSystemdSocket() &&
		(o.WebConfig.WebListenAddresses == nil || len(*o.WebConfig.WebListenAddresses) == 0) {
		return errors.New("alertmanager/app: Options.WebConfig must contain at least one listen address (or enable WebSystemdSocket)")
	}
	// exporter-toolkit/web dereferences WebConfigFile unconditionally when
	// serving. The cmd/alertmanager binary always populates it via kingpin,
	// but a programmatic embedder might not, which would otherwise surface
	// as a nil-pointer panic deep inside the toolkit rather than a clear
	// validation error here.
	if o.WebConfig.WebConfigFile == nil {
		return errors.New("alertmanager/app: Options.WebConfig.WebConfigFile must be set (use a pointer to an empty string to disable web TLS/auth config)")
	}

	// Cluster timeouts only matter when HA is enabled. When it is, the
	// intervals that feed memberlist tickers must be positive. Note that
	// SettleTimeout is intentionally excluded: it is used as a
	// context.WithTimeout deadline, so a zero (or negative) value is a
	// valid request to settle immediately without waiting — the
	// acceptance tests rely on --cluster.settle-timeout=0s.
	if o.ClusterBindAddr != "" {
		for _, f := range []struct {
			name string
			val  time.Duration
		}{
			{"PeerTimeout", o.PeerTimeout},
			{"PeersResolveTimeout", o.PeersResolveTimeout},
			{"GossipInterval", o.GossipInterval},
			{"PushPullInterval", o.PushPullInterval},
			{"TCPTimeout", o.TCPTimeout},
			{"ProbeTimeout", o.ProbeTimeout},
			{"ProbeInterval", o.ProbeInterval},
			{"ReconnectInterval", o.ReconnectInterval},
			{"PeerReconnectTimeout", o.PeerReconnectTimeout},
		} {
			if f.val <= 0 {
				return fmt.Errorf("alertmanager/app: Options.%s must be positive when clustering is enabled", f.name)
			}
		}
	}

	return nil
}
