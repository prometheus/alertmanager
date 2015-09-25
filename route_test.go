package manager

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
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

	var tree Route
	if err := yaml.Unmarshal([]byte(in), &tree); err != nil {
		t.Fatal(err)
	}

	lset := func(labels ...string) map[model.LabelName]struct{} {
		s := map[model.LabelName]struct{}{}
		for _, ls := range labels {
			s[model.LabelName(ls)] = struct{}{}
		}
		return s
	}

	gwait := func(d time.Duration) *time.Duration { return &d }

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
					groupWait: gwait(2 * time.Minute),
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
					groupWait: gwait(1 * time.Minute),
				},
				{
					SendTo:    "notify-productionB",
					GroupBy:   lset("job"),
					groupWait: gwait(10 * time.Minute),
				},
			},
		},
	}

	for _, test := range tests {
		matches := tree.Match(test.input)

		if !reflect.DeepEqual(matches, test.result) {
			t.Errorf("expected:\n%v\n\ngot:\n%v", test.result, matches)
		}
	}
}
