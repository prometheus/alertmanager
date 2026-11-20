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
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestWechatNoAPIURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.wechat-no-api-secret.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.wechat-no-api-url.yml", err)
	}
	if err.Error() != "no global Wechat Api Secret set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no global Wechat Api Secret set either inline or in a file", err.Error())
	}
}

func TestWechatBothAPIURLAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.wechat-both-file-and-secret.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.wechat-both-file-and-secret.yml", err)
	}
	if err.Error() != "at most one of wechat_api_secret & wechat_api_secret_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of wechat_api_secret & wechat_api_secret_file must be configured", err.Error())
	}
}

func TestWechatGlobalAPISecretFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.wechat-default-api-secret-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.wechat-default-api-secret-file.yml", err)
	}

	// no override
	firstConfig := conf.Receivers[0].WechatConfigs[0]
	if firstConfig.APISecretFile != "/global_file" || string(firstConfig.APISecret) != "" {
		t.Fatalf("Invalid Wechat API Secret file: %s\nExpected: %s", firstConfig.APISecretFile, "/global_file")
	}

	// override the file
	secondConfig := conf.Receivers[0].WechatConfigs[1]
	if secondConfig.APISecretFile != "/override_file" || string(secondConfig.APISecret) != "" {
		t.Fatalf("Invalid Wechat API Secret file: %s\nExpected: %s", secondConfig.APISecretFile, "/override_file")
	}

	// override the global file with an inline URL
	thirdConfig := conf.Receivers[0].WechatConfigs[2]
	if string(thirdConfig.APISecret) != "my_inline_secret" || thirdConfig.APISecretFile != "" {
		t.Fatalf("Invalid Wechat API Secret: %s\nExpected: %s", string(thirdConfig.APISecret), "my_inline_secret")
	}
}

func TestWechatEmptyReceiverConfig(t *testing.T) {
	_, err := Load(`
global:
route:
  receiver: 'test-receiver'
receivers:
- name: 'test-receiver'
  wechat_configs:
  -
`)
	require.ErrorContains(t, err, "no global Wechat Api Secret set either inline or in a file")
}

func TestWechatRejectsReceiverSecretAndFile(t *testing.T) {
	var cfg WechatConfig
	err := yaml.UnmarshalStrict([]byte("api_secret: test-secret\napi_secret_file: /test-file\n"), &cfg)
	require.EqualError(t, err, "at most one of api_secret & api_secret_file must be configured")
}
