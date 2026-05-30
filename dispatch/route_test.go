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

package dispatch

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/config"
)

func TestRouteMatch(t *testing.T) {
	in := `
receiver: 'notify-def'

routes:
- match:
    owner: 'team-A'

  receiver: 'notify-A'

  routes:
  - match:
      env: 'testing'

    receiver: 'notify-testing'
    group_by: [...]

  - match:
      env: "production"

    receiver: 'notify-productionA'
    group_wait: 1m

    continue: true

  - match_re:
      env: "produ.*"
      job: ".*"

    receiver: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']

- match_re:
    owner: 'team-(B|C)'

  group_by: ['foo', 'bar']
  group_wait: 2m
  receiver: 'notify-BC'

- match:
    group_by: 'role'
  group_by: ['role']

  routes:
  - match:
      env: 'testing'
    receiver: 'notify-testing'
    routes:
    - match:
        wait: 'long'
      group_wait: 2m
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	var (
		def  = DefaultRouteOpts
		tree = NewRoute(&ctree, nil)
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
		keys   []string
	}{
		{
			input: model.LabelSet{
				"owner": "team-A",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "unset",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-C",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-BC",
					GroupBy:             lset("foo", "bar"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=~\"^(?:team-(B|C))$\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "testing",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset(),
					GroupByAll:          true,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=\"team-A\"}/{env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "production",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-productionA",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           1 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
				{
					Receiver:            "notify-productionB",
					GroupBy:             lset("job"),
					GroupByAll:          false,
					GroupWait:           30 * time.Second,
					GroupInterval:       5 * time.Minute,
					RepeatInterval:      1 * time.Hour,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{
				"{}/{owner=\"team-A\"}/{env=\"production\"}",
				"{}/{owner=\"team-A\"}/{env=~\"^(?:produ.*)$\",job=~\"^(?:.*)$\"}",
			},
		},
		{
			input: model.LabelSet{
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-def",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
				"wait":     "long",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}/{wait=\"long\"}"},
		},
	}

	for _, test := range tests {
		var matches []*RouteOpts
		var keys []string

		for _, r := range tree.Match(test.input) {
			matches = append(matches, &r.RouteOpts)
			keys = append(keys, r.Key())
		}

		if !reflect.DeepEqual(matches, test.result) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.result, matches)
		}

		if !reflect.DeepEqual(keys, test.keys) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.keys, keys)
		}
	}
}

func TestRouteWalk(t *testing.T) {
	in := `
receiver: 'notify-def'

routes:
- match:
    owner: 'team-A'

  receiver: 'notify-A'

  routes:
  - match:
      env: 'testing'

    receiver: 'notify-testing'
    group_by: [...]

  - match:
      env: "production"

    receiver: 'notify-productionA'
    group_wait: 1m

    continue: true

  - match_re:
      env: "produ.*"
      job: ".*"

    receiver: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']


- match_re:
    owner: 'team-(B|C)'

  group_by: ['foo', 'bar']
  group_wait: 2m
  receiver: 'notify-BC'

- match:
    group_by: 'role'
  group_by: ['role']

  routes:
  - match:
      env: 'testing'
    receiver: 'notify-testing'
    routes:
    - match:
        wait: 'long'
      group_wait: 2m
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	tree := NewRoute(&ctree, nil)

	expected := []string{
		"notify-def",
		"notify-A",
		"notify-testing",
		"notify-productionA",
		"notify-productionB",
		"notify-BC",
		"notify-def",
		"notify-testing",
		"notify-testing",
	}

	var got []string
	tree.Walk(func(r *Route) {
		got = append(got, r.RouteOpts.Receiver)
	})

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, got)
	}
}

func TestInheritParentGroupByAll(t *testing.T) {
	in := `
routes:
- match:
    env: 'parent'
  group_by: ['...']

  routes:
  - match:
      env: 'child1'

  - match:
      env: 'child2'
    group_by: ['foo']
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}

	tree := NewRoute(&ctree, nil)
	parent := tree.Routes[0]
	child1 := parent.Routes[0]
	child2 := parent.Routes[1]
	require.True(t, parent.RouteOpts.GroupByAll)
	require.True(t, child1.RouteOpts.GroupByAll)
	require.False(t, child2.RouteOpts.GroupByAll)
}

func TestInheritParentMuteTimeIntervals(t *testing.T) {
	in := `
routes:
- match:
    env: 'parent'
  group_by: ['...']
  mute_time_intervals: ['weekend_mute']

  routes:
  - match:
      env: 'child1'

  - match:
      env: 'child2'
    mute_time_intervals: ['override_mute']
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}

	tree := NewRoute(&ctree, nil)
	parent := tree.Routes[0]
	child1 := parent.Routes[0]
	child2 := parent.Routes[1]
	require.Equal(t, []string{"weekend_mute"}, parent.RouteOpts.MuteTimeIntervals)
	require.Equal(t, []string{"weekend_mute"}, child1.RouteOpts.MuteTimeIntervals)
	require.Equal(t, []string{"override_mute"}, child2.RouteOpts.MuteTimeIntervals)
}

