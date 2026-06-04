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

	// valid returns a fully populated Options that passes validation;
	// each subtest mutates one field to exercise a specific branch.
	valid := func() Options {
		return Options{
			Logger:     logger,
			Registerer: reg,
			Flagger:    ff,
			WebConfig: &web.FlagConfig{
				WebListenAddresses: &addrs,
				WebConfigFile:      &cfgFile,
			},
		}
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
		{name: "missing web config", mutate: func(o *Options) { o.WebConfig = nil }},
		{name: "nil listen addresses", mutate: func(o *Options) { o.WebConfig.WebListenAddresses = nil }},
		{name: "empty listen addresses", mutate: func(o *Options) { o.WebConfig.WebListenAddresses = &emptyAddrs }},
		{name: "nil web config file", mutate: func(o *Options) { o.WebConfig.WebConfigFile = nil }},
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
}
