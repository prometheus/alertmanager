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
	"strings"
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
  subject: 'New Alert'
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
		checkFn func(map[string]string)
	}{
		{
			in: `
routing_key: 'xyz'
`,
			checkFn: func(d map[string]string) {
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
			checkFn: func(d map[string]string) {
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
			checkFn: func(d map[string]string) {
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

func TestWebhookURLIsPresent(t *testing.T) {
	in := `{}`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "one of url or url_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookURLOrURLFile(t *testing.T) {
	in := `
url: 'http://example.com'
url_file: 'http://example.com'
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of url & url_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookHttpConfigIsValid(t *testing.T) {
	in := `
url: 'http://example.com'
http_config:
  bearer_token: foo
  bearer_token_file: /tmp/bar
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "at most one of bearer_token & bearer_token_file must be configured"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookHttpConfigIsOptional(t *testing.T) {
	in := `
url: 'http://example.com'
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}
}

func TestWebhookPasswordIsObfuscated(t *testing.T) {
	in := `
url: 'http://example.com'
http_config:
  basic_auth:
    username: foo
    password: supersecret
`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}

	ycfg, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("no error expected, returned:\n%v", err.Error())
	}
	if strings.Contains(string(ycfg), "supersecret") {
		t.Errorf("Found password in the YAML cfg: %s\n", ycfg)
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

func TestLoadSlackConfiguration(t *testing.T) {
	tests := []struct {
		in       string
		expected SlackConfig
	}{
		{
			in: `
color: green
username: mark
channel: engineering
title_link: http://example.com/
image_url: https://example.com/logo.png
`,
			expected: SlackConfig{
				Color: "green", Username: "mark", Channel: "engineering",
				TitleLink: "http://example.com/",
				ImageURL:  "https://example.com/logo.png",
			},
		},
		{
			in: `
color: green
username: mark
channel: alerts
title_link: http://example.com/alert1
mrkdwn_in:
- pretext
- text
`,
			expected: SlackConfig{
				Color: "green", Username: "mark", Channel: "alerts",
				MrkdwnIn: []string{"pretext", "text"}, TitleLink: "http://example.com/alert1",
			},
		},
	}
	for _, rt := range tests {
		var cfg SlackConfig
		err := yaml.UnmarshalStrict([]byte(rt.in), &cfg)
		if err != nil {
			t.Fatalf("\nerror returned when none expected, error:\n%v", err)
		}
		if rt.expected.Color != cfg.Color {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.Color, cfg.Color)
		}
		if rt.expected.Username != cfg.Username {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.Username, cfg.Username)
		}
		if rt.expected.Channel != cfg.Channel {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.Channel, cfg.Channel)
		}
		if rt.expected.ThumbURL != cfg.ThumbURL {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.ThumbURL, cfg.ThumbURL)
		}
		if rt.expected.TitleLink != cfg.TitleLink {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.TitleLink, cfg.TitleLink)
		}
		if rt.expected.ImageURL != cfg.ImageURL {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.ImageURL, cfg.ImageURL)
		}
		if len(rt.expected.MrkdwnIn) != len(cfg.MrkdwnIn) {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected.MrkdwnIn, cfg.MrkdwnIn)
		}
		for i := range cfg.MrkdwnIn {
			if rt.expected.MrkdwnIn[i] != cfg.MrkdwnIn[i] {
				t.Errorf("\nexpected:\n%v\ngot:\n%v\nat index %v", rt.expected.MrkdwnIn[i], cfg.MrkdwnIn[i], i)
			}
		}
	}
}

func TestSlackFieldConfigValidation(t *testing.T) {
	tests := []struct {
		in       string
		expected string
	}{
		{
			in: `
fields:
- title: first
  value: hello
- title: second
`,
			expected: "missing value in Slack field configuration",
		},
		{
			in: `
fields:
- title: first
  value: hello
  short: true
- value: world
  short: true
`,
			expected: "missing title in Slack field configuration",
		},
		{
			in: `
fields:
- title: first
  value: hello
  short: true
- title: second
  value: world
`,
			expected: "",
		},
	}

	for _, rt := range tests {
		var cfg SlackConfig
		err := yaml.UnmarshalStrict([]byte(rt.in), &cfg)

		// Check if an error occurred when it was NOT expected to.
		if rt.expected == "" && err != nil {
			t.Fatalf("\nerror returned when none expected, error:\n%v", err)
		}
		// Check that an error occurred if one was expected to.
		if rt.expected != "" && err == nil {
			t.Fatalf("\nno error returned, expected:\n%v", rt.expected)
		}
		// Check that the error that occurred was what was expected.
		if err != nil && err.Error() != rt.expected {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expected, err.Error())
		}
	}
}

func TestSlackFieldConfigUnmarshaling(t *testing.T) {
	in := `
fields:
- title: first
  value: hello
  short: true
- title: second
  value: world
- title: third
  value: slack field test
  short: false
`
	expected := []*SlackField{
		{
			Title: "first",
			Value: "hello",
			Short: newBoolPointer(true),
		},
		{
			Title: "second",
			Value: "world",
			Short: nil,
		},
		{
			Title: "third",
			Value: "slack field test",
			Short: newBoolPointer(false),
		},
	}

	var cfg SlackConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatalf("\nerror returned when none expected, error:\n%v", err)
	}

	for index, field := range cfg.Fields {
		exp := expected[index]
		if field.Title != exp.Title {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Title, field.Title)
		}
		if field.Value != exp.Value {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Value, field.Value)
		}
		if exp.Short == nil && field.Short != nil {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Short, *field.Short)
		}
		if exp.Short != nil && field.Short == nil {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", *exp.Short, field.Short)
		}
		if exp.Short != nil && *exp.Short != *field.Short {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", *exp.Short, *field.Short)
		}
	}
}

func TestSlackActionsValidation(t *testing.T) {
	in := `
actions:
- type: button
  text: hello
  url: https://localhost
  style: danger
- type: button
  text: hello
  name: something
  style: default
  confirm:
    title: please confirm
    text: are you sure?
    ok_text: yes
    dismiss_text: no
`
	expected := []*SlackAction{
		{
			Type:  "button",
			Text:  "hello",
			URL:   "https://localhost",
			Style: "danger",
		},
		{
			Type:  "button",
			Text:  "hello",
			Name:  "something",
			Style: "default",
			ConfirmField: &SlackConfirmationField{
				Title:       "please confirm",
				Text:        "are you sure?",
				OkText:      "yes",
				DismissText: "no",
			},
		},
	}

	var cfg SlackConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)
	if err != nil {
		t.Fatalf("\nerror returned when none expected, error:\n%v", err)
	}

	for index, action := range cfg.Actions {
		exp := expected[index]
		if action.Type != exp.Type {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Type, action.Type)
		}
		if action.Text != exp.Text {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Text, action.Text)
		}
		if action.URL != exp.URL {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.URL, action.URL)
		}
		if action.Style != exp.Style {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Style, action.Style)
		}
		if action.Name != exp.Name {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Name, action.Name)
		}
		if action.Value != exp.Value {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.Value, action.Value)
		}
		if action.ConfirmField != nil && exp.ConfirmField == nil || action.ConfirmField == nil && exp.ConfirmField != nil {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.ConfirmField, action.ConfirmField)
		} else if action.ConfirmField != nil && exp.ConfirmField != nil {
			if action.ConfirmField.Title != exp.ConfirmField.Title {
				t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.ConfirmField.Title, action.ConfirmField.Title)
			}
			if action.ConfirmField.Text != exp.ConfirmField.Text {
				t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.ConfirmField.Text, action.ConfirmField.Text)
			}
			if action.ConfirmField.OkText != exp.ConfirmField.OkText {
				t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.ConfirmField.OkText, action.ConfirmField.OkText)
			}
			if action.ConfirmField.DismissText != exp.ConfirmField.DismissText {
				t.Errorf("\nexpected:\n%v\ngot:\n%v", exp.ConfirmField.DismissText, action.ConfirmField.DismissText)
			}
		}
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
			name: "with no bot_token & bot_token_file - it fails",
			in: `
bot_token: ''
bot_token_file: ''
`,
			expected: errors.New("missing bot_token or bot_token_file on telegram_config"),
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
			name: "with no chat_id set - it fails",
			in: `
bot_token: xyz
`,
			expected: errors.New("missing chat_id on telegram_config"),
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

func newBoolPointer(b bool) *bool {
	return &b
}
