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

package common

import (
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

func mustUnmarshalInhibitRule(t *testing.T, input string) InhibitRule {
	t.Helper()
	var r InhibitRule
	err := yaml.Unmarshal([]byte(input), &r)
	require.NoError(t, err)
	return r
}

func unmarshalInhibitRule(input string) (InhibitRule, error) {
	var r InhibitRule
	err := yaml.Unmarshal([]byte(input), &r)
	return r, err
}

const inhibitRuleEqualYAML = `
source_matchers: ['foo=bar']
target_matchers: ['bar=baz']
equal: ['qux', 'corge']
`

const inhibitRuleEqualUTF8YAML = `
source_matchers: ['foo=bar']
target_matchers: ['bar=baz']
equal: ['qux🙂', 'corge']
`

func TestInhibitRuleEqual(t *testing.T) {
	r := mustUnmarshalInhibitRule(t, inhibitRuleEqualYAML)

	// The inhibition rule should have the expected equal labels.
	require.Equal(t, []string{"qux", "corge"}, r.Equal)

	// Should not be able to unmarshal configuration with UTF-8 in equals list.
	_, err := unmarshalInhibitRule(inhibitRuleEqualUTF8YAML)
	require.Error(t, err)
	require.Equal(t, "invalid label name \"qux🙂\" in equal list", err.Error())

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	r = mustUnmarshalInhibitRule(t, inhibitRuleEqualYAML)

	// The inhibition rule should have the expected equal labels.
	require.Equal(t, []string{"qux", "corge"}, r.Equal)

	// Should also be able to unmarshal configuration with UTF-8 in equals list.
	r = mustUnmarshalInhibitRule(t, inhibitRuleEqualUTF8YAML)

	// The inhibition rule should have the expected equal labels.
	require.Equal(t, []string{"qux🙂", "corge"}, r.Equal)
}

func TestInhibitRuleSourcesValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "sources cannot be combined with legacy fields",
			input: `
sources:
  - matchers: ['foo=bar']
source_matchers: ['baz=qux']
target_matchers: ['x=y']
`,
			wantErr: "sources cannot be combined with source_match, source_match_re, source_matchers, or equal",
		},
		{
			name: "source with empty matchers",
			input: `
sources:
  - matchers: []
target_matchers: ['x=y']
`,
			wantErr: "source 0: matchers must not be empty",
		},
		{
			name: "source where all matchers match empty string",
			input: `
sources:
  - matchers: ['foo=']
target_matchers: ['x=y']
`,
			wantErr: "source 0: at least one matcher must not match the empty string",
		},
		{
			name: "source with invalid equal label name",
			input: `
sources:
  - matchers: ['foo=bar']
    equal: ['invalid🙂']
target_matchers: ['x=y']
`,
			wantErr: "invalid label name \"invalid🙂\" in source equal list",
		},
		{
			name: "valid sources config",
			input: `
sources:
  - matchers: ['foo=bar']
    equal: ['cluster']
  - matchers: ['baz=qux']
    equal: ['severity']
target_matchers: ['x=y']
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unmarshalInhibitRule(tc.input)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
