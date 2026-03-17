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
