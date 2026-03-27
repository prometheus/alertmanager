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

package alert

import (
	"bytes"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

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

func TestAlertSliceLogValue(t *testing.T) {
	makeAlert := func(name string) *Alert {
		return &Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					model.AlertNameLabel: model.LabelValue(name),
				},
			},
		}
	}

	cases := []struct {
		name   string
		alerts AlertSlice
		want   string
	}{
		{
			name:   "empty",
			alerts: AlertSlice{},
			want:   "",
		},
		{
			name:   "single alert",
			alerts: AlertSlice{makeAlert("CPUHigh")},
			want:   "CPUHigh: 1",
		},
		{
			name: "multiple same name",
			alerts: AlertSlice{
				makeAlert("CPUHigh"),
				makeAlert("CPUHigh"),
				makeAlert("CPUHigh"),
			},
			want: "CPUHigh: 3",
		},
		{
			name: "multiple different names",
			alerts: AlertSlice{
				makeAlert("CPUHigh"),
				makeAlert("MemoryLow"),
				makeAlert("CPUHigh"),
				makeAlert("DiskFull"),
				makeAlert("MemoryLow"),
			},
			want: "CPUHigh: 2, MemoryLow: 2, DiskFull: 1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.alerts.LogValue().String()
			if got != tc.want {
				t.Errorf("LogValue() = %q, want %q", got, tc.want)
			}

			// Exercise through slog to see the actual log output.
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			logger.Debug("flushing", "numAlerts", len(tc.alerts), "alerts", tc.alerts)
			t.Logf("slog output: %s", buf.String())
		})
	}
}
