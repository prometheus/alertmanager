// Copyright 2016 Prometheus Team
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
	"encoding/json"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

func TestSecretTemplURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid http URL",
			input:       `"http://example.com/webhook"`,
			expectError: false,
		},
		{
			name:        "invalid URL missing scheme",
			input:       `"example.com/webhook"`,
			expectError: true,
			errorMsg:    "unsupported scheme",
		},
		{
			name:        "invalid URL unsupported scheme",
			input:       `"ftp://example.com/webhook"`,
			expectError: true,
			errorMsg:    "unsupported scheme",
		},
		{
			name:        "templated URL is not validated",
			input:       `"http://example.com/{{ .GroupLabels.alertname }}"`,
			expectError: false,
		},
		{
			name:        "invalid URL with template is not validated",
			input:       `"not-a-url-{{ .GroupLabels.alertname }}"`,
			expectError: false,
		},
		{
			name:        "invalid template syntax",
			input:       `"http://example.com/{{ .Invalid"`,
			expectError: true,
			errorMsg:    "invalid template syntax",
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: false,
		},
		{
			name:        "secret token",
			input:       `"<secret>"`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var u amcommoncfg.SecretTemplateURL
			err := yaml.Unmarshal([]byte(tc.input), &u)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSecretTemplURLMarshaling(t *testing.T) {
	t.Run("marshals to secret token by default", func(t *testing.T) {
		u := amcommoncfg.SecretTemplateURL("http://example.com/secret")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "<secret>\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `"<secret>"`, string(jsonOut))
	})

	t.Run("marshals actual value when MarshalSecretValue is true", func(t *testing.T) {
		commoncfg.MarshalSecretValue = true
		defer func() { commoncfg.MarshalSecretValue = false }()

		u := amcommoncfg.SecretTemplateURL("http://example.com/secret")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "http://example.com/secret\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `"http://example.com/secret"`, string(jsonOut))
	})

	t.Run("empty URL marshals to empty", func(t *testing.T) {
		u := amcommoncfg.SecretTemplateURL("")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "null\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `""`, string(jsonOut))
	})
}
