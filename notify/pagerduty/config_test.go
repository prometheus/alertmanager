// Copyright The Prometheus Authors
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

package pagerduty

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestPagerdutyTestRoutingKey(t *testing.T) {
	t.Run("error if no routing key or key file", func(t *testing.T) {
		in := `
routing_key: ''
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "missing service or routing key in PagerDuty config"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})

	t.Run("error if both routing key and key file", func(t *testing.T) {
		in := `
routing_key: 'xyz'
routing_key_file: 'xyz'
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "at most one of routing_key & routing_key_file must be configured"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})
}

func TestPagerdutyServiceKey(t *testing.T) {
	t.Run("error if no service key or key file", func(t *testing.T) {
		in := `
service_key: ''
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "missing service or routing key in PagerDuty config"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})

	t.Run("error if both service key and key file", func(t *testing.T) {
		in := `
service_key: 'xyz'
service_key_file: 'xyz'
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "at most one of service_key & service_key_file must be configured"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})
}

func TestPagerdutyDetails(t *testing.T) {
	tests := []struct {
		in      string
		checkFn func(map[string]any)
	}{
		{
			in: `
routing_key: 'xyz'
`,
			checkFn: func(d map[string]any) {
				if len(d) != 4 {
					t.Errorf("expected 4 items, got: %d", len(d))
				}
			},
		},
		{
			in: `
routing_key: 'xyz'
details:
  key1: val1
`,
			checkFn: func(d map[string]any) {
				if len(d) != 5 {
					t.Errorf("expected 5 items, got: %d", len(d))
				}
			},
		},
		{
			in: `
routing_key: 'xyz'
details:
  key1: val1
  key2: val2
  firing: firing
`,
			checkFn: func(d map[string]any) {
				if len(d) != 6 {
					t.Errorf("expected 6 items, got: %d", len(d))
				}
			},
		},
	}
	for _, tc := range tests {
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
		if err != nil {
			t.Errorf("expected no error, got:%v", err)
		}

		if tc.checkFn != nil {
			tc.checkFn(cfg.Details)
		}
	}
}

func TestPagerDutySource(t *testing.T) {
	for _, tc := range []struct {
		title string
		in    string

		expectedSource string
	}{
		{
			title: "check source field is backward compatible",
			in: `
routing_key: 'xyz'
client: 'alert-manager-client'
`,
			expectedSource: "alert-manager-client",
		},
		{
			title: "check source field is set",
			in: `
routing_key: 'xyz'
client: 'alert-manager-client'
source: 'alert-manager-source'
`,
			expectedSource: "alert-manager-source",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			var cfg PagerdutyConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			require.NoError(t, err)
			require.Equal(t, tc.expectedSource, cfg.Source)
		})
	}
}
