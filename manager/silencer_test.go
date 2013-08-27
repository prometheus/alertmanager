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

type testSilencerScenario struct {
	silences   Silences
	silenced   Alerts
	unsilenced Alerts
}

func (scenario *testSilencerScenario) test(i int, t *testing.T) {
	s := NewSilencer()

	for j, sc := range scenario.silences {
		id := s.AddSilence(sc)
		retrievedSilence, err := s.GetSilence(id)
		if err != nil {
			t.Fatalf("%d.%d. Error getting silence: %s", i, j, err)
		}
		if retrievedSilence.Id != id {
			t.Fatalf("%d.%d. Expected ID %d, got %d", i, j, id, retrievedSilence.Id)
		}
		sc.Id = id
		if sc != retrievedSilence {
			t.Fatalf("%d.%d. Expected silence %v, got  %v", i, j, sc, retrievedSilence)
		}
	}

	for j, a := range scenario.silenced {
		silenced, sc := s.IsSilenced(a.Labels)
		if !silenced {
			t.Fatalf("%d.%d. Expected %v to be silenced", i, j, a)
		}
		if sc == nil {
			t.Fatalf("%d.%d. Expected non-nil Silence for silenced event %v", i, j, a)
		}
	}

	for j, a := range scenario.unsilenced {
		silenced, sc := s.IsSilenced(a.Labels)
		if silenced {
			t.Fatalf("%d.%d. Expected %v to not be silenced, was silenced by %v", i, j, a, sc)
		}
	}

	l := AlertLabelSets{}
	for _, a := range append(scenario.silenced, scenario.unsilenced...) {
		l = append(l, a.Labels)
	}
	unsilenced := AlertLabelSets{}
	for _, a := range scenario.unsilenced {
		unsilenced = append(unsilenced, a.Labels)
	}
	filtered := s.Filter(l)
	labelSetsMustBeEqual(i, t, filtered, unsilenced)

	silences := s.SilenceSummary()
	if len(silences) != len(scenario.silences) {
		t.Fatalf("%d. Expected %d silences, got %d", i, len(scenario.silences), len(silences))
	}

	for j, sc := range silences {
		if err := s.DelSilence(sc.Id); err != nil {
			t.Fatalf("%d.%d. Got error while deleting silence: %s", i, j, err)
		}

		newSilences := s.SilenceSummary()
		if len(newSilences) != len(silences)-j-1 {
			t.Fatalf("%d. Expected %d silences, got %d", i, len(silences), len(newSilences))
		}
	}

	s.Close()
}

func TestSilencer(t *testing.T) {
	scenarios := []testSilencerScenario{
		{
			// No silences, one event.
			unsilenced: Alerts{
				&Alert{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
		},
		{
			// One rule, two matching events, one non-matching.
			silences: Silences{
				&Silence{
					Filters: Filters{NewFilter("service", "test(-)?service")},
					EndsAt:  time.Now().Add(time.Hour),
				},
				&Silence{
					Filters: Filters{NewFilter("testlabel", ".*")},
					EndsAt:  time.Now().Add(time.Hour),
				},
			},
			silenced: Alerts{
				&Alert{
					Labels: map[string]string{
						"service": "testservice",
						"foo":     "bar",
					},
				},
				&Alert{
					Labels: map[string]string{
						"service": "test-service",
						"bar":     "baz",
					},
				},
				&Alert{
					Labels: map[string]string{
						"service":   "bar-service",
						"testlabel": "testvalue",
					},
				},
			},
			unsilenced: Alerts{
				&Alert{
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
