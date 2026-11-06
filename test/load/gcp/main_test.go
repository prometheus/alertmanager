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

package main

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/config"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/stretchr/testify/require"
)

func TestLoadtestInstanceCount(t *testing.T) {
	t.Parallel()

	project, err := workspace.LoadProject("Pulumi.yaml")
	require.NoError(t, err)
	stack, err := workspace.LoadProjectStack(nil, project, "Pulumi.loadtest.yaml")
	require.NoError(t, err)

	value, ok := stack.Config[config.MustMakeKey(string(project.Name), "instanceCount")]
	require.True(t, ok, "instanceCount must use the project namespace")
	count, err := value.Value(nil)
	require.NoError(t, err)
	got, err := parseInstanceCount(count)
	require.NoError(t, err)
	require.Equal(t, 3, got)
}

func TestParseInstanceCount(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		value   string
		want    int
		wantErr string
	}{
		{name: "default", value: "", want: 1},
		{name: "minimum", value: "1", want: 1},
		{name: "maximum", value: "3", want: 3},
		{name: "zero", value: "0", wantErr: "between 1 and 3"},
		{name: "above maximum", value: "4", wantErr: "between 1 and 3"},
		{name: "not an integer", value: "many", wantErr: "must be an integer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseInstanceCount(tt.value)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
