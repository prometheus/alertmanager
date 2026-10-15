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

package v2

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/alert"
	alertgroup_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroup"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

// BenchmarkGetAlertGroupsHandler measures the alert groups handler end to end
// against a real silencer and inhibitor, which is where the handler spends
// its time: the mute pipeline runs for every alert in every group. Alerts are
// spread over 100 services and 100 nodes; silences cover 10 services and
// inhibiting source alerts cover 10 nodes, so about 10% of the alerts are
// silenced and about 10% inhibited.
func BenchmarkGetAlertGroupsHandler(b *testing.B) {
	for _, tc := range []struct{ groups, alertsPerGroup int }{
		{100, 10},
		{1000, 10},
		{100, 100},
	} {
		b.Run(fmt.Sprintf("groups=%d/alerts=%d", tc.groups, tc.alertsPerGroup), func(b *testing.B) {
			benchmarkGetAlertGroupsHandler(b, tc.groups, tc.alertsPerGroup)
		})
	}
}

func benchmarkGetAlertGroupsHandler(b *testing.B, numGroups, alertsPerGroup int) {
	const (
		numServices       = 100
		numNodes          = 100
		silencedServices  = 10
		inhibitedNodes    = 10
		inhibitionTimeout = 10 * time.Second
	)
	ctx := context.Background()
	logger := promslog.NewNopLogger()
	now := time.Now()

	cfg, err := config.Load(`
route:
  receiver: team
receivers:
- name: team
inhibit_rules:
- source_matchers: ['alertname="NodeDown"']
  target_matchers: ['severity="warning"']
  equal: ['node']
`)
	require.NoError(b, err)

	// Silencer with real silences.
	silences, err := silence.New(silence.Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(b, err)
	for i := range silencedServices {
		require.NoError(b, silences.Set(ctx, &silencepb.Silence{
			Matchers:  []*silencepb.Matcher{{Type: silencepb.Matcher_EQUAL, Name: "svc", Pattern: fmt.Sprintf("svc-%d", i)}},
			StartsAt:  timestamppb.New(now.Add(-time.Hour)),
			EndsAt:    timestamppb.New(now.Add(time.Hour)),
			CreatedBy: "bench",
			Comment:   "bench",
		}))
	}
	silencer := silence.NewSilencer(silences, logger, eventrecorder.NopRecorder())

	// Inhibitor fed with real source alerts through a provider.
	sources, err := mem.NewAlerts(ctx, time.Hour, 0, nil, logger, eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	require.NoError(b, err)
	defer sources.Close()
	for i := range inhibitedNodes {
		require.NoError(b, sources.Put(ctx, alert.New(model.Alert{
			Labels:   model.LabelSet{"alertname": "NodeDown", "node": model.LabelValue(fmt.Sprintf("node-%d", i))},
			StartsAt: now.Add(-time.Minute),
			EndsAt:   now.Add(time.Hour),
		}, now, false)))
	}
	inhibitor := inhibit.NewInhibitor(sources, cfg.InhibitRules, logger, eventrecorder.NopRecorder())
	go inhibitor.Run()
	defer inhibitor.Stop()
	// Wait until the inhibitor has seeded its source cache.
	probe := labelset.FromModel(model.LabelSet{"alertname": "Probe", "severity": "warning", "node": "node-0"})
	require.Eventually(b, func() bool { return inhibitor.Mutes(ctx, probe) }, inhibitionTimeout, time.Millisecond)

	// Aggregation groups with distinct alerts, as the dispatcher would hold them.
	groups := make(dispatch.AlertGroups, 0, numGroups)
	receivers := map[model.Fingerprint][]string{}
	for g := range numGroups {
		ag := &dispatch.AlertGroup{
			Labels:   model.LabelSet{"alertname": model.LabelValue(fmt.Sprintf("Alert%d", g))},
			Receiver: "team",
			GroupKey: fmt.Sprintf("{}:{alertname=\"Alert%d\"}", g),
			RouteID:  "{}",
		}
		for m := range alertsPerGroup {
			n := g*alertsPerGroup + m
			a := alert.New(model.Alert{
				Labels: model.LabelSet{
					"alertname": model.LabelValue(fmt.Sprintf("Alert%d", g)),
					"instance":  model.LabelValue(fmt.Sprintf("inst-%d", m)),
					"svc":       model.LabelValue(fmt.Sprintf("svc-%d", n%numServices)),
					"node":      model.LabelValue(fmt.Sprintf("node-%d", n%numNodes)),
					"severity":  "warning",
				},
				StartsAt: now.Add(-time.Minute),
				EndsAt:   now.Add(time.Hour),
			}, now, false)
			ag.Alerts = append(ag.Alerts, a)
			receivers[a.Fingerprint()] = []string{"team"}
		}
		groups = append(groups, ag)
	}

	api := API{
		uptime:      time.Now(),
		logger:      logger,
		alertGroups: benchGroupsFn(groups, receivers),
	}
	api.Update(cfg, func(ctx context.Context, lset labelset.LabelSet) {
		inhibitor.Mutes(ctx, lset)
		silencer.Mutes(ctx, lset)
	})

	r, err := http.NewRequest(http.MethodGet, "/api/v2/alerts/groups", nil)
	require.NoError(b, err)
	truePtr := true
	params := alertgroup_ops.GetAlertGroupsParams{
		HTTPRequest: r,
		Active:      &truePtr,
		Silenced:    &truePtr,
		Inhibited:   &truePtr,
		Muted:       &truePtr,
	}

	b.ReportAllocs()
	for b.Loop() {
		resp, ok := api.getAlertGroupsHandler(params).(*alertgroup_ops.GetAlertGroupsOK)
		if !ok || len(resp.Payload) != numGroups {
			b.Fatalf("unexpected response %T", resp)
		}
	}
}

// benchGroupsFn mimics dispatch.Groups over a fixed set of groups: it calls
// the filter once per alert per group and records the returned status.
func benchGroupsFn(groups dispatch.AlertGroups, receivers map[model.Fingerprint][]string) groupsFn {
	return func(_ context.Context, _ func(*dispatch.Route) bool, af dispatch.AlertFilter) (dispatch.AlertGroups, map[model.Fingerprint][]string, error) {
		now := time.Now()
		res := make(dispatch.AlertGroups, 0, len(groups))
		for _, g := range groups {
			out := &dispatch.AlertGroup{
				Labels:        g.Labels,
				Receiver:      g.Receiver,
				GroupKey:      g.GroupKey,
				RouteID:       g.RouteID,
				Alerts:        make(alert.AlertSlice, 0, len(g.Alerts)),
				AlertStatuses: make(map[model.Fingerprint]alert.AlertStatus, len(g.Alerts)),
			}
			for _, a := range g.Alerts {
				status, ok := af(a, now)
				if !ok {
					continue
				}
				out.Alerts = append(out.Alerts, a)
				out.AlertStatuses[a.Fingerprint()] = status
			}
			res = append(res, out)
		}
		return res, receivers, nil
	}
}
