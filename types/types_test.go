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
	"encoding/json"
	"reflect"
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

func TestAlertStatusMarshal(t *testing.T) {
	type statusTest struct {
		alertStatus AlertStatus
		status      string
		values      []string
	}

	tests := []statusTest{
		statusTest{
			alertStatus: AlertStatus{},
			status:      "unprocessed",
			values:      []string{},
		},
		statusTest{
			alertStatus: AlertStatus{status: Unprocessed},
			status:      "unprocessed",
			values:      []string{},
		},
		statusTest{
			alertStatus: AlertStatus{status: Active},
			status:      "active",
			values:      []string{},
		},
		statusTest{
			alertStatus: AlertStatus{status: Silenced, values: []string{"123456"}},
			status:      "silenced",
			values:      []string{"123456"},
		},
		statusTest{
			alertStatus: AlertStatus{status: Inhibited},
			status:      "inhibited",
			values:      []string{},
		},
		statusTest{
			alertStatus: AlertStatus{status: 255},
			status:      "unknown",
			values:      []string{},
		},
	}
	for _, asTest := range tests {
		b, err := json.Marshal(&asTest.alertStatus)
		if err != nil {
			t.Error(err)
		}
		expectedJSON, _ := json.Marshal(map[string]interface{}{
			"status": asTest.status,
			"values": asTest.values,
		})
		if string(b) != string(expectedJSON) {
			t.Errorf("%v serialization failed, expected %s, got %s", asTest.alertStatus, expectedJSON, b)
		}
	}
}
