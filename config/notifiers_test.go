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
