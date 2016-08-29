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

package types

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestAlertMerge(t *testing.T) {
	now := time.Now()

	pairs := []struct {
		A, B, Res *Alert
	}{
		{
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
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
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
		},
	}

	for _, p := range pairs {
		if res := p.A.Merge(p.B); !reflect.DeepEqual(p.Res, res) {
			t.Errorf("unexpected merged alert %#v", res)
		}
	}
}

func TestSilencesSliceSortsCreatedAt(t *testing.T) {
	t0 := time.Now()
	ts := []time.Time{
		t0.Add(15 * time.Minute),
		t0.Add(10 * time.Minute),
		t0.Add(5 * time.Minute),
		t0,
	}
	ss := NewSilencesSorter(
		[]*Silence{
			&Silence{
				Silence: model.Silence{CreatedAt: ts[1]},
			},
			&Silence{
				Silence: model.Silence{CreatedAt: ts[0]},
			},
			&Silence{
				Silence: model.Silence{CreatedAt: ts[3]},
			},
			&Silence{
				Silence: model.Silence{CreatedAt: ts[2]},
			},
		}, ByCreatedAt)

	sort.Sort(ss)
	for i, sil := range ss.silences {
		if ts[i] != sil.Silence.CreatedAt {
			t.Fatalf("sort descending failed")
		}
	}
}
func TestSilencesSliceSortsEndsAt(t *testing.T) {
	t0 := time.Now()
	ts := []time.Time{
		t0.Add(15 * time.Minute),
		t0.Add(10 * time.Minute),
		t0.Add(5 * time.Minute),
		t0,
	}
	ss := NewSilencesSorter(
		[]*Silence{
			&Silence{
				Silence: model.Silence{EndsAt: ts[1]},
			},
			&Silence{
				Silence: model.Silence{EndsAt: ts[0]},
			},
			&Silence{
				Silence: model.Silence{EndsAt: ts[3]},
			},
			&Silence{
				Silence: model.Silence{EndsAt: ts[2]},
			},
		}, ByEndsAt)

	sort.Sort(ss)
	for i, sil := range ss.silences {
		if ts[i] != sil.Silence.EndsAt {
			t.Fatalf("sort descending failed")
		}
	}
}
func TestSilencesSliceSortsStartsAt(t *testing.T) {
	t0 := time.Now()
	ts := []time.Time{
		t0.Add(15 * time.Minute),
		t0.Add(10 * time.Minute),
		t0.Add(5 * time.Minute),
		t0,
	}
	ss := NewSilencesSorter(
		[]*Silence{
			&Silence{
				Silence: model.Silence{StartsAt: ts[1]},
			},
			&Silence{
				Silence: model.Silence{StartsAt: ts[0]},
			},
			&Silence{
				Silence: model.Silence{StartsAt: ts[3]},
			},
			&Silence{
				Silence: model.Silence{StartsAt: ts[2]},
			},
		}, ByStartsAt)

	sort.Sort(ss)
	for i, sil := range ss.silences {
		if ts[i] != sil.Silence.StartsAt {
			t.Fatalf("sort descending failed")
		}
	}
}