func TestInheritParentActiveTimeIntervals(t *testing.T) {
	in := `
routes:
- match:
    env: 'parent'
  group_by: ['...']
  active_time_intervals: ['weekend_active']

  routes:
  - match:
      env: 'child1'

  - match:
      env: 'child2'
    active_time_intervals: ['override_active']
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}

	tree := NewRoute(&ctree, nil)
	parent := tree.Routes[0]
	child1 := parent.Routes[0]
	child2 := parent.Routes[1]
	require.Equal(t, []string{"weekend_active"}, parent.RouteOpts.ActiveTimeIntervals)
	require.Equal(t, []string{"weekend_active"}, child1.RouteOpts.ActiveTimeIntervals)
	require.Equal(t, []string{"override_active"}, child2.RouteOpts.ActiveTimeIntervals)
}

func TestRouteMatchers(t *testing.T) {
	in := `
receiver: 'notify-def'

routes:
- matchers: ['{owner="team-A"}', '{level!="critical"}']

  receiver: 'notify-A'

  routes:
  - matchers: ['{env="testing"}', '{baz!~".*quux"}']

    receiver: 'notify-testing'
    group_by: [...]

  - matchers: ['{env="production"}']

    receiver: 'notify-productionA'
    group_wait: 1m

    continue: true

  - matchers: [ env=~"produ.*", job=~".*"]

    receiver: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']


- matchers: [owner=~"team-(B|C)"]

  group_by: ['foo', 'bar']
  group_wait: 2m
  receiver: 'notify-BC'

- matchers: [group_by="role"]
  group_by: ['role']

  routes:
  - matchers: ['{env="testing"}']
    receiver: 'notify-testing'
    routes:
    - matchers: [wait="long"]
      group_wait: 2m
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	var (
		def  = DefaultRouteOpts
		tree = NewRoute(&ctree, nil)
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
		keys   []string
	}{
		{
			input: model.LabelSet{
				"owner": "team-A",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "unset",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-C",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-BC",
					GroupBy:             lset("foo", "bar"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=~\"team-(B|C)\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "testing",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset(),
					GroupByAll:          true,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}/{baz!~\".*quux\",env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "production",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-productionA",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           1 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
				{
					Receiver:            "notify-productionB",
					GroupBy:             lset("job"),
					GroupByAll:          false,
					GroupWait:           30 * time.Second,
					GroupInterval:       5 * time.Minute,
					RepeatInterval:      1 * time.Hour,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{
				"{}/{level!=\"critical\",owner=\"team-A\"}/{env=\"production\"}",
				"{}/{level!=\"critical\",owner=\"team-A\"}/{env=~\"produ.*\",job=~\".*\"}",
			},
		},
		{
			input: model.LabelSet{
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-def",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
				"wait":     "long",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}/{wait=\"long\"}"},
		},
	}

	for _, test := range tests {
		var matches []*RouteOpts
		var keys []string

		for _, r := range tree.Match(test.input) {
			matches = append(matches, &r.RouteOpts)
			keys = append(keys, r.Key())
		}

		if !reflect.DeepEqual(matches, test.result) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.result, matches)
		}

		if !reflect.DeepEqual(keys, test.keys) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.keys, keys)
		}
	}
}

