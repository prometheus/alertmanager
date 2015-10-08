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
