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

	"github.com/prometheus/common/model"
)

type testInhibitorScenario struct {
	rules       InhibitRules
	inhibited   []model.LabelSet
	uninhibited []model.LabelSet
}

func (s *testInhibitorScenario) test(i int, t *testing.T) {
	allLabelSets := append(s.inhibited, s.uninhibited...)

	// Set the inhibit rules to an empty list.
	inhibitor := new(Inhibitor)
	filtered := inhibitor.Filter(allLabelSets)
	labelSetsMustBeEqual(i, t, allLabelSets, filtered)

	// Add inhibit rules through SetInhibitRules().
	inhibitor.SetInhibitRules(s.rules)
	filtered = inhibitor.Filter(allLabelSets)
	labelSetsMustBeEqual(i, t, s.uninhibited, filtered)
}

func TestInhibitor(t *testing.T) {
	scenarios := []testInhibitorScenario{
		// No rules.
		{
			uninhibited: []model.LabelSet{
				{
					"alertname": "InstanceDown",
					"instance":  "1",
					"job":       "testjob",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "2",
					"job":       "testjob",
				},
				{
					"alertname": "JobDown",
					"job":       "testinstance",
				},
			},
		},
		// One rule not matching anything.
		{
			rules: InhibitRules{
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "OtherAlert"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "OtherAlert2"),
					},
					MatchOn: []string{"job"},
				},
			},
			uninhibited: []model.LabelSet{
				{
					"alertname": "InstanceDown",
					"instance":  "1",
					"job":       "testjob",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "2",
					"job":       "testjob",
				},
				{
					"alertname": "JobDown",
					"job":       "testinstance",
				},
			},
		},
		// One rule matching source and target alerts, but those not matching on labels.
		{
			rules: InhibitRules{
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "JobDown"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "InstanceDown"),
					},
					MatchOn: []string{"job", "zone"},
				},
			},
			uninhibited: []model.LabelSet{
				{
					"alertname": "InstanceDown",
					"instance":  "1",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "2",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "JobDown",
					"job":       "testinstance",
					"zone":      "ab",
				},
			},
		},
		// Two rules, various match behaviors.
		{
			rules: InhibitRules{
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "JobDown"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "InstanceDown"),
					},
					MatchOn: []string{"job", "zone"},
				},
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "EverythingDown"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "JobDown"),
					},
					MatchOn: []string{"owner"},
				},
			},
			uninhibited: []model.LabelSet{
				{
					"alertname": "JobDown",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "JobDown",
					"job":       "testjob",
					"zone":      "ab",
				},
			},
			inhibited: []model.LabelSet{
				{
					"alertname": "InstanceDown",
					"instance":  "1",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "2",
					"job":       "testjob",
					"zone":      "aa",
				},
			},
		},
		// Inhibited alert inhibiting another alert (ZoneDown => JobDown => InstanceDown).
		{
			rules: InhibitRules{
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "JobDown"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "InstanceDown"),
					},
					MatchOn: []string{"job", "zone"},
				},
				&InhibitRule{
					SourceFilters: Filters{
						NewFilter("alertname", "ZoneDown"),
					},
					TargetFilters: Filters{
						NewFilter("alertname", "JobDown"),
					},
					MatchOn: []string{"zone"},
				},
			},
			uninhibited: []model.LabelSet{
				{
					"alertname": "ZoneDown",
					"zone":      "aa",
				},
				{
					"alertname": "JobDown",
					"job":       "testjob",
					"zone":      "ab",
				},
			},
			inhibited: []model.LabelSet{
				{
					"alertname": "JobDown",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "1",
					"job":       "testjob",
					"zone":      "aa",
				},
				{
					"alertname": "InstanceDown",
					"instance":  "2",
					"job":       "testjob",
					"zone":      "aa",
				},
			},
		},
	}

	for i, scenario := range scenarios {
		scenario.test(i, t)
	}
}
