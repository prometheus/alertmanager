// Copyright 2026 Prometheus Team
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

func TestWechatInlineSecretDecoding(t *testing.T) {
	// These expectations were checked against the fork before the secret type changed.
	const fallback = "global-inline"
	const local = "receiver-inline"
	const route = "route:\n  receiver: wechat\nreceivers:\n- name: wechat\n  wechat_configs:\n  - "
	tests := []struct {
		name     string
		suffix   string
		want     string
		wantType string
	}{
		{name: "omitted"},
		{name: "null", suffix: ":null"},
		{name: "empty", suffix: `:""`},
		{name: "scalar", suffix: `:"inline"`, want: "inline"},
		{name: "numeric scalar", suffix: ":123", want: "123", wantType: "number"},
		{name: "object", suffix: `:{"value":"inline"}`, wantType: "object"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			global, receiver := "{}", "{}"
			if tc.suffix != "" {
				global = `{"wechat_api_secret"` + tc.suffix + "}"
				receiver = `{"api_secret"` + tc.suffix + "}"
			}

			t.Run("direct YAML", func(t *testing.T) {
				var g GlobalConfig
				var r WechatConfig
				globalErr := yaml.UnmarshalStrict([]byte(global), &g)
				receiverErr := yaml.UnmarshalStrict([]byte(receiver), &r)
				if tc.wantType == "object" {
					require.ErrorContains(t, globalErr, "cannot unmarshal !!map")
					require.ErrorContains(t, receiverErr, "cannot unmarshal !!map")
					return
				}
				require.NoError(t, globalErr)
				require.NoError(t, receiverErr)
				require.Equal(t, tc.want, string(g.WeChatAPISecret))
				require.Equal(t, tc.want, string(r.APISecret))
			})

			t.Run("direct JSON", func(t *testing.T) {
				var g GlobalConfig
				var r WechatConfig
				globalErr := json.Unmarshal([]byte(global), &g)
				receiverErr := json.Unmarshal([]byte(receiver), &r)
				if tc.wantType != "" {
					var globalTypeErr, receiverTypeErr *json.UnmarshalTypeError
					require.ErrorAs(t, globalErr, &globalTypeErr)
					require.ErrorAs(t, receiverErr, &receiverTypeErr)
					require.Equal(t, tc.wantType, globalTypeErr.Value)
					require.Equal(t, tc.wantType, receiverTypeErr.Value)
					return
				}
				require.NoError(t, globalErr)
				require.NoError(t, receiverErr)
				require.Equal(t, tc.want, string(g.WeChatAPISecret))
				require.Equal(t, tc.want, string(r.APISecret))
			})

			t.Run("global YAML in Load", func(t *testing.T) {
				cfg, err := Load("global: " + global + "\n" + route + `{"corp_id":"corp","api_secret":"` + local + `"}` + "\n")
				if tc.wantType == "object" {
					require.ErrorContains(t, err, "cannot unmarshal !!map")
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.want, string(cfg.Global.WeChatAPISecret))
				require.Equal(t, local, string(cfg.Receivers[0].WechatConfigs[0].APISecret))
			})

			t.Run("receiver YAML in Load", func(t *testing.T) {
				cfg, err := Load("global:\n  wechat_api_corp_id: corp\n  wechat_api_secret: " + fallback + "\n" + route + receiver + "\n")
				if tc.wantType == "object" {
					require.ErrorContains(t, err, "cannot unmarshal !!map")
					return
				}
				require.NoError(t, err)
				want := tc.want
				if want == "" {
					want = fallback
				}
				require.Equal(t, want, string(cfg.Receivers[0].WechatConfigs[0].APISecret))
			})
		})
	}
}
