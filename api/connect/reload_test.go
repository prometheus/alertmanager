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

package apiconnect

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/config"
	configcommon "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestReloadSnapshots(t *testing.T) {
	t.Parallel()

	t.Run("publishes an immutable configuration, route, receiver, and callback view", func(t *testing.T) {
		t.Parallel()

		matcher, err := labels.NewMatcher(labels.MatchEqual, "service", "api")
		require.NoError(t, err)
		cfg := &config.Config{
			Route: &config.Route{
				Receiver: "primary",
				Matchers: configcommon.Matchers{matcher},
				Labels:   model.LabelSet{"owner": "platform"},
			},
			Receivers: []config.Receiver{
				{Name: "primary", Labels: map[string]string{"name": "primary", "team": "platform"}},
				{Name: "secondary", Labels: map[string]string{"name": "secondary"}},
			},
		}
		var callbackCalls atomic.Int64
		api := NewAPI(Options{})
		api.Update(cfg, func(context.Context, labelset.LabelSet) { callbackCalls.Add(1) })

		snapshot, err := api.currentReloadSnapshot()
		require.NoError(t, err)
		require.NotEmpty(t, snapshot.configuration)
		require.Equal(t, "primary", snapshot.routes.RouteOpts.Receiver)
		require.Equal(t, "api", snapshot.routes.Matchers[0].Value)
		require.Equal(t, model.LabelValue("platform"), snapshot.routes.RouteOpts.Labels["owner"])
		require.Equal(t, []receiverMetadata{
			{name: "primary", labels: map[string]string{"name": "primary", "team": "platform"}},
			{name: "secondary", labels: map[string]string{"name": "secondary"}},
		}, snapshot.receivers)

		cfg.Route.Receiver = "changed"
		cfg.Route.Matchers[0].Value = "changed"
		cfg.Route.Labels["owner"] = "changed"
		cfg.Receivers[0].Labels["team"] = "changed"
		require.Equal(t, "primary", snapshot.routes.RouteOpts.Receiver)
		require.Equal(t, "api", snapshot.routes.Matchers[0].Value)
		require.Equal(t, model.LabelValue("platform"), snapshot.routes.RouteOpts.Labels["owner"])
		require.Equal(t, "platform", snapshot.receivers[0].labels["team"])

		snapshot.setAlertStatus(t.Context(), labelset.LabelSet{})
		require.Equal(t, int64(1), callbackCalls.Load())
	})

	t.Run("returns Unavailable before configuration is loaded", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{})
		_, err := api.currentReloadSnapshot()
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(translateRPCError(t.Context(), err)))

		api.Update(&config.Config{}, nil)
		_, err = api.currentReloadSnapshot()
		require.NoError(t, err)
		api.Update(nil, nil)
		_, err = api.currentReloadSnapshot()
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(translateRPCError(t.Context(), err)))
	})

	t.Run("atomically swaps complete snapshots", func(t *testing.T) {
		t.Parallel()

		api := NewAPI(Options{})
		makeConfig := func(value string) *config.Config {
			return &config.Config{
				Route:     &config.Route{Receiver: value},
				Receivers: []config.Receiver{{Name: value, Labels: map[string]string{"version": value}}},
			}
		}
		api.Update(makeConfig("a"), nil)

		var waitGroup sync.WaitGroup
		var inconsistent atomic.Bool
		start := make(chan struct{})
		waitGroup.Add(5)
		for range 4 {
			go func() {
				defer waitGroup.Done()
				<-start
				for range 1_000 {
					snapshot := api.reloadSnapshot.Load()
					name := snapshot.receivers[0].name
					if snapshot.routes.RouteOpts.Receiver != name || snapshot.receivers[0].labels["version"] != name {
						inconsistent.Store(true)
					}
				}
			}()
		}
		go func() {
			defer waitGroup.Done()
			<-start
			for index := range 1_000 {
				if index%2 == 0 {
					api.Update(makeConfig("a"), nil)
				} else {
					api.Update(makeConfig("b"), nil)
				}
			}
		}()
		close(start)
		waitGroup.Wait()
		require.False(t, inconsistent.Load())
	})
}

func TestReloadSnapshotsOverHTTP(t *testing.T) {
	t.Parallel()

	transports := []struct {
		name string
		opts []connect.ClientOption
	}{
		{name: "Connect"},
		{name: "gRPC-Web", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
		{name: "gRPC", opts: []connect.ClientOption{connect.WithGRPC()}},
	}
	for _, transport := range transports {
		t.Run(transport.name, func(t *testing.T) {
			t.Parallel()

			api := NewAPI(Options{})
			srv := newTestServer(t, api.Handler(), true)
			client := statusv3alphaconnect.NewStatusServiceClient(newH2CClient(t, 5*time.Second), srv.URL, transport.opts...)
			var previous *reloadSnapshot
			for _, name := range []string{"primary", "secondary"} {
				cfg := &config.Config{
					Route:     &config.Route{Receiver: name},
					Receivers: []config.Receiver{{Name: name, Labels: map[string]string{"name": name}}},
				}
				api.Update(cfg, nil)
				response, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
				require.NoError(t, err)
				require.Equal(t, cfg.String(), response.Msg.GetStatus().GetConfig().GetOriginal())
				snapshot, err := api.currentReloadSnapshot()
				require.NoError(t, err)
				require.Equal(t, name, snapshot.routes.RouteOpts.Receiver)
				require.Len(t, snapshot.receivers, 1)
				require.Equal(t, name, snapshot.receivers[0].name)
				require.Equal(t, name, snapshot.receivers[0].labels["name"])
				if previous != nil {
					require.NotSame(t, previous, snapshot)
					require.Equal(t, "primary", previous.routes.RouteOpts.Receiver)
					require.Equal(t, "primary", previous.receivers[0].name)
				}
				previous = snapshot
			}
		})
	}
}
