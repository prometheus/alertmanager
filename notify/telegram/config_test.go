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

package telegram

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestTelegramConfiguration(t *testing.T) {
	tc := []struct {
		name     string
		in       string
		expected error
	}{
		{
			name: "with both bot_token & bot_token_file - it fails",
			in: `
bot_token: xyz
bot_token_file: /file
`,
			expected: errors.New("at most one of bot_token & bot_token_file must be configured"),
		},
		{
			name: "with bot_token and chat_id set - it succeeds",
			in: `
bot_token: xyz
chat_id: 123
`,
		},
		{
			name: "with bot_token, chat_id and message_thread_id set - it succeeds",
			in: `
bot_token: xyz
chat_id: 123
message_thread_id: 456
`,
		},
		{
			name: "with bot_token_file and chat_id set - it succeeds",
			in: `
bot_token_file: /file
chat_id: 123
`,
		},
		{
			name: "with bot_token_file and chat_id_file set - it succeeds",
			in: `
bot_token_file: /file
chat_id_file: /chat_id_file
`,
		},
		{
			name: "with no chat_id set - it fails",
			in: `
bot_token: xyz
`,
			expected: errors.New("missing chat_id or chat_id_file on telegram_config"),
		},
		{
			name: "with both chat_id and chat_id_file - it fails",
			in: `
bot_token: xyz
chat_id: 123
chat_id_file: /file
`,
			expected: errors.New("at most one of chat_id & chat_id_file must be configured"),
		},
		{
			name: "with unknown parse_mode - it fails",
			in: `
bot_token: xyz
chat_id: 123
parse_mode: invalid
`,
			expected: errors.New("unknown parse_mode on telegram_config, must be Markdown, MarkdownV2, HTML or empty string"),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			var cfg TelegramConfig
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)

			require.Equal(t, tt.expected, err)
		})
	}
}
