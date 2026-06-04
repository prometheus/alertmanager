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
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/exporter-toolkit/web"

	"github.com/prometheus/alertmanager/featurecontrol"
)

// DefaultClusterAddr is the default listen address used when the operator
// does not pass --cluster.listen-address.
const DefaultClusterAddr = "0.0.0.0:9094"

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

func (o *Options) validate() error {
	if o.Logger == nil {
		return errors.New("alertmanager/app: Options.Logger is required")
	}
	if o.Registerer == nil {
		return errors.New("alertmanager/app: Options.Registerer is required")
	}
	if o.Flagger == nil {
		return errors.New("alertmanager/app: Options.Flagger is required")
	}
	if o.WebConfig == nil || o.WebConfig.WebListenAddresses == nil || len(*o.WebConfig.WebListenAddresses) == 0 {
		return errors.New("alertmanager/app: Options.WebConfig must contain at least one listen address")
	}
	// exporter-toolkit/web dereferences WebConfigFile unconditionally when
	// serving. The cmd/alertmanager binary always populates it via kingpin,
	// but a programmatic embedder might not, which would otherwise surface
	// as a nil-pointer panic deep inside the toolkit rather than a clear
	// validation error here.
	if o.WebConfig.WebConfigFile == nil {
		return errors.New("alertmanager/app: Options.WebConfig.WebConfigFile must be set (use a pointer to an empty string to disable web TLS/auth config)")
	}
	return nil
}
