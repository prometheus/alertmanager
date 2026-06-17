// Copyright 2018 Prometheus Team
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

package config

import (
	"errors"
	"net/mail"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestEmailToIsPresent(t *testing.T) {
	in := `
to: ''
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing to address in email config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestEmailHeadersCollision(t *testing.T) {
	in := `
to: 'to@email.com'
headers:
  Subject: 'Alert'
  sUbject: 'New Alert'
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "duplicate header \"Subject\" in email config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestEmailToAllowsMultipleAdresses(t *testing.T) {
	in := `
to: 'a@example.com, ,b@example.com,c@example.com'
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatal(err)
	}

	expected := []*mail.Address{
		{Address: "a@example.com"},
		{Address: "b@example.com"},
		{Address: "c@example.com"},
	}

	res, err := mail.ParseAddressList(cfg.To)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("expected %v, got %v", expected, res)
	}
}

func TestEmailDisallowMalformed(t *testing.T) {
	in := `
to: 'a@'
`
	var cfg EmailConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mail.ParseAddressList(cfg.To)
	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", "mail: no angle-addr")
	}
}

func TestPagerdutyTestRoutingKey(t *testing.T) {
	t.Run("error if no routing key or key file", func(t *testing.T) {
		in := `
routing_key: ''
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "missing service or routing key in PagerDuty config"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})

	t.Run("error if both routing key and key file", func(t *testing.T) {
		in := `
routing_key: 'xyz'
routing_key_file: 'xyz'
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "at most one of routing_key & routing_key_file must be configured"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})
}

func TestPagerdutyServiceKey(t *testing.T) {
	t.Run("error if no service key or key file", func(t *testing.T) {
		in := `
service_key: ''
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "missing service or routing key in PagerDuty config"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})

	t.Run("error if both service key and key file", func(t *testing.T) {
		in := `
service_key: 'xyz'
service_key_file: 'xyz'
`
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "at most one of service_key & service_key_file must be configured"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})
}

func TestPagerdutyDetails(t *testing.T) {
	tests := []struct {
		in      string
		checkFn func(map[string]any)
	}{
		{
			in: `
routing_key: 'xyz'
`,
			checkFn: func(d map[string]any) {
				if len(d) != 4 {
					t.Errorf("expected 4 items, got: %d", len(d))
				}
			},
		},
		{
			in: `
routing_key: 'xyz'
details:
  key1: val1
`,
			checkFn: func(d map[string]any) {
				if len(d) != 5 {
					t.Errorf("expected 5 items, got: %d", len(d))
				}
			},
		},
		{
			in: `
routing_key: 'xyz'
details:
  key1: val1
  key2: val2
  firing: firing
`,
			checkFn: func(d map[string]any) {
				if len(d) != 6 {
					t.Errorf("expected 6 items, got: %d", len(d))
				}
			},
		},
	}
	for _, tc := range tests {
		var cfg PagerdutyConfig
		err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
		if err != nil {
			t.Errorf("expected no error, got:%v", err)
		}

		if tc.checkFn != nil {
			tc.checkFn(cfg.Details)
		}
	}
}

func TestPagerDutySource(t *testing.T) {
	for _, tc := range []struct {
		title string
		in    string

		expectedSource string
	}{
		{
			title: "check source field is backward compatible",
			in: `
routing_key: 'xyz'
client: 'alert-manager-client'
`,
			expectedSource: "alert-manager-client",
		},
		{
			title: "check source field is set",
			in: `
routing_key: 'xyz'
client: 'alert-manager-client'
source: 'alert-manager-source'
`,
			expectedSource: "alert-manager-source",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			var cfg PagerdutyConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			require.NoError(t, err)
			require.Equal(t, tc.expectedSource, cfg.Source)
		})
	}
}

