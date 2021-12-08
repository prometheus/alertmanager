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
	"strings"
	"testing"
)

const (
	AuthorizationCredentials = "theanswertothegreatquestionoflifetheuniverseandeverythingisfortytwo"
)

func TestInvalidHttpConfig(t *testing.T) {
	_, err := LoadHttpConfig("testdata/http_config.bad.yml")
	errMsg := `authorization type cannot be set to "basic", use "basic_auth" instead`
	if !strings.Contains(err.Error(), errMsg) {
		t.Errorf("Expected error for invalid HTTP client configuration to contain %q but got: %s", errMsg, err)
	}
}
func TestValidHttpConfig(t *testing.T) {

	cfg, err := LoadHttpConfig("testdata/http_config.good.yml")
	if err != nil {
		t.Fatalf("Error loading HTTP client config: %v", err)
	}

	proxyURL := "http://remote.host"
	if cfg.ProxyURL.String() != proxyURL {
		t.Fatalf("Expected proxy_url is %q but got: %s", proxyURL, cfg.ProxyURL.String())
	}

	bearerToken, err := FetchBearerToken(cfg.Authorization)
	if err != nil {
		t.Fatalf("failed to fetch bearer token: %v", err)
	}

	if bearerToken != AuthorizationCredentials {
		t.Fatalf("Expected bearer token is %q but got: %s", AuthorizationCredentials, bearerToken)
	}
}

func TestValidBearerTokenFileHttpConfig(t *testing.T) {

	cfg, err := LoadHttpConfig("testdata/http_config.bearer-token-file.good.yml")
	if err != nil {
		t.Fatalf("Error loading HTTP client config: %v", err)
	}

	bearerToken, err := FetchBearerToken(cfg.Authorization)
	if err != nil {
		t.Fatalf("failed to fetch bearer token: %v", err)
	}

	if bearerToken != AuthorizationCredentials {
		t.Fatalf("Expected bearer token is %q but got: %s", AuthorizationCredentials, bearerToken)
	}
}

func TestFetchBasicAuthPassword(t *testing.T) {

	cfg, err := LoadHttpConfig("testdata/http_config.basic_auth.good.yml")
	if err != nil {
		t.Fatalf("Error loading HTTP client config: %v", err)
	}

	if cfg.BasicAuth.Username != "user" {
		t.Fatalf("Expected username is %q but got: %s", "user", cfg.BasicAuth.Username)
	}

	password, _ := FetchBasicAuthPassword(cfg.BasicAuth)
	if password != "password" {
		t.Fatalf("Expected password is %q but got: %s", "password", password)
	}
}
