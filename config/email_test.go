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

package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestEmailSMTPDefaults(t *testing.T) {
	cfg, err := Load(`
global:
  smtp_from: sender@example.com
  smtp_smarthost: localhost:587
  smtp_tls_config:
    server_name: relay.example.com
  smtp_force_implicit_tls: true
  smtp_auth_secret_file: global-secret
route:
  receiver: mail
receivers:
- name: mail
  email_configs:
  - to: omitted@example.com
  - to: null@example.com
    tls_config: null
    force_implicit_tls: null
  - to: override@example.com
    tls_config: {}
    force_implicit_tls: false
    auth_secret_file: receiver-secret
`)
	require.NoError(t, err)
	omitted, null, override := cfg.Receivers[0].EmailConfigs[0], cfg.Receivers[0].EmailConfigs[1], cfg.Receivers[0].EmailConfigs[2]
	for _, ec := range []*EmailConfig{omitted, null} {
		require.NotNil(t, ec.TLSConfig)
		require.Equal(t, "relay.example.com", ec.TLSConfig.ServerName)
		require.Equal(t, new(true), ec.ForceImplicitTLS)
		require.Equal(t, "global-secret", ec.AuthSecretFile)
	}
	require.NotNil(t, override.TLSConfig)
	require.Empty(t, override.TLSConfig.ServerName)
	require.Equal(t, new(false), override.ForceImplicitTLS)
	require.Equal(t, "receiver-secret", override.AuthSecretFile)
}

func TestEmailForceImplicitTLSDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want *bool
	}{
		{name: "omitted", in: `{"to":"mail@example.com"}`},
		{name: "null", in: `{"to":"mail@example.com","force_implicit_tls":null}`},
		{name: "false", in: `{"to":"mail@example.com","force_implicit_tls":false}`, want: new(false)},
		{name: "true", in: `{"to":"mail@example.com","force_implicit_tls":true}`, want: new(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fromYAML, fromJSON EmailConfig
			require.NoError(t, yaml.UnmarshalStrict([]byte(tc.in), &fromYAML))
			require.NoError(t, json.Unmarshal([]byte(tc.in), &fromJSON))
			require.Equal(t, tc.want, fromYAML.ForceImplicitTLS)
			require.Equal(t, tc.want, fromJSON.ForceImplicitTLS)
		})
	}
}

func TestEmailSMTPValidation(t *testing.T) {
	t.Run("reject null receiver entry", func(t *testing.T) {
		_, err := Load(`
route: {receiver: mail}
receivers:
- name: mail
  email_configs: [null]
`)
		require.EqualError(t, err, "missing email config")
	})
	t.Run("reject both global secret sources", func(t *testing.T) {
		_, err := Load(`
global:
  smtp_auth_secret: sample-secret
  smtp_auth_secret_file: secret-file
route: {receiver: mail}
receivers:
- name: mail
`)
		require.EqualError(t, err, "at most one of smtp_auth_secret & smtp_auth_secret_file must be configured")
	})
}

func TestEmailThreadingRejectsCustomHeaders(t *testing.T) {
	for _, header := range []string{"References", "In-Reply-To"} {
		t.Run(header, func(t *testing.T) {
			var cfg EmailConfig
			err := yaml.UnmarshalStrict([]byte(`
to: mail@example.com
threading: {enabled: true, thread_by_date: none}
headers: {`+header+`: custom}
`), &cfg)
			require.ErrorContains(t, err, "threading.enabled conflicts with custom "+header+" header")
		})
	}
}
