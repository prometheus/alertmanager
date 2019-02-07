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
	"strings"
	"testing"

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

func TestPagerdutyRoutingKeyIsPresent(t *testing.T) {
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
}

func TestPagerdutyServiceKeyIsPresent(t *testing.T) {
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
}

func TestPagerdutyDetails(t *testing.T) {

	var tests = []struct {
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

func TestHipchatRoomIDIsPresent(t *testing.T) {
	in := `
room_id: ''
`
	var cfg HipchatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing room id in Hipchat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestWebhookURLIsPresent(t *testing.T) {
	in := `{}`
	var cfg WebhookConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing URL in webhook config"

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

func TestWebhookPasswordIsObsfucated(t *testing.T) {
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

func TestWechatAPIKeyIsPresent(t *testing.T) {
	in := `
api_secret: ''
`
	var cfg WechatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing Wechat APISecret in Wechat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}
func TestWechatCorpIDIsPresent(t *testing.T) {
	in := `
api_secret: 'api_secret'
corp_id: ''
`
	var cfg WechatConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "missing Wechat CorpID in Wechat config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestVictorOpsRoutingKeyIsPresent(t *testing.T) {
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
}

func TestVictorOpsCustomFieldsValidation(t *testing.T) {
	in := `
routing_key: 'test'
custom_fields:
  entity_state: 'state_message'
`
	var cfg VictorOpsConfig
	err := yaml.UnmarshalStrict([]byte(in), &cfg)

	expected := "VictorOps config contains custom field entity_state which cannot be used as it conflicts with the fixed/static fields"

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

	expected := "missing user key in Pushover config"

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

	expected := "missing token in Pushover config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestSlackFieldConfigValidation(t *testing.T) {
	var tests = []struct {
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

func TestSlackFieldConfigUnmarshalling(t *testing.T) {
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
		&SlackField{
			Title: "first",
			Value: "hello",
			Short: newBoolPointer(true),
		},
		&SlackField{
			Title: "second",
			Value: "world",
			Short: nil,
		},
		&SlackField{
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

func newBoolPointer(b bool) *bool {
	return &b
}
