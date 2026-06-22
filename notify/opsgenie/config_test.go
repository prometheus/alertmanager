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

package opsgenie

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestOpsGenieConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string

		err bool
	}{
		{
			name: "valid configuration",
			in: `api_key: xyz
responders:
- id: foo
  type: scheDule
- name: bar
  type: teams
- username: fred
  type: USER
api_url: http://example.com
`,
		},
		{
			name: "api_key and api_key_file both defined",
			in: `api_key: xyz
api_key_file: xyz
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "invalid responder type",
			in: `api_key: xyz
responders:
- id: foo
  type: wrong
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "missing responder field",
			in: `api_key: xyz
responders:
- type: schedule
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "valid responder type template",
			in: `api_key: xyz
responders:
- id: foo
  type: "{{/* valid comment */}}team"
api_url: http://example.com
`,
		},
		{
			name: "invalid responder type template",
			in: `api_key: xyz
responders:
- id: foo
  type: "{{/* invalid comment }}team"
api_url: http://example.com
`,
			err: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg OpsGenieConfig

			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestOpsgenieTypeMatcher(t *testing.T) {
	good := []string{"team", "user", "escalation", "schedule"}
	for _, g := range good {
		if !opsgenieTypeMatcher.MatchString(g) {
			t.Fatalf("failed to match with %s", g)
		}
	}
	bad := []string{"0user", "team1", "2escalation3", "sche4dule", "User", "TEAM"}
	for _, b := range bad {
		if opsgenieTypeMatcher.MatchString(b) {
			t.Errorf("mistakenly match with %s", b)
		}
	}
}