func TestVictorOpsConfiguration(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		in := `
routing_key: test
api_key_file: /global_file
`
		var cfg VictorOpsConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)
		if err != nil {
			t.Fatalf("no error was expected:\n%v", err)
		}
	})

	t.Run("routing key is missing", func(t *testing.T) {
		in := `
routing_key: ''
`
		var cfg VictorOpsConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "missing Routing key in VictorOps config"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})

	t.Run("api_key and api_key_file both defined", func(t *testing.T) {
		in := `
routing_key: test
api_key: xyz
api_key_file: /global_file
`
		var cfg VictorOpsConfig
		err := yaml.UnmarshalStrict([]byte(in), &cfg)

		expected := "at most one of api_key & api_key_file must be configured"

		if err == nil {
			t.Fatalf("no error returned, expected:\n%v", expected)
		}
		if err.Error() != expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
		}
	})
}

func TestVictorOpsCustomFieldsValidation(t *testing.T) {
	in := `
routing_key: 'test'
custom_fields:
  entity_state: 'state_message'
`
	var cfg VictorOpsConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "victorOps config contains custom field entity_state which cannot be used as it conflicts with the fixed/static fields"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}

	in = `
routing_key: 'test'
custom_fields:
  my_special_field: 'special_label'
`

	err = yaml.UnmarshalStrict([]byte(in), &cfg)

	expected = "special_label"

	if err != nil {
		t.Fatalf("Unexpected error returned, got:\n%v", err.Error())
	}

	val, ok := cfg.CustomFields["my_special_field"]

	if !ok {
		t.Fatalf("Expected Custom Field to have value %v set, field is empty", expected)
	}
	if val != expected {
		t.Errorf("\nexpected custom field my_special_field value:\n%v\ngot:\n%v", expected, val)
	}
}

