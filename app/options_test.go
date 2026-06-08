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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
)

func TestOptions_Validate(t *testing.T) {
	logger := promslog.NewNopLogger()
	reg := prometheus.NewRegistry()
	ff, err := featurecontrol.NewFlags(logger, "")
	require.NoError(t, err)

	addrs := []string{"127.0.0.1:0"}
	emptyAddrs := []string{}
	cfgFile := ""
	systemdOn := true

	// valid returns Options that pass validation: DefaultOptions seeds
	// every interval with a sane positive value, and we add the required
	// dependencies, config path and a listen address. Each subtest
	// mutates one field to exercise a specific branch.
	valid := func() Options {
		o := DefaultOptions()
		o.ConfigFile = "alertmanager.yml"
		o.Logger = logger
		o.Registerer = reg
		o.Flagger = ff
		o.WebConfig = &web.FlagConfig{
			WebListenAddresses: &addrs,
			WebConfigFile:      &cfgFile,
		}
		return o
	}

	base := valid()
	require.NoError(t, base.validate())

	for _, tc := range []struct {
		name   string
		mutate func(*Options)
	}{
		{name: "missing logger", mutate: func(o *Options) { o.Logger = nil }},
		{name: "missing registerer", mutate: func(o *Options) { o.Registerer = nil }},
		{name: "missing flagger", mutate: func(o *Options) { o.Flagger = nil }},
		{name: "missing config file", mutate: func(o *Options) { o.ConfigFile = "" }},
		{name: "missing data dir", mutate: func(o *Options) { o.DataDir = "" }},
		{name: "zero retention", mutate: func(o *Options) { o.Retention = 0 }},
		{name: "zero maintenance interval", mutate: func(o *Options) { o.MaintenanceInterval = 0 }},
		{name: "zero alert gc interval", mutate: func(o *Options) { o.AlertGCInterval = 0 }},
		{name: "zero dispatch maintenance interval", mutate: func(o *Options) { o.DispatchMaintenanceInterval = 0 }},
		{name: "negative retention", mutate: func(o *Options) { o.Retention = -time.Second }},
		{name: "missing web config", mutate: func(o *Options) { o.WebConfig = nil }},
		{name: "nil listen addresses", mutate: func(o *Options) { o.WebConfig.WebListenAddresses = nil }},
		{name: "empty listen addresses", mutate: func(o *Options) { o.WebConfig.WebListenAddresses = &emptyAddrs }},
		{name: "nil web config file", mutate: func(o *Options) { o.WebConfig.WebConfigFile = nil }},
		{name: "cluster enabled zero gossip interval", mutate: func(o *Options) {
			o.ClusterBindAddr = DefaultClusterAddr
			o.GossipInterval = 0
		}},
		{name: "cluster enabled zero probe timeout", mutate: func(o *Options) {
			o.ClusterBindAddr = DefaultClusterAddr
			o.ProbeTimeout = 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := valid()
			// Copy the WebConfig so per-subtest mutations don't leak
			// into the shared addrs/cfgFile pointers.
			wc := *o.WebConfig
			o.WebConfig = &wc
			tc.mutate(&o)
			require.Error(t, o.validate())
		})
	}

	t.Run("systemd socket without listen addresses is valid", func(t *testing.T) {
		o := valid()
		o.WebConfig = &web.FlagConfig{
			WebSystemdSocket: &systemdOn,
			WebConfigFile:    &cfgFile,
			// No WebListenAddresses: systemd provides the listeners.
		}
		require.NoError(t, o.validate())
	})

	t.Run("cluster enabled with zero settle timeout is valid", func(t *testing.T) {
		// SettleTimeout is a context deadline, so 0 ("settle now") is a
		// valid request; the acceptance tests pass --cluster.settle-timeout=0s.
		o := valid()
		o.ClusterBindAddr = DefaultClusterAddr
		o.SettleTimeout = 0
		require.NoError(t, o.validate())
	})

	t.Run("cluster enabled with defaults is valid", func(t *testing.T) {
		o := valid()
		o.ClusterBindAddr = DefaultClusterAddr
		require.NoError(t, o.validate())
	})
}
