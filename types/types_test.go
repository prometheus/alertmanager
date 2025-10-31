// Copyright 2015 Prometheus Team
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

package types //nolint:revive

import (
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

func TestMemMarker_Muted(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)

	// No groups should be muted.
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Mark the group as muted because it's the weekend.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Other groups should not be marked as muted.
	timeIntervalNames, isMuted = marker.Muted("route1", "group2")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Other routes should not be marked as muted either.
	timeIntervalNames, isMuted = marker.Muted("route2", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// The group is no longer muted.
	marker.SetMuted("route1", "group1", nil)
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestMemMarker_DeleteByGroupKey(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)

	// Mark the group and check that it is muted.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for a different group key. The group should
	// still be muted.
	marker.DeleteByGroupKey("route1", "group2")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for the correct group key. The group should
	// no longer be muted.
	marker.DeleteByGroupKey("route1", "group1")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestMemMarker_Count(t *testing.T) {
	r := prometheus.NewRegistry()
	marker := NewMarker(r)
	now := time.Now()

	states := []AlertState{AlertStateSuppressed, AlertStateActive, AlertStateUnprocessed}
	countByState := func(state AlertState) int {
		return marker.Count(state)
	}

	countTotal := func() int {
		var count int
		for _, s := range states {
			count += countByState(s)
		}
		return count
	}

	require.Equal(t, 0, countTotal())

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "suppressed"},
	}
	a3 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(-1 * time.Minute),
		Labels:   model.LabelSet{"test": "resolved"},
	}

	// Insert an active alert.
	marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)
	require.Equal(t, 1, countByState(AlertStateActive))
	require.Equal(t, 1, countTotal())

	// Insert a silenced alert.
	marker.SetActiveOrSilenced(a2.Fingerprint(), 1, []string{"1"}, nil)
	require.Equal(t, 1, countByState(AlertStateSuppressed))
	require.Equal(t, 2, countTotal())

	// Insert a resolved silenced alert - it'll count as suppressed.
	marker.SetActiveOrSilenced(a3.Fingerprint(), 1, []string{"1"}, nil)
	require.Equal(t, 2, countByState(AlertStateSuppressed))
	require.Equal(t, 3, countTotal())

	// Remove the silence from a3 - it'll count as active.
	marker.SetActiveOrSilenced(a3.Fingerprint(), 1, nil, nil)
	require.Equal(t, 2, countByState(AlertStateActive))
	require.Equal(t, 3, countTotal())
}

func TestAlertMerge(t *testing.T) {
	now := time.Now()

	// By convention, alert A is always older than alert B.
	pairs := []struct {
		A, B, Res *Alert
	}{
		{
			// Both alerts have the Timeout flag set.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert B.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now,
				Timeout:   true,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
		},
		{
			// Alert A has the Timeout flag set while Alert B has it unset.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert B.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now,
				Timeout:   true,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now,
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
		{
			// Alert A has the Timeout flag unset while Alert B has it set.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert A.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now,
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
		},
		{
			// Both alerts have the Timeout flag unset and are not resolved.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert A.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now,
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
		{
			// Both alerts have the Timeout flag unset and are not resolved.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert B.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(4 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(4 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
		{
			// Both alerts have the Timeout flag unset, A is resolved while B isn't.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert B.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-3 * time.Minute),
					EndsAt:   now.Add(-time.Minute),
				},
				UpdatedAt: now,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now.Add(time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-3 * time.Minute),
					EndsAt:   now.Add(time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
		{
			// Both alerts have the Timeout flag unset, B is resolved while A isn't.
			// StartsAt is defined by Alert A.
			// EndsAt is defined by Alert B.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now,
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-2 * time.Minute),
					EndsAt:   now,
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
		{
			// Both alerts are resolved (EndsAt < now).
			// StartsAt is defined by Alert B.
			// EndsAt is defined by Alert A.
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-3 * time.Minute),
					EndsAt:   now.Add(-time.Minute),
				},
				UpdatedAt: now.Add(-time.Minute),
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-4 * time.Minute),
					EndsAt:   now.Add(-2 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-4 * time.Minute),
					EndsAt:   now.Add(-1 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
			},
		},
	}

	for i, p := range pairs {
		p := p
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if res := p.A.Merge(p.B); !reflect.DeepEqual(p.Res, res) {
				t.Errorf("unexpected merged alert %#v", res)
			}
			if res := p.B.Merge(p.A); !reflect.DeepEqual(p.Res, res) {
				t.Errorf("unexpected merged alert %#v", res)
			}
		})
	}
}