func TestPushoverUserKeyIsPresent(t *testing.T) {
	in := `
user_key: ''
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "one of user_key or user_key_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverUserKeyOrUserKeyFile(t *testing.T) {
	in := `
user_key: 'user key'
user_key_file: /pushover/user_key
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of user_key & user_key_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverTokenIsPresent(t *testing.T) {
	in := `
user_key: '<user_key>'
token: ''
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "one of token or token_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverTokenOrTokenFile(t *testing.T) {
	in := `
token: 'pushover token'
token_file: /pushover/token
user_key: 'user key'
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of token & token_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestPushoverHTMLOrMonospace(t *testing.T) {
	in := `
token: 'pushover token'
user_key: 'user key'
html: true
monospace: true
`
	var cfg PushoverConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of monospace & html must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestOpsgenieTypeMatcher(t *testing.T) {
	good := []string{"team", "user", "escalation", "schedule"}
	for _, g := range good {
		if !opsgenieTypeMatcher.MatchString(g) {
			t.Fatalf("failed to match with %s", g)
		}
	}
	bad := []string{"0user", "team1", "2escalation3", "sche4dule", "User", "TEAM"}
	for _, b := range bad {
		if opsgenieTypeMatcher.MatchString(b) {
			t.Errorf("mistakenly match with %s", b)
		}
	}
}

func TestOpsGenieConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string

		err bool
	}{
		{
			name: "valid configuration",
			in: `api_key: xyz
responders:
- id: foo
  type: scheDule
- name: bar
  type: teams
- username: fred
  type: USER
api_url: http://example.com
`,
		},
		{
			name: "api_key and api_key_file both defined",
			in: `api_key: xyz
api_key_file: xyz
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "invalid responder type",
			in: `api_key: xyz
responders:
- id: foo
  type: wrong
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "missing responder field",
			in: `api_key: xyz
responders:
- type: schedule
api_url: http://example.com
`,
			err: true,
		},
		{
			name: "valid responder type template",
			in: `api_key: xyz
responders:
- id: foo
  type: "{{/* valid comment */}}team"
api_url: http://example.com
`,
		},
		{
			name: "invalid responder type template",
			in: `api_key: xyz
responders:
- id: foo
  type: "{{/* invalid comment }}team"
api_url: http://example.com
`,
			err: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg OpsGenieConfig

			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestSNS(t *testing.T) {
	for _, tc := range []struct {
		in  string
		err bool
	}{
		{
			// Valid configuration without sigv4.
			in:  `target_arn: target`,
			err: false,
		},
		{
			// Valid configuration without sigv4.
			in:  `topic_arn: topic`,
			err: false,
		},
		{
			// Valid configuration with sigv4.
			in: `phone_number: phone
sigv4:
    access_key: abc
    secret_key: abc
`,
			err: false,
		},
		{
			// at most one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in: `topic_arn: topic
target_arn: target
`,
			err: true,
		},
		{
			// at most one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in: `topic_arn: topic
phone_number: phone
`,
			err: true,
		},
		{
			// one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in:  "{}",
			err: true,
		},
		{
			// one of 'target_arn', 'topic_arn' or 'phone_number' must be provided with sigv4.
			in: `sigv4:
    access_key: abc
    secret_key: abc
`,
			err: true,
		},
		{
			// 'secret_key' must be provided with 'access_key'.
			in: `topic_arn: topic
sigv4:
    access_key: abc
`,
			err: true,
		},
		{
			// 'access_key' must be provided with 'secret_key'.
			in: `topic_arn: topic
sigv4:
    secret_key: abc
`,
			err: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			var cfg SNSConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if err != nil {
				if !tc.err {
					t.Errorf("expecting no error, got %q", err)
				}
				return
			}

			if tc.err {
				t.Logf("%#v", cfg)
				t.Error("expecting error, got none")
			}
		})
	}
}

func TestWeChatTypeMatcher(t *testing.T) {
	good := []string{"text", "markdown"}
	for _, g := range good {
		if !wechatTypeMatcher.MatchString(g) {
			t.Fatalf("failed to match with %s", g)
		}
	}
	bad := []string{"TEXT", "MarkDOwn"}
	for _, b := range bad {
		if wechatTypeMatcher.MatchString(b) {
			t.Errorf("mistakenly match with %s", b)
		}
	}
}

func TestWebexConfiguration(t *testing.T) {
	tc := []struct {
		name string

		in       string
		expected error
	}{
		{
			name: "with no room_id - it fails",
			in: `
message: xyz123
`,
			expected: errors.New("missing room_id on webex_config"),
		},
		{
			name: "with room_id and http_config.authorization set - it succeeds",
			in: `
room_id: 2
http_config:
  authorization:
    credentials: "xxxyyyzz"
`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			var cfg WebexConfig
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)

			require.Equal(t, tt.expected, err)
		})
	}
}

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

func TestEmailConfig_UnmarshalYAML(t *testing.T) {
	testConfig := []struct {
		name     string
		in       string
		expected error
	}{
		{
			name: "with basic config - it succeeds",
			in: `
to: foobar@example.com
headers: {X-Custom-Header: CustomValue}
`,
		},
		{
			name: "with empty to address - it fails",
			in: `
to: ''`,
			expected: errors.New("missing to address in email config"),
		},
		{
			name: "with correct threading - it succeeds",
			in: `
to: foobar@example.com
threading:
  enabled: true
  thread_by_date: daily
`,
		},
		{
			name: "with invalid threading - it fails",
			in: `
to: foobar@example.com
threading:
  enabled: true
  thread_by_date: weekly
`,
			expected: errors.New("threading.thread_by_date must be either 'none' or 'daily'"),
		},
		{
			name: "with duplicate headers - it failes",
			in: `
to: foobar@example.com
headers: {X-Custom-Header: CustomValue, X-CUSTOM-HEADER: AnotherValue}
`,
			expected: errors.New("duplicate header \"X-Custom-Header\" in email config"),
		},
	}

	for _, tt := range testConfig {
		t.Run(tt.name, func(t *testing.T) {
			var cfg EmailConfig
			err := yaml.UnmarshalStrict([]byte(tt.in), &cfg)

			require.Equal(t, tt.expected, err)
		})
	}
}
