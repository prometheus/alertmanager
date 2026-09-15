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
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestMarshalRegexpWithNilValue(t *testing.T) {
	r := &Regexp{}

	out, err := json.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "null", string(out))

	out, err = yaml.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "null\n", string(out))
}

func TestUnmarshalEmptyRegexp(t *testing.T) {
	b := []byte(`""`)

	{
		var re Regexp
		err := json.Unmarshal(b, &re)
		require.NoError(t, err)
		require.Equal(t, regexp.MustCompile("^(?:)$"), re.Regexp)
		require.Empty(t, re.Original)
	}

	{
		var re Regexp
		err := yaml.Unmarshal(b, &re)
		require.NoError(t, err)
		require.Equal(t, regexp.MustCompile("^(?:)$"), re.Regexp)
		require.Empty(t, re.Original)
	}
}

func TestUnmarshalNullRegexp(t *testing.T) {
	input := []byte(`null`)

	{
		var re Regexp
		err := json.Unmarshal(input, &re)
		require.NoError(t, err)
		require.Empty(t, re.Original)
	}

	{
		var re Regexp
		err := yaml.Unmarshal(input, &re) // Interestingly enough, unmarshalling `null` in YAML doesn't even call UnmarshalYAML.
		require.NoError(t, err)
		require.Nil(t, re.Regexp)
		require.Empty(t, re.Original)
	}
}

func TestMarshalEmptyMatchers(t *testing.T) {
	r := Matchers{}

	out, err := json.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "[]", string(out))

	out, err = yaml.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "[]\n", string(out))
}
