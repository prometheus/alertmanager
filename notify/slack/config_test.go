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

package slack

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestLoadConfiguration(t *testing.T) {
	tests := []struct {
		in       string
		expected Config
	}{
		{
			in: `
color: green
username: mark
channel: engineering
title_link: http://example.com/
image_url: https://example.com/logo.png
`,
			expected: Config{
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
			expected: Config{
				Color: "green", Username: "mark", Channel: "alerts",
				MrkdwnIn: []string{"pretext", "text"}, TitleLink: "http://example.com/alert1",
			},
		},
	}
	for _, rt := range tests {
		var cfg Config
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

func TestSlackAuthMethodConfigValidation(t *testing.T) {
	tests := []struct {
		in          string
		expectedErr string
	}{
		{
			in: `
api_url: 'https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX'
api_url_file: /slack_url
`,
			expectedErr: "at most one of api_url & api_url_file must be configured",
		},
		{
			in: `
app_token: 'xoxb-1234-abcdefgh'
app_token_file: /slack_app_token
`,
			expectedErr: "at most one of app_token & app_token_file must be configured",
		},
		{
			in: `
app_token: 'xoxb-1234-abcdefgh'
api_url: 'https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX'
`,
			expectedErr: "at most one of api_url/api_url_file & app_token/app_token_file must be configured",
		},
	}

	for _, rt := range tests {
		var cfg Config
		err := yaml.UnmarshalStrict([]byte(rt.in), &cfg)

		// Check if an error occurred when it was NOT expected to.
		if rt.expectedErr == "" && err != nil {
			t.Fatalf("\nerror returned when none expected, error:\n%v", err)
		}
		// Check that an error occurred if one was expected to.
		if rt.expectedErr != "" && err == nil {
			t.Fatalf("\nno error returned, expected:\n%v", rt.expectedErr)
		}
		// Check that the error that occurred was what was expected.
		if err != nil && err.Error() != rt.expectedErr {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expectedErr, err.Error())
		}
	}
}

// TestValidateMessageStrategy is the strategy-framework analogue of the
// `update_message: true` + no `api_url` regression case once carried by
// TestSlackAuthMethodConfigValidation: a config that asks to update or
// thread but supplies no api_url at any level must surface a validation
// error from ValidateMessageStrategy without panicking.
func TestValidateMessageStrategy(t *testing.T) {
	tests := []struct {
		in          string
		expectedErr string
	}{
		{
			in: `
message_strategy: update
`,
			expectedErr: `message_strategy "update" requires api_url or api_url_file`,
		},
		{
			in: `
message_strategy: thread
`,
			expectedErr: `message_strategy "thread" requires api_url or api_url_file`,
		},
	}

	for _, rt := range tests {
		var cfg Config
		if err := yaml.UnmarshalStrict([]byte(rt.in), &cfg); err != nil {
			t.Fatalf("\nunexpected unmarshal error: %v", err)
		}
		err := cfg.ValidateMessageStrategy()
		if err == nil {
			t.Fatalf("\nno error returned, expected:\n%v", rt.expectedErr)
		}
		if err.Error() != rt.expectedErr {
			t.Errorf("\nexpected:\n%v\ngot:\n%v", rt.expectedErr, err.Error())
		}
	}
}

func TestFieldConfigValidation(t *testing.T) {
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
		var cfg Config
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

func TestFieldConfigUnmarshaling(t *testing.T) {
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
	expected := []*Field{
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

	var cfg Config
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

func TestActionsValidation(t *testing.T) {
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
	expected := []*Action{
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
			ConfirmField: &ConfirmationField{
				Title:       "please confirm",
				Text:        "are you sure?",
				OkText:      "yes",
				DismissText: "no",
			},
		},
	}

	var cfg Config
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
