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
    group_wait: 10m
    group_by: ['job']


- match_re:
    owner: '^team-(B|C)$'

  group_by: ['foo', 'bar']
  group_wait: 2m
  send_to: 'notify-BC'
`

	var ctree config.Route
	if err := yaml.Unmarshal([]byte(in), &ctree); err != nil {
		t.Fatal(err)
	}

	tree := NewRoute(&ctree)

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
					SendTo:  "notify-A",
					GroupBy: lset(),
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
					SendTo:  "notify-A",
					GroupBy: lset(),
				},
			},
		},
		{
			input: model.LabelSet{
				"owner": "team-C",
			},
			result: []*RouteOpts{
				{
					SendTo:    "notify-BC",
					GroupBy:   lset("foo", "bar"),
					GroupWait: 2 * time.Minute,
					hasWait:   true,
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
					SendTo:  "notify-testing",
					GroupBy: lset(),
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
					SendTo:    "notify-productionA",
					GroupBy:   lset(),
					GroupWait: 1 * time.Minute,
					hasWait:   true,
				},
				{
					SendTo:    "notify-productionB",
					GroupBy:   lset("job"),
					GroupWait: 10 * time.Minute,
					hasWait:   true,
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
