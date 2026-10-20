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

	"github.com/prometheus/alertmanager/alert"
	alertgroup_ops "github.com/prometheus/alertmanager/api/v2/restapi/operations/alertgroup"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/timeinterval"
)

// This benchmark uses the real dispatcher and real silencer/inhibitor. It
// measures the handler through response construction, without JSON encoding.
func BenchmarkGetAlertGroupsHandlerTimeIntervals(b *testing.B) {
	for _, tc := range []struct {
		name    string
		alerts  int
		active  bool
		windows int
	}{
		{"500_routes_10_groups/no_intervals", 10, false, 0},
		{"500_routes_10_groups/active", 10, true, 1},
		{"500_routes_10_groups/20_mute_windows", 10, false, 20},
		{"500_routes_500_groups/active", 500, true, 1},
		{"500_routes_no_groups/active", 0, true, 1},
	} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			logger := promslog.NewNopLogger()
			recorder := eventrecorder.NopRecorder()
			cfg := &config.Config{
				Route:     &config.Route{Receiver: "team", GroupBy: []model.LabelName{"alertname"}},
				Receivers: []config.Receiver{{Name: "team"}},
			}
			intervals := map[string][]timeinterval.TimeInterval{}
			if tc.active {
				intervals["working"] = []timeinterval.TimeInterval{{
					Times:    []timeinterval.TimeRange{{StartMinute: 0, EndMinute: 1440}},
					Weekdays: []timeinterval.WeekdayRange{{InclusiveRange: timeinterval.InclusiveRange{Begin: 0, End: 6}}},
				}}
			} else if tc.windows > 0 {
				for n := range tc.windows {
					intervals["maintenance"] = append(intervals["maintenance"], timeinterval.TimeInterval{
						Years: []timeinterval.YearRange{{InclusiveRange: timeinterval.InclusiveRange{Begin: 3000 + n, End: 3000 + n}}},
					})
				}
			}
			for name, windows := range intervals {
				cfg.TimeIntervals = append(cfg.TimeIntervals, config.TimeInterval{Name: name, TimeIntervals: windows})
			}
			for r := range 500 {
				rt := &config.Route{Match: map[string]string{"team": fmt.Sprint(r)}}
				if tc.active {
					rt.ActiveTimeIntervals = []string{"working"}
				}
				if tc.windows > 0 && !tc.active {
					rt.MuteTimeIntervals = []string{"maintenance"}
				}
				cfg.Route.Routes = append(cfg.Route.Routes, rt)
			}
			alerts, err := mem.NewAlerts(ctx, time.Hour, 0, nil, logger, recorder, prometheus.NewRegistry(), nil)
			require.NoError(b, err)
			defer alerts.Close()
			now := time.Now()
			for i := range tc.alerts {
				require.NoError(b, alerts.Put(ctx, alert.New(model.Alert{
					Labels:   model.LabelSet{"alertname": model.LabelValue(fmt.Sprintf("Alert%d", i)), "team": model.LabelValue(fmt.Sprint(i))},
					StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
				}, now, false)))
			}
			intervener := timeinterval.NewIntervener(intervals)
			d := dispatch.NewDispatcher(alerts, dispatch.NewRoute(cfg.Route, nil), nil, intervener, nil, time.Hour, nil, logger, recorder, nil, nil)
			go d.Run(now.Add(time.Hour))
			d.WaitForLoading()
			defer d.Stop()
			groups, _, err := d.Groups(ctx, func(*dispatch.Route) bool { return true }, func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true })
			require.NoError(b, err)
			require.Len(b, groups, tc.alerts)
			silences, err := silence.New(silence.Options{Metrics: prometheus.NewRegistry()})
			require.NoError(b, err)
			silencer := silence.NewSilencer(silences, logger, recorder)
			inhibitor := inhibit.NewInhibitor(nil, nil, logger, recorder)
			api := API{uptime: now, logger: logger, alertGroups: d.Groups}
			api.Update(cfg, func(ctx context.Context, ls labelset.LabelSet) {
				inhibitor.Mutes(ctx, ls)
				silencer.Mutes(ctx, ls)
			})
			r, err := http.NewRequest(http.MethodGet, "/api/v2/alerts/groups", nil)
			require.NoError(b, err)
			params := alertgroup_ops.GetAlertGroupsParams{HTTPRequest: r, Active: new(true), Silenced: new(true), Inhibited: new(true), Muted: new(true)}
			b.ReportAllocs()
			for b.Loop() {
				resp, ok := api.getAlertGroupsHandler(params).(*alertgroup_ops.GetAlertGroupsOK)
				if !ok || len(resp.Payload) != tc.alerts {
					b.Fatalf("unexpected response: %T", resp)
				}
			}
		})
	}
}
