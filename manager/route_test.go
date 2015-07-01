package manager

import (
	"reflect"
	"testing"

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

- match_re:
    owner: '^team-(B|C)$'

  send_to: 'notify-BC'
`

	var tree Route
	if err := yaml.Unmarshal([]byte(in), &tree); err != nil {
		t.Fatal(err)
	}

	var emptySet = map[model.LabelName]struct{}{}

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
					GroupBy: emptySet,
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
