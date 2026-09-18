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

package victorops

import (
	"testing"

	"gopkg.in/yaml.v2"
)

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
