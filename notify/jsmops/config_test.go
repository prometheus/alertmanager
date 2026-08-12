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

package jsmops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestJSMOpsConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		errMsg string
	}{
		{
			name: "valid configuration",
			in: `cloud_id: my-cloud-id
responders:
- id: foo
  type: scheDule
- name: bar
  type: teams
- username: fred
  type: USER
`,
		},
		{
			name: "missing cloud_id",
			in: `responders:
- id: foo
  type: team
`,
			errMsg: "missing cloud_id in jsmops_config",
		},
		{
			name: "invalid responder type",
			in: `cloud_id: my-cloud-id
responders:
- id: foo
  type: wrong
`,
			errMsg: "does not match valid options",
		},
		{
			name: "missing responder field",
			in: `cloud_id: my-cloud-id
responders:
- type: schedule
`,
			errMsg: "has to have at least one of id, username or name specified",
		},
		{
			name: "valid responder type template",
			in: `cloud_id: my-cloud-id
responders:
- id: foo
  type: "{{/* valid comment */}}team"
`,
		},
		{
			name: "invalid responder type template",
			in: `cloud_id: my-cloud-id
responders:
- id: foo
  type: "{{/* invalid comment }}team"
`,
			errMsg: "contains invalid template syntax",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg JSMOpsConfig

			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if tc.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestJSMOpsTypeMatcher(t *testing.T) {
	good := []string{"team", "user", "escalation", "schedule"}
	for _, g := range good {
		require.True(t, jsmopsTypeMatcher.MatchString(g), "expected %q to match", g)
	}

	bad := []string{"0user", "team1", "2escalation3", "sche4dule", "User", "TEAM"}
	for _, b := range bad {
		require.False(t, jsmopsTypeMatcher.MatchString(b), "expected %q not to match", b)
	}
}
