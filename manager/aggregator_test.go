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

	pb "github.com/prometheus/alertmanager/config/generated"
)

type dummyNotifier struct{}

func (d *dummyNotifier) QueueNotification(*Event, string) error {
	return nil
}

func (d *dummyNotifier) SetNotificationConfigs([]*pb.NotificationConfig) {}

func (d *dummyNotifier) Dispatch(IsInhibitedInterrogator) {}

func (d *dummyNotifier) Close() {}

type testAggregatorScenario struct {
	rules     AggregationRules
	inMatch   Events
	inNoMatch Events
}

func (s *testAggregatorScenario) test(i int, t *testing.T) {
	a := NewAggregator(&dummyNotifier{})
	a.SetRules(s.rules)

	if len(s.inMatch) > 0 {
		err := a.Receive(s.inMatch)
		if err != nil {
			t.Fatalf("%d. Expected input %v to match, got error: %s", i, s.inMatch, err)
		}
	}

	if len(s.inNoMatch) > 0 {
		err := a.Receive(s.inNoMatch)
		// BUG: we need to define more clearly what should happen if a subset of
		// events doesn't match. Right now we only return an error if no rules
		// are configured.
		if len(s.rules) == 0 && err == nil {
			t.Fatalf("%d. Expected aggregation error when no rules are set", i)
		}
	}

	aggs := a.AlertAggregates()
	if len(aggs) != len(s.inMatch) {
		t.Fatalf("%d. Expected %d aggregates, got %d", i, len(s.inMatch), len(aggs))
	}

	for j, agg := range aggs {
		ev := s.inMatch[j]
		if len(agg.Event.Labels) != len(ev.Labels) {
			t.Fatalf("%d.%d. Expected %d labels, got %d", i, j, len(ev.Labels), len(agg.Event.Labels))
		}

		for l, v := range agg.Event.Labels {
			if ev.Labels[l] != v {
				t.Fatalf("%d.%d. Expected label %s=%s, got %s=%s", l, ev.Labels[l], l, v)
			}
		}
	}

	a.Close()
}

func TestAggregator(t *testing.T) {
	scenarios := []testAggregatorScenario{
		{
			// No rules, one event.
			inNoMatch: Events{
				&Event{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
		},
		{
			// One rule, two matching events, one non-matching.
			rules: AggregationRules{
				&AggregationRule{
					Filters: Filters{NewFilter("service", "test(-)?service")},
				},
			},
			inMatch: Events{
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
			},
			inNoMatch: Events{
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
