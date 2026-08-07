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

package gotify

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestGotifyConfig_UnmarshalYAML(t *testing.T) {
	test := []struct {
		name     string
		in       string
		expected error
	}{
		{
			name: "with url and token - successful run",
			in: `
url: http://localhost:3000
token: 00000000-0000-0000-0000-0000000000001
`,
		}, {
			name: "with url_file and token_file - successful run",
			in: `
url_file: /path/to/file
token_file: /path/to/token
`,
		}, {
			name: "with url and token_file - successful run",
			in: `
url: http://localhost:3000
token_file: /path/to/token
`,
		}, {
			name: "with url_file and token - successful run",
			in: `
url_file: /path/to/file
token: 00000000-0000-0000-0000-0000000000001
`,
		}, {
			name: "missing url and url_file, token provided - expected error missing url or url_file",
			in: `
token: 00000000-0000-0000-0000-0000000000001
`,
			expected: errors.New("one of url or url_file must be configured"),
		}, {
			name: "missing token and token_file, url provided - expected error missing token or token_file",
			in: `
url: http://localhost:3000
`,
			expected: errors.New("one of token or token_file must be configured"),
		}, {
			name: "url and url_file provided - expected error at most one of url & url_file",
			in: `
url: http://localhost:3000
url_file: /path/to/file
`,
			expected: errors.New("at most one of url & url_file must be configured"),
		}, {
			name: "token and token_file provided - expected error at most one of token & token_file",
			in: `
url: http://localhost:3000
token: 00000000-0000-0000-0000-0000000000001
token_file: /path/to/token
`,
			expected: errors.New("at most one of token & token_file must be configured"),
		}, {
			name: "empty content type - should default to text/plain",
			in: `
url: http://localhost:3000
token: 00000000-0000-0000-0000-0000000000001
content_type: ""
`,
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			var cfg GotifyConfig
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)
			require.Equal(t, tt.expected, err)
		})
	}
}