func TestValidateUTF8Ls(t *testing.T) {
	tests := []struct {
		name string
		ls   model.LabelSet
		err  string
	}{{
		name: "valid UTF-8 label set",
		ls: model.LabelSet{
			"a":                "a",
			"00":               "b",
			"Σ":                "c",
			"\xf0\x9f\x99\x82": "dΘ",
		},
	}, {
		name: "invalid UTF-8 label set",
		ls: model.LabelSet{
			"\xff": "a",
		},
		err: "invalid name \"\\xff\"",
	}}

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLs(test.ls)
			if err != nil && err.Error() != test.err {
				t.Errorf("unexpected err for %s: %s", test.ls, err)
			} else if err == nil && test.err != "" {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestCalcSilenceState(t *testing.T) {
	var (
		pastStartTime = time.Now()
		pastEndTime   = time.Now()

		futureStartTime = time.Now().Add(time.Hour)
		futureEndTime   = time.Now().Add(time.Hour)
	)

	expected := CalcSilenceState(futureStartTime, futureEndTime)
	require.Equal(t, SilenceStatePending, expected)

	expected = CalcSilenceState(pastStartTime, futureEndTime)
	require.Equal(t, SilenceStateActive, expected)

	expected = CalcSilenceState(pastStartTime, pastEndTime)
	require.Equal(t, SilenceStateExpired, expected)
}

func TestSilenceExpired(t *testing.T) {
	now := time.Now()
	silence := Silence{StartsAt: now, EndsAt: now}
	require.True(t, silence.Expired())

	silence = Silence{StartsAt: now.Add(time.Hour), EndsAt: now.Add(time.Hour)}
	require.True(t, silence.Expired())

	silence = Silence{StartsAt: now, EndsAt: now.Add(time.Hour)}
	require.False(t, silence.Expired())
}

func TestAlertSliceSort(t *testing.T) {
	var (
		a1 = &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"job":       "j1",
					"instance":  "i1",
					"alertname": "an1",
				},
			},
		}
		a2 = &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"job":       "j1",
					"instance":  "i1",
					"alertname": "an2",
				},
			},
		}
		a3 = &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"job":       "j2",
					"instance":  "i1",
					"alertname": "an1",
				},
			},
		}
		a4 = &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "an1",
				},
			},
		}
		a5 = &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "an2",
				},
			},
		}
	)

	cases := []struct {
		alerts AlertSlice
		exp    AlertSlice
	}{
		{
			alerts: AlertSlice{a2, a1},
			exp:    AlertSlice{a1, a2},
		},
		{
			alerts: AlertSlice{a3, a2, a1},
			exp:    AlertSlice{a1, a2, a3},
		},
		{
			alerts: AlertSlice{a4, a2, a4},
			exp:    AlertSlice{a2, a4, a4},
		},
		{
			alerts: AlertSlice{a5, a4},
			exp:    AlertSlice{a4, a5},
		},
	}

	for _, tc := range cases {
		sort.Stable(tc.alerts)
		if !reflect.DeepEqual(tc.alerts, tc.exp) {
			t.Fatalf("expected %v but got %v", tc.exp, tc.alerts)
		}
	}
}

type fakeRegisterer struct {
	registeredCollectors []prometheus.Collector
}

func (r *fakeRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (r *fakeRegisterer) MustRegister(c ...prometheus.Collector) {
	r.registeredCollectors = append(r.registeredCollectors, c...)
}

func (r *fakeRegisterer) Unregister(prometheus.Collector) bool {
	return false
}

func TestNewMarkerRegistersMetrics(t *testing.T) {
	fr := fakeRegisterer{}
	NewMarker(&fr)

	if len(fr.registeredCollectors) == 0 {
		t.Error("expected NewMarker to register metrics on the given registerer")
	}
}