func TestRouteMatchersAndMatch(t *testing.T) {
	in := `
receiver: 'notify-def'

routes:
- matchers: ['{owner="team-A"}', '{level!="critical"}']

  receiver: 'notify-A'

  routes:
  - matchers: ['{env="testing"}', '{baz!~".*quux"}']

    receiver: 'notify-testing'
    group_by: [...]

  - match:
      env: "production"

    receiver: 'notify-productionA'
    group_wait: 1m

    continue: true

  - matchers: [ env=~"produ.*", job=~".*"]

    receiver: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']

- match_re:
    owner: 'team-(B|C)'

  group_by: ['foo', 'bar']
  group_wait: 2m
  receiver: 'notify-BC'

- matchers: [group_by="role"]
  group_by: ['role']

  routes:
  - matchers: ['{env="testing"}']
    receiver: 'notify-testing'
    routes:
    - matchers: [wait="long"]
      group_wait: 2m
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	var (
		def  = DefaultRouteOpts
		tree = NewRoute(&ctree, nil)
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
		keys   []string
	}{
		{
			input: model.LabelSet{
				"owner": "team-A",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "unset",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-A",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-C",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-BC",
					GroupBy:             lset("foo", "bar"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{owner=~\"^(?:team-(B|C))$\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "testing",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset(),
					GroupByAll:          true,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{level!=\"critical\",owner=\"team-A\"}/{baz!~\".*quux\",env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"owner": "team-A",
				"env":   "production",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-productionA",
					GroupBy:             def.GroupBy,
					GroupByAll:          false,
					GroupWait:           1 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
				{
					Receiver:            "notify-productionB",
					GroupBy:             lset("job"),
					GroupByAll:          false,
					GroupWait:           30 * time.Second,
					GroupInterval:       5 * time.Minute,
					RepeatInterval:      1 * time.Hour,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{
				"{}/{level!=\"critical\",owner=\"team-A\"}/{env=\"production\"}",
				"{}/{level!=\"critical\",owner=\"team-A\"}/{env=~\"produ.*\",job=~\".*\"}",
			},
		},
		{
			input: model.LabelSet{
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-def",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           def.GroupWait,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}"},
		},
		{
			input: model.LabelSet{
				"env":      "testing",
				"group_by": "role",
				"wait":     "long",
			},
			result: []*RouteOpts{
				{
					Receiver:            "notify-testing",
					GroupBy:             lset("role"),
					GroupByAll:          false,
					GroupWait:           2 * time.Minute,
					GroupInterval:       def.GroupInterval,
					RepeatInterval:      def.RepeatInterval,
					MuteTimeIntervals:   def.MuteTimeIntervals,
					ActiveTimeIntervals: def.ActiveTimeIntervals,
				},
			},
			keys: []string{"{}/{group_by=\"role\"}/{env=\"testing\"}/{wait=\"long\"}"},
		},
	}

	for _, test := range tests {
		var matches []*RouteOpts
		var keys []string

		for _, r := range tree.Match(test.input) {
			matches = append(matches, &r.RouteOpts)
			keys = append(keys, r.Key())
		}

		if !reflect.DeepEqual(matches, test.result) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.result, matches)
		}

		if !reflect.DeepEqual(keys, test.keys) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", test.keys, keys)
		}
	}
}

func TestRouteID(t *testing.T) {
	in := `
receiver: default
routes:
- continue: true
  matchers:
  - foo=bar
  receiver: test1
  routes:
  - matchers:
    - bar=baz
- continue: true
  matchers:
  - foo=bar
  receiver: test1
  routes:
  - matchers:
    - bar=baz
- continue: true
  matchers:
  - foo=bar
  receiver: test2
  routes:
  - matchers:
    - bar=baz
- continue: true
  matchers:
  - bar=baz
  receiver: test3
  routes:
  - matchers:
    - baz=qux
  - matchers:
    - qux=corge
- continue: true
  matchers:
  - qux=~"[a-zA-Z0-9]+"
- continue: true
  matchers:
  - corge!~"[0-9]+"
`
	cr := config.Route{}
	require.NoError(t, yaml.Unmarshal([]byte(in), &cr))
	r := NewRoute(&cr, nil)

	expected := []string{
		"{}",
		"{}/{foo=\"bar\"}/0",
		"{}/{foo=\"bar\"}/0/{bar=\"baz\"}/0",
		"{}/{foo=\"bar\"}/1",
		"{}/{foo=\"bar\"}/1/{bar=\"baz\"}/0",
		"{}/{foo=\"bar\"}/2",
		"{}/{foo=\"bar\"}/2/{bar=\"baz\"}/0",
		"{}/{bar=\"baz\"}/3",
		"{}/{bar=\"baz\"}/3/{baz=\"qux\"}/0",
		"{}/{bar=\"baz\"}/3/{qux=\"corge\"}/1",
		"{}/{qux=~\"[a-zA-Z0-9]+\"}/4",
		"{}/{corge!~\"[0-9]+\"}/5",
	}

	var actual []string
	r.Walk(func(r *Route) {
		actual = append(actual, r.ID())
	})
	require.ElementsMatch(t, actual, expected)
}

func TestRouteIndices(t *testing.T) {
	in := `
receiver: 'notify-def'

routes:
- match:
    owner: 'team-A'

  receiver: 'notify-A'

  routes:
  - match:
      env: 'testing'

    receiver: 'notify-testing'
    group_by: [...]

  - match:
      env: "production"

    receiver: 'notify-productionA'
    group_wait: 1m

    continue: true

  - match_re:
      env: "produ.*"
      job: ".*"

    receiver: 'notify-productionB'
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 1h
    group_by: ['job']

- match_re:
    owner: 'team-(B|C)'

  group_by: ['foo', 'bar']
  group_wait: 2m
  receiver: 'notify-BC'

- match:
    group_by: 'role'
  group_by: ['role']

  routes:
  - match:
      env: 'testing'
    receiver: 'notify-testing'
    routes:
    - match:
        wait: 'long'
      group_wait: 2m
`

	var ctree config.Route
	if err := yaml.UnmarshalStrict([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}
	tree := NewRoute(&ctree, nil)

	// Collect all indices
	var indices []int
	var totalNodes int
	tree.Walk(func(r *Route) {
		indices = append(indices, r.Idx)
		totalNodes++
	})

	// All indices are unique
	seenIndices := make(map[int]bool)
	for _, idx := range indices {
		require.False(t, seenIndices[idx], "Index %d appears more than once", idx)
		seenIndices[idx] = true
	}

	// Root index equals total nodes - 1
	require.Equal(t, totalNodes-1, tree.Idx, "Root index should equal total nodes - 1")

	// All indices are in range [0, totalNodes)
	for _, idx := range indices {
		require.GreaterOrEqual(t, idx, 0, "Index should be >= 0")
		require.Less(t, idx, totalNodes, "Index should be < total nodes")
	}
}
