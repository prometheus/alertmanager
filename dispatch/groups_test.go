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

package dispatch

import (
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/provider/mem"
)

// timeMuterFuncs is a TimeMuter backed by functions.
type timeMuterFuncs struct {
	mutes    func([]string, time.Time) (bool, []string, error)
	contains func([]string, time.Time) (bool, error)
}

func (f timeMuterFuncs) Mutes(names []string, now time.Time) (bool, []string, error) {
	return f.mutes(names, now)
}

func (f timeMuterFuncs) ContainsTime(names []string, now time.Time) (bool, error) {
	return f.contains(names, now)
}

func TestGroupsTimeIntervalEvaluation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		alerts       int
		active       bool
		rejectRoute  bool
		rejectAlerts bool
		muted        bool
		zeroCount    bool
		fail         bool
		wantGroups   int
		wantCalls    int
	}{
		{name: "empty route", fail: true},
		{name: "rejected route", alerts: 3, rejectRoute: true, fail: true},
		{name: "rejected alerts", alerts: 3, rejectAlerts: true, fail: true},
		{name: "unmuted groups", alerts: 3, wantGroups: 3, wantCalls: 1},
		{name: "muted groups", alerts: 3, muted: true, wantGroups: 3, wantCalls: 1},
		{name: "published groups before count update", alerts: 3, zeroCount: true, wantGroups: 3, wantCalls: 1},
		{name: "interval error", alerts: 3, fail: true, wantCalls: 1},
		{name: "active unmuted groups", alerts: 3, active: true, wantGroups: 3, wantCalls: 1},
		{name: "active muted groups", alerts: 3, active: true, muted: true, wantGroups: 3, wantCalls: 1},
		{name: "active interval error", alerts: 3, active: true, fail: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				logger := promslog.NewNopLogger()
				recorder := eventrecorder.NopRecorder()
				alerts, err := mem.NewAlerts(t.Context(), time.Hour, 0, nil, logger, recorder, prometheus.NewRegistry(), nil)
				require.NoError(t, err)
				defer alerts.Close()
				for i := range tc.alerts {
					require.NoError(t, alerts.Put(t.Context(), newAlert(model.LabelSet{"alertname": model.LabelValue(fmt.Sprintf("Alert%d", i))})))
				}
				child := &config.Route{MuteTimeIntervals: []string{"maintenance"}}
				if tc.active {
					child = &config.Route{ActiveTimeIntervals: []string{"maintenance"}}
				}
				route := NewRoute(&config.Route{
					Receiver: "test",
					GroupBy:  []model.LabelName{"alertname"},
					Routes:   []*config.Route{child},
				}, nil)
				var (
					calls     int
					queryTime time.Time
				)
				intervalErr := errors.New("cannot evaluate intervals")
				check := func(names []string, now time.Time) error {
					calls++
					require.Equal(t, []string{"maintenance"}, names)
					require.Equal(t, queryTime, now)
					if tc.fail {
						return intervalErr
					}
					return nil
				}
				muter := timeMuterFuncs{
					mutes: func(names []string, now time.Time) (bool, []string, error) {
						// Active intervals only need the predicate, so Mutes
						// must not be called for them.
						require.False(t, tc.active, "Mutes called for active time intervals")
						if err := check(names, now); err != nil {
							return false, nil, err
						}
						if tc.muted {
							return true, names, nil
						}
						return false, nil, nil
					},
					contains: func(names []string, now time.Time) (bool, error) {
						// Mute intervals need the matching names, so
						// ContainsTime must not be called for them.
						require.True(t, tc.active, "ContainsTime called for mute time intervals")
						if err := check(names, now); err != nil {
							return false, err
						}
						// A group is muted by its active intervals when now is
						// outside all of them.
						return !tc.muted, nil
					},
				}
				d := NewDispatcher(alerts, route, nil, muter, nil, testMaintenanceInterval, nil, logger, recorder, nil, nil)
				// Populate groups without running the notification pipeline.
				go d.Run(time.Now().Add(time.Hour))
				d.WaitForLoading()
				defer d.Stop()
				if tc.zeroCount {
					// Reproduce the window after publishing a group but before
					// incrementing the count used to size the snapshot.
					d.routeGroupsSlice[route.Routes[0].Idx].groupsLen.Store(0)
				}
				for range 2 {
					calls = 0
					queryTime = time.Now()
					groups, _, err := d.Groups(t.Context(), func(*Route) bool { return !tc.rejectRoute }, func(_ *alert.Alert, now time.Time) (alert.AlertStatus, bool) {
						require.Equal(t, queryTime, now)
						return alert.AlertStatus{}, !tc.rejectAlerts
					})
					if tc.fail && tc.wantCalls > 0 {
						require.ErrorIs(t, err, intervalErr)
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, tc.wantCalls, calls)
					require.Len(t, groups, tc.wantGroups)
					for _, group := range groups {
						// The route has intervals of one kind only, so only
						// that kind can mute its groups.
						mutedBy, otherKind := group.MutedByMuteTimeIntervals, group.MutedByActiveTimeIntervals
						if tc.active {
							mutedBy, otherKind = otherKind, mutedBy
						}
						require.Empty(t, otherKind)
						if tc.muted {
							require.Equal(t, []string{"maintenance"}, mutedBy)
						} else {
							require.Empty(t, mutedBy)
						}
					}
					// Each query must evaluate intervals using its own timestamp.
					time.Sleep(time.Second)
				}
			})
		})
	}
}
