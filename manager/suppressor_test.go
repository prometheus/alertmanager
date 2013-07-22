// Copyright 2013 Prometheus Team
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

package manager

import (
	"testing"
	"time"
)

type testSuppressorScenario struct {
	suppressions Suppressions
	inhibited    Events
	uninhibited  Events
}

func (sc *testSuppressorScenario) test(i int, t *testing.T) {
	s := NewSuppressor()

	for j, sup := range sc.suppressions {
		id := s.AddSuppression(sup)
		retrievedSup, err := s.GetSuppression(id)
		if err != nil {
			t.Fatalf("%d.%d. Error getting suppression: %s", i, j, err)
		}
		if retrievedSup.Id != id {
			t.Fatalf("%d.%d. Expected ID %d, got %d", i, j, id, retrievedSup.Id)
		}
		sup.Id = id
		if sup != retrievedSup {
			t.Fatalf("%d.%d. Expected suppression %v, got  %v", i, j, sup, retrievedSup)
		}
	}

	for j, ev := range sc.inhibited {
		inhibited, sup := s.IsInhibited(ev)
		if !inhibited {
			t.Fatalf("%d.%d. Expected %v to be inhibited", i, j, ev)
		}
		if sup == nil {
			t.Fatalf("%d.%d. Expected non-nil Suppression for inhibited event %v", i, j, ev)
		}
	}

	for j, ev := range sc.uninhibited {
		inhibited, sup := s.IsInhibited(ev)
		if inhibited {
			t.Fatalf("%d.%d. Expected %v to not be inhibited, was inhibited by %v", i, j, ev, sup)
		}
	}

	suppressions := s.SuppressionSummary()
	if len(suppressions) != len(sc.suppressions) {
		t.Fatalf("%d. Expected %d suppressions, got %d", i, len(sc.suppressions), len(suppressions))
	}

	for j, sup := range suppressions {
		if err := s.DelSuppression(sup.Id); err != nil {
			t.Fatalf("%d.%d. Got error while deleting suppression: %s", i, j, err)
		}

		newSuppressions := s.SuppressionSummary()
		if len(newSuppressions) != len(suppressions)-j-1 {
			t.Fatalf("%d. Expected %d suppressions, got %d", i, len(suppressions), len(newSuppressions))
		}
	}

	s.Close()
}

func TestSuppressor(t *testing.T) {
	scenarios := []testSuppressorScenario{
		{
			// No suppressions, one event.
			uninhibited: Events{
				&Event{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
		},
		{
			// One rule, two matching events, one non-matching.
			suppressions: Suppressions{
				&Suppression{
					Filters: Filters{NewFilter("service", "test(-)?service")},
					EndsAt:  time.Now().Add(time.Hour),
				},
				&Suppression{
					Filters: Filters{NewFilter("testlabel", ".*")},
					EndsAt:  time.Now().Add(time.Hour),
				},
			},
			inhibited: Events{
				&Event{
					Labels: map[string]string{
						"service": "testservice",
						"foo":     "bar",
					},
				},
				&Event{
					Labels: map[string]string{
						"service": "test-service",
						"bar":     "baz",
					},
				},
				&Event{
					Labels: map[string]string{
						"service":   "bar-service",
						"testlabel": "testvalue",
					},
				},
			},
			uninhibited: Events{
				&Event{
					Labels: map[string]string{
						"service": "testservice2",
						"foo":     "bar",
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		scenario.test(i, t)
	}
}
