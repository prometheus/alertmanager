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

package matrix

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestMatrixConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		in       string
		expected string
	}{
		{
			name: "with access_token and room_id - it succeeds",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
`,
		},
		{
			name: "with a room ID from a room version before 12 - it succeeds",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
room_id: '!abc123:example.org'
`,
		},
		{
			name: "with access_token_file, a templated room_id and m.notice - it succeeds",
			in: `
homeserver_url: https://matrix.example.org
access_token_file: /token
room_id: '{{ .CommonLabels.room }}'
msgtype: m.notice
`,
		},
		{
			name: "without homeserver_url - it fails",
			in: `
access_token: secret
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
`,
			expected: "missing homeserver_url on matrix_config",
		},
		{
			name: "without a token - it fails",
			in: `
homeserver_url: https://matrix.example.org
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
`,
			expected: "missing access_token or access_token_file on matrix_config",
		},
		{
			name: "with both access_token and access_token_file - it fails",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
access_token_file: /token
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
`,
			expected: "at most one of access_token & access_token_file must be configured",
		},
		{
			name: "without room_id - it fails",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
`,
			expected: "missing room_id on matrix_config",
		},
		{
			name: "with a room alias - it fails",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
room_id: '#alerts:example.org'
`,
			expected: "room_id on matrix_config must be a room ID such as !deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc, not a room alias",
		},
		{
			name: "with an unknown msgtype - it fails",
			in: `
homeserver_url: https://matrix.example.org
access_token: secret
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
msgtype: m.emote
`,
			expected: "msgtype on matrix_config must be m.text or m.notice",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cfg MatrixConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if tc.expected != "" {
				require.EqualError(t, err, tc.expected)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMatrixConfigurationDefaults(t *testing.T) {
	t.Parallel()
	var cfg MatrixConfig
	require.NoError(t, yaml.UnmarshalStrict([]byte(`
homeserver_url: https://matrix.example.org
access_token: secret
room_id: '!deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc'
`), &cfg))
	require.True(t, cfg.SendResolved())
	require.Equal(t, `{{ template "matrix.default.message" . }}`, cfg.Message)
	require.Empty(t, cfg.HTMLMessage)
	require.Equal(t, MsgTypeText, cfg.MsgType)
}
