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

package notify

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/alertmanager/types"
)

func TestMuteStage(t *testing.T) {
	// Mute all label sets that have a "mute" key.
	muter := MuteFunc(func(ctx context.Context, lset model.LabelSet) bool {
		_, ok := lset["mute"]
		return ok
	})

	metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})
	stage := NewMuteStage(muter, metrics)

	in := []model.LabelSet{
		{},
		{"test": "set"},
		{"mute": "me"},
		{"foo": "bar", "test": "set"},
		{"foo": "bar", "mute": "me"},
		{},
		{"not": "muted"},
	}
	out := []model.LabelSet{
		{},
		{"test": "set"},
		{"foo": "bar", "test": "set"},
		{},
		{"not": "muted"},
	}

	var inAlerts []*types.Alert
	for _, lset := range in {
		inAlerts = append(inAlerts, &types.Alert{
			Alert: model.Alert{Labels: lset},
		})
	}

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
	suppressed := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got)) != suppressed {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressed)
	}
}

func TestMuteStageWithSilences(t *testing.T) {
	silences, err := silence.New(silence.Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	sil := &silencepb.Silence{
		EndsAt: timestamppb.New(utcNow().Add(time.Hour)),
		MatcherSets: []*silencepb.MatcherSet{{
			Matchers: []*silencepb.Matcher{{Name: "mute", Pattern: "me"}},
		}},
	}
	if err = silences.Set(t.Context(), sil); err != nil {
		t.Fatal(err)
	}

	reg := prometheus.NewRegistry()
	marker := types.NewMarker(reg)
	silencer := silence.NewSilencer(silences, marker, promslog.NewNopLogger())
	metrics := NewMetrics(reg, featurecontrol.NoopFlags{})
	stage := NewMuteStage(silencer, metrics)

	in := []model.LabelSet{
		{},
		{"test": "set"},
		{"mute": "me"},
		{"foo": "bar", "test": "set"},
		{"foo": "bar", "mute": "me"},
		{},
		{"not": "muted"},
	}
	out := []model.LabelSet{
		{},
		{"test": "set"},
		{"foo": "bar", "test": "set"},
		{},
		{"not": "muted"},
	}

	var inAlerts []*types.Alert
	for _, lset := range in {
		inAlerts = append(inAlerts, &types.Alert{
			Alert: model.Alert{Labels: lset},
		})
	}

	// Set the second alert as previously silenced with an old version
	// number. This is expected to get unsilenced by the stage.
	marker.SetActiveOrSilenced(inAlerts[1].Fingerprint(), []string{"123"})

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
	suppressedRoundOne := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got)) != suppressedRoundOne {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundOne)
	}

	// Do it again to exercise the version tracking of silences.
	_, alerts, err = stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	got = got[:0]
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}

	suppressedRoundTwo := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got) + suppressedRoundOne) != suppressedRoundTwo {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundTwo)
	}

	// Expire the silence and verify that no alerts are silenced now.
	if err := silences.Expire(t.Context(), sil.Id); err != nil {
		t.Fatal(err)
	}

	_, alerts, err = stage.Exec(t.Context(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}
	got = got[:0]
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Unmuting failed, expected: %v\ngot %v", in, got)
	}
	suppressedRoundThree := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got) + suppressedRoundTwo) != suppressedRoundThree {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundThree)
	}
}

