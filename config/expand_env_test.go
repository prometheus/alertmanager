// Copyright 2021 Prometheus Team
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
	"os"
	"testing"
)

func TestOnlyResolveEnvironmentConfigIfEnabled(t *testing.T) {
	// Setting one env variable that will be ignored since RESOLVE_ENV_IN_CONFIG won't be set
	os.Setenv("USERNAME", "any")
	defer os.Unsetenv("USERNAME")

	config, err := LoadFile("testdata/conf.env-variables.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good-env-variables.yml", err)
	}

	if config.Global.SMTPAuthUsername != "$(USERNAME)" {
		t.Error(`An environment variable (smtp_auth_username: '$(USERNAME)') was resolved without having resolution enabled`)
	}
}

func TestWontFailOnMissingEnvironmentVariables(t *testing.T) {
	// Setting the resolve flag: RESOLVE_ENV_IN_CONFIG but no other env variables so nothing will be subsituted
	os.Setenv("RESOLVE_ENV_IN_CONFIG", "true")
	defer os.Unsetenv("RESOLVE_ENV_IN_CONFIG")

	config, err := LoadFile("testdata/conf.env-variables.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good-env-variables.yml", err)
	}

	if config.Global.SMTPAuthUsername != "$(USERNAME)" {
		t.Error(`An environment variable (smtp_auth_username: '$(USERNAME)') was resolved without having resolution enabled`)
	}
}

func TestResolveEnvironmentVariables(t *testing.T) {
	for env, value := range map[string]string{
		"RESOLVE_ENV_IN_CONFIG": "true",
		"EXAMPLE":               "example",
		"USERNAME":              "username",
		"PASSWORD":              "password",
		"RECEIVER_NAME":         "my_receiver",
	} {
		os.Setenv(env, value)
		defer os.Unsetenv(env)
	}

	config, err := LoadFile("testdata/conf.env-variables.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good-env-variables.yml", err)
	}

	if config.Receivers[0].Name != "my_receiver" {
		t.Error("$(RECEIVER_NAME) was not resolved")
	}

	if config.Global.SMTPFrom != "alertmanager@example.org" {
		t.Error("$(EXAMPLE) was not resolved")
	}

	if config.Global.SMTPAuthUsername != "username" {
		t.Error("$(USERNAME) was not resolved")
	}

	if config.Global.SMTPAuthPassword != "password" {
		t.Error("$(PASSWORD) was not resolved")
	}
}
