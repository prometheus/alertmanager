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

package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/config"
)

func TestRouteMatch(t *testing.T) {
	in := `
send_to: 'notify-def'

routes:
- match:
    owner: 'team-A'

  send_to: 'notify-A'

  routes:
  - match:
      env: 'testing'

    send_to: 'notify-testing'

  - match:
      env: "production"

    send_to: 'notify-productionA'
    group_wait: 1m

    continue: true

  - match_re:
      env: "^produ.*$"

    send_to: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']


- match_re:
    owner: '^team-(B|C)$'

  group_by: ['foo', 'bar']
  group_wait: 2m
  send_resolved: false
  send_to: 'notify-BC'
`

	var ctree config.Route
	if err := yaml.Unmarshal([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	var (
		def  = DefaultRouteOpts
		tree = NewRoute(&ctree, &def)
	)
	lset := func(labels ...string) map[model.LabelName]struct{} {
		s := map[model.LabelName]struct{}{}
		for _, ls := range labels {
			s[model.LabelName(ls)] = struct{}{}
		}
		return s
	}

	tests := []struct {
		input  model.LabelSet
		result []*RouteOpts
	}{
		{
			input: model.LabelSet{
				"owner": "team-A",
			},
			result: []*RouteOpts{
				{
					SendTo:         "notify-A",
					GroupBy:        lset(),
					GroupWait:      def.GroupWait,
					GroupInterval:  def.GroupInterval,
					RepeatInterval: def.RepeatInterval,
					SendResolved:   def.SendResolved,
				},
			},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "unset",
			},
			result: []*RouteOpts{
				{
					SendTo:         "notify-A",
					GroupBy:        lset(),
					GroupWait:      def.GroupWait,
					GroupInterval:  def.GroupInterval,
					RepeatInterval: def.RepeatInterval,
					SendResolved:   def.SendResolved,
				},
			},
		},
		{
			input: model.LabelSet{
				"owner": "team-C",
			},
			result: []*RouteOpts{
				{
					SendTo:         "notify-BC",
					GroupBy:        lset("foo", "bar"),
					GroupWait:      2 * time.Minute,
					GroupInterval:  def.GroupInterval,
					RepeatInterval: def.RepeatInterval,
					SendResolved:   false,
				},
			},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "testing",
			},
			result: []*RouteOpts{
				{
					SendTo:         "notify-testing",
					GroupBy:        lset(),
					GroupWait:      def.GroupWait,
					GroupInterval:  def.GroupInterval,
					RepeatInterval: def.RepeatInterval,
					SendResolved:   def.SendResolved,
				},
			},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "production",
			},
			result: []*RouteOpts{
				{
					SendTo:         "notify-productionA",
					GroupBy:        lset(),
					GroupWait:      1 * time.Minute,
					GroupInterval:  def.GroupInterval,
					RepeatInterval: def.RepeatInterval,
					SendResolved:   def.SendResolved,
				},
				{
					SendTo:         "notify-productionB",
					GroupBy:        lset("job"),
					GroupWait:      30 * time.Second,
					GroupInterval:  5 * time.Minute,
					RepeatInterval: 1 * time.Hour,
					SendResolved:   def.SendResolved,
				},
			},
		},
	}

	for _, test := range tests {
		matches := tree.Match(test.input)

		if !reflect.DeepEqual(matches, test.result) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.result, matches)
		}
	}
}