func TestTimeMuteStage(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatalf("Failed to load location Australia/Sydney: %s", err)
	}
	eveningsAndWeekends := map[string][]timeinterval.TimeInterval{
		"evenings": {{
			Times: []timeinterval.TimeRange{{
				StartMinute: 0,   // 00:00
				EndMinute:   540, // 09:00
			}, {
				StartMinute: 1020, // 17:00
				EndMinute:   1440, // 24:00
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
		"weekends": {{
			Weekdays: []timeinterval.WeekdayRange{{
				InclusiveRange: timeinterval.InclusiveRange{Begin: 6, End: 6}, // Saturday
			}, {
				InclusiveRange: timeinterval.InclusiveRange{Begin: 0, End: 0}, // Sunday
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
	}

	tests := []struct {
		name      string
		intervals map[string][]timeinterval.TimeInterval
		now       time.Time
		alerts    []*types.Alert
		mutedBy   []string
	}{{
		name:      "Should be muted outside working hours",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 0, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings"},
	}, {
		name:      "Should not be muted during workings hours",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 9, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   nil,
	}, {
		name:      "Should be muted during weekends",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekends"},
	}, {
		name:      "Should be muted at 12pm UTC on a weekday",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings"},
	}, {
		name:      "Should be muted at 12pm UTC on a weekend",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings", "weekends"},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			marker := types.NewMarker(r)
			metrics := NewMetrics(r, featurecontrol.NoopFlags{})
			intervener := timeinterval.NewIntervener(test.intervals)
			st := NewTimeMuteStage(intervener, marker, metrics)

			// Get the names of all time intervals for the context.
			muteTimeIntervalNames := make([]string, 0, len(test.intervals))
			for name := range test.intervals {
				muteTimeIntervalNames = append(muteTimeIntervalNames, name)
			}
			// Sort the names so we can compare mutedBy with test.mutedBy.
			sort.Strings(muteTimeIntervalNames)

			ctx := context.Background()
			ctx = WithNow(ctx, test.now)
			ctx = WithGroupKey(ctx, "group1")
			ctx = WithActiveTimeIntervals(ctx, nil)
			ctx = WithMuteTimeIntervals(ctx, muteTimeIntervalNames)
			ctx = WithRouteID(ctx, "route1")

			_, active, err := st.Exec(ctx, promslog.NewNopLogger(), test.alerts...)
			require.NoError(t, err)

			if len(test.mutedBy) == 0 {
				// All alerts should be active.
				require.Len(t, active, len(test.alerts))
				// The group should not be marked.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.False(t, isMuted)
				require.Empty(t, mutedBy)
				// The metric for total suppressed notifications should not
				// have been incremented, which means it will not be collected.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
`)))
			} else {
				// All alerts should be muted.
				require.Empty(t, active)
				// The group should be marked as muted.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.True(t, isMuted)
				require.Equal(t, test.mutedBy, mutedBy)
				// Gets the metric for total suppressed notifications.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(fmt.Sprintf(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
# HELP alertmanager_notifications_suppressed_total The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.
# TYPE alertmanager_notifications_suppressed_total counter
alertmanager_notifications_suppressed_total{reason="mute_time_interval"} %d
`, len(test.alerts)))))
			}
		})
	}
}

func TestTimeActiveStage(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatalf("Failed to load location Australia/Sydney: %s", err)
	}
	weekdays := map[string][]timeinterval.TimeInterval{
		"weekdays": {{
			Weekdays: []timeinterval.WeekdayRange{{
				InclusiveRange: timeinterval.InclusiveRange{
					Begin: 1, // Monday
					End:   5, // Friday
				},
			}},
			Times: []timeinterval.TimeRange{{
				StartMinute: 540,  // 09:00
				EndMinute:   1020, // 17:00
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
	}

	tests := []struct {
		name      string
		intervals map[string][]timeinterval.TimeInterval
		now       time.Time
		alerts    []*types.Alert
		mutedBy   []string
	}{{
		name:      "Should be muted outside working hours",
		intervals: weekdays,
		now:       time.Date(2024, 1, 1, 0, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}, {
		name:      "Should not be muted during workings hours",
		intervals: weekdays,
		now:       time.Date(2024, 1, 1, 9, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   nil,
	}, {
		name:      "Should be muted during weekends",
		intervals: weekdays,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}, {
		name:      "Should be muted at 12pm UTC",
		intervals: weekdays,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			marker := types.NewMarker(r)
			metrics := NewMetrics(r, featurecontrol.NoopFlags{})
			intervener := timeinterval.NewIntervener(test.intervals)
			st := NewTimeActiveStage(intervener, marker, metrics)

			// Get the names of all time intervals for the context.
			activeTimeIntervalNames := make([]string, 0, len(test.intervals))
			for name := range test.intervals {
				activeTimeIntervalNames = append(activeTimeIntervalNames, name)
			}
			// Sort the names so we can compare mutedBy with test.mutedBy.
			sort.Strings(activeTimeIntervalNames)

			ctx := context.Background()
			ctx = WithNow(ctx, test.now)
			ctx = WithGroupKey(ctx, "group1")
			ctx = WithActiveTimeIntervals(ctx, activeTimeIntervalNames)
			ctx = WithMuteTimeIntervals(ctx, nil)
			ctx = WithRouteID(ctx, "route1")

			_, active, err := st.Exec(ctx, promslog.NewNopLogger(), test.alerts...)
			require.NoError(t, err)

			if len(test.mutedBy) == 0 {
				// All alerts should be active.
				require.Len(t, active, len(test.alerts))
				// The group should not be marked.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.False(t, isMuted)
				require.Empty(t, mutedBy)
				// The metric for total suppressed notifications should not
				// have been incremented, which means it will not be collected.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
`)))
			} else {
				// All alerts should be muted.
				require.Empty(t, active)
				// The group should be marked as muted.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.True(t, isMuted)
				require.Equal(t, test.mutedBy, mutedBy)
				// Gets the metric for total suppressed notifications.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(fmt.Sprintf(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
# HELP alertmanager_notifications_suppressed_total The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.
# TYPE alertmanager_notifications_suppressed_total counter
alertmanager_notifications_suppressed_total{reason="active_time_interval"} %d
`, len(test.alerts)))))
			}
		})
	}
}
