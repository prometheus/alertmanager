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

package cli

import (
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		input string
		name  string
		value string
		err   bool
	}{
		{input: "summary=hello world", name: "summary", value: "hello world"},
		{input: `summary="hello world"`, name: "summary", value: "hello world"},
		{input: `description=Disk "data" is 95% full`, name: "description", value: `Disk "data" is 95% full`},
		{input: "runbook=https://example.com/x?a=b&c=d", name: "runbook", value: "https://example.com/x?a=b&c=d"},
		{input: "summary=", name: "summary", value: ""},
		{input: "summary!=x", err: true},
		{input: "summary", err: true},
		{input: `="no name"`, err: true},
	}

	for _, feature := range []string{"", featurecontrol.FeatureClassicMode, featurecontrol.FeatureUTF8StrictMode} {
		t.Run("features="+feature, func(t *testing.T) {
			ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), feature)
			require.NoError(t, err)
			compat.InitFromFlags(promslog.NewNopLogger(), ff)
			// compat.InitFromFlags mutates package-global state, restore the
			// default classic mode when done.
			t.Cleanup(func() {
				ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
				require.NoError(t, err)
				compat.InitFromFlags(promslog.NewNopLogger(), ff)
			})

			for _, test := range tests {
				name, value, err := parseAnnotation(test.input)
				if test.err {
					require.Error(t, err, "input %q", test.input)
					continue
				}
				require.NoError(t, err, "input %q", test.input)
				require.Equal(t, test.name, name, "input %q", test.input)
				require.Equal(t, test.value, value, "input %q", test.input)
			}
		})
	}
}
