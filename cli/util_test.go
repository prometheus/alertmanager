// Copyright 2023 Prometheus Team
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

	"github.com/stretchr/testify/require"
)

func TestParseMatchers(t *testing.T) {
	// Should not be able to parse lists of matchers using PromQL syntax.
	// If you want to do use, should put all matchers in a single string.
	m, err := parseMatchers([]string{"{foo=bar}"})
	require.EqualError(t, err, "bad matcher format: {foo=bar}")
	require.Len(t, m, 0)
}
