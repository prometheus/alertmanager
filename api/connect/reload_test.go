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

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	configcommon "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var _ = Describe("reload snapshots", func() {
	It("publishes an immutable configuration, route, receiver, and callback view", func() {
		matcher, err := labels.NewMatcher(labels.MatchEqual, "service", "api")
		Expect(err).NotTo(HaveOccurred())
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
		api := newTestAPI(Options{})
		api.Update(cfg, func(context.Context, model.LabelSet) { callbackCalls.Add(1) })

		snapshot, err := api.currentReloadSnapshot()
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.configuration).NotTo(BeEmpty())
		Expect(snapshot.routes.RouteOpts.Receiver).To(Equal("primary"))
		Expect(snapshot.routes.Matchers[0].Value).To(Equal("api"))
		Expect(snapshot.routes.RouteOpts.Labels["owner"]).To(Equal(model.LabelValue("platform")))
		Expect(snapshot.receivers).To(Equal([]receiverMetadata{
			{name: "primary", labels: map[string]string{"name": "primary", "team": "platform"}},
			{name: "secondary", labels: map[string]string{"name": "secondary"}},
		}))

		cfg.Route.Receiver = "changed"
		cfg.Route.Matchers[0].Value = "changed"
		cfg.Route.Labels["owner"] = "changed"
		cfg.Receivers[0].Labels["team"] = "changed"
		Expect(snapshot.routes.RouteOpts.Receiver).To(Equal("primary"))
		Expect(snapshot.routes.Matchers[0].Value).To(Equal("api"))
		Expect(snapshot.routes.RouteOpts.Labels["owner"]).To(Equal(model.LabelValue("platform")))
		Expect(snapshot.receivers[0].labels["team"]).To(Equal("platform"))

		snapshot.setAlertStatus(context.Background(), nil)
		Expect(callbackCalls.Load()).To(Equal(int64(1)))
	})

	It("returns Unavailable before configuration is loaded", func() {
		api := newTestAPI(Options{})
		_, err := api.currentReloadSnapshot()
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeUnavailable))

		api.Update(&config.Config{}, nil)
		_, err = api.currentReloadSnapshot()
		Expect(err).NotTo(HaveOccurred())
		api.Update(nil, nil)
		_, err = api.currentReloadSnapshot()
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeUnavailable))
	})

	It("atomically swaps complete snapshots", func() {
		api := newTestAPI(Options{})
		makeConfig := func(value string) *config.Config {
			return &config.Config{
				Route:     &config.Route{Receiver: value},
				Receivers: []config.Receiver{{Name: value, Labels: map[string]string{"version": value}}},
			}
		}
		api.Update(makeConfig("a"), nil)

		var waitGroup sync.WaitGroup
		var inconsistent atomic.Bool
		waitGroup.Add(5)
		for range 4 {
			go func() {
				defer waitGroup.Done()
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
			for index := range 1_000 {
				if index%2 == 0 {
					api.Update(makeConfig("a"), nil)
				} else {
					api.Update(makeConfig("b"), nil)
				}
			}
		}()
		waitGroup.Wait()
		Expect(inconsistent.Load()).To(BeFalse())
	})
})
