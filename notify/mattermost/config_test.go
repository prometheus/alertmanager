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

package mattermost

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestMattermostField_UnmarshalYAML(t *testing.T) {
	mf := []struct {
		name     string
		in       string
		expected error
	}{
		{
			name: "with title, value and short - it succeeds",
			in: `
title: some title
value: some value
short: true
`,
		},
		{
			name: "with title and value - it succeeds",
			in: `
title: some title
value: some value
`,
		},
		{
			name: "with no value - it fails",
			in: `
title: some title
`,
			expected: errors.New("missing value in Mattermost field configuration"),
		},
		{
			name: "with no title - it fails",
			in: `
value: some value
`,
			expected: errors.New("missing title in Mattermost field configuration"),
		},
	}

	for _, tt := range mf {
		t.Run(tt.name, func(t *testing.T) {
			var cfg MattermostField
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)

			require.Equal(t, tt.expected, err)
		})
	}
}

func TestMattermostConfig_UnmarshalYAML(t *testing.T) {
	mc := []struct {
		name     string
		in       string
		expected error
	}{
		{
			name: "with url and text - it succeeds",
			in: `
webhook_url: http://some.url
channel: some_channel
username: some_username
text: some text
`,
		},
		{
			name: "with url_file, attachments and props - it succeeds",
			in: `
webhook_url_file: /some/url.file
channel: some_channel
username: some_username
attachments:
- text: some text
props:
  card: some text
`,
		},
		{
			name: "with url and url_file - it fails",
			in: `
webhook_url: http://some.url
webhook_url_file: /some/url.file
channel: some_channel
username: some_username
attachments:
- text: some text
`,
			expected: errors.New("at most one of webhook_url & webhook_url_file must be configured"),
		},
		{
			name: "with text and attachments - it succeeds",
			in: `
webhook_url: http://some.url
channel: some_channel
username: some_username
text: some text
attachments:
- text: some text
`,
		},
	}

	for _, tt := range mc {
		t.Run(tt.name, func(t *testing.T) {
			var cfg MattermostConfig
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)

			require.Equal(t, tt.expected, err)
		})
	}
}

func TestMattermostConfig_APIMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		expected string
	}{
		{
			name: "api_url with channel_id and update_message - it succeeds",
			in: `
api_url: https://mattermost.example.com
channel_id: abc123
update_message: true
`,
		},
		{
			name: "api_url with channel_id only - it succeeds",
			in: `
api_url: https://mattermost.example.com
channel_id: abc123
`,
		},
		{
			name: "webhook_url only - it succeeds",
			in: `
webhook_url: https://mattermost.example.com/hooks/xxxxxxxxxxxxxxxxxxxxxxxxxx
`,
		},
		{
			name: "api_url without channel_id - it fails",
			in: `
api_url: https://mattermost.example.com
`,
			expected: "channel_id must be set when api_url is configured",
		},
		{
			name: "api_url with webhook_url - it fails",
			in: `
api_url: https://mattermost.example.com
channel_id: abc123
webhook_url: https://mattermost.example.com/hooks/xxxxxxxxxxxxxxxxxxxxxxxxxx
`,
			expected: "at most one of api_url & webhook_url/webhook_url_file must be configured",
		},
		{
			name: "api_url with webhook_url_file - it fails",
			in: `
api_url: https://mattermost.example.com
channel_id: abc123
webhook_url_file: /tmp/webhook
`,
			expected: "at most one of api_url & webhook_url/webhook_url_file must be configured",
		},
		{
			name: "update_message with webhook_url - it fails",
			in: `
webhook_url: https://mattermost.example.com/hooks/xxxxxxxxxxxxxxxxxxxxxxxxxx
update_message: true
`,
			expected: "update_message can only be used with api_url, webhook URLs do not support updates",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cfg MattermostConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if tc.expected == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expected)
		})
	}
}
