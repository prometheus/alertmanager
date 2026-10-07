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

package receiver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/notify/webhook"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
)

type sendResolved bool

func (s sendResolved) SendResolved() bool { return bool(s) }

func TestBuildReceiverIntegrations(t *testing.T) {
	for _, tc := range []struct {
		receiver config.Receiver
		err      bool
		exp      []notify.Integration
	}{
		{
			receiver: config.Receiver{
				Name: "foo",
				WebhookConfigs: []*webhook.WebhookConfig{
					{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
					},
					{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
						NotifierConfig: amcommoncfg.NotifierConfig{
							VSendResolved: true,
						},
					},
				},
			},
			exp: []notify.Integration{
				notify.NewIntegration(nil, sendResolved(false), "webhook", 0, "foo"),
				notify.NewIntegration(nil, sendResolved(true), "webhook", 1, "foo"),
			},
		},
		{
			receiver: config.Receiver{
				Name: "foo",
				WebhookConfigs: []*webhook.WebhookConfig{
					{
						HTTPConfig: &commoncfg.HTTPClientConfig{
							TLSConfig: commoncfg.TLSConfig{
								CAFile: "not_existing",
							},
						},
					},
				},
			},
			err: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			integrations, err := BuildReceiverIntegrations(tc.receiver, nil, nil)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, integrations, len(tc.exp))
			for i := range tc.exp {
				require.Equal(t, tc.exp[i].SendResolved(), integrations[i].SendResolved())
				require.Equal(t, tc.exp[i].Name(), integrations[i].Name())
				require.Equal(t, tc.exp[i].Index(), integrations[i].Index())
			}
		})
	}
}

// TestBuildReceiverIntegrationsUnreadableFile checks that the *_file settings
// a notifier reads are checked when the integration is built, so that a
// missing file fails the configuration (re)load instead of the first
// notification.
func TestBuildReceiverIntegrationsUnreadableFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setting string
		global  string
		conf    string
		// unused is set when the notifier never reads the file, so a
		// missing file must not fail the build.
		unused bool
	}{
		{name: "webhook", setting: "url_file", conf: `webhook_configs: [{url_file: FILE}]`},
		{name: "discord", setting: "webhook_url_file", conf: `discord_configs: [{webhook_url_file: FILE}]`},
		{name: "email", setting: "auth_password_file", conf: `email_configs: [{to: a@example.com, from: b@example.com, smarthost: localhost:25, auth_password_file: FILE}]`},
		{name: "email", setting: "auth_secret_file", conf: `email_configs: [{to: a@example.com, from: b@example.com, smarthost: localhost:25, auth_secret_file: FILE}]`},
		{name: "incidentio", setting: "url_file", conf: `incidentio_configs: [{url_file: FILE}]`},
		{name: "mattermost", setting: "webhook_url_file", conf: `mattermost_configs: [{webhook_url_file: FILE}]`},
		{name: "msteams", setting: "webhook_url_file", conf: `msteams_configs: [{webhook_url_file: FILE}]`},
		{name: "msteamsv2", setting: "webhook_url_file", conf: `msteamsv2_configs: [{webhook_url_file: FILE}]`},
		{name: "opsgenie", setting: "api_key_file", conf: `opsgenie_configs: [{api_key_file: FILE}]`},
		{name: "pagerduty", setting: "service_key_file", conf: `pagerduty_configs: [{service_key_file: FILE}]`},
		{name: "pagerduty", setting: "routing_key_file", conf: `pagerduty_configs: [{routing_key_file: FILE}]`},
		{name: "pushover", setting: "user_key_file", conf: `pushover_configs: [{user_key_file: FILE, token: token}]`},
		{name: "pushover", setting: "token_file", conf: `pushover_configs: [{user_key: key, token_file: FILE}]`},
		{name: "slack", setting: "api_url_file", conf: `slack_configs: [{api_url_file: FILE}]`},
		{name: "telegram", setting: "bot_token_file", conf: `telegram_configs: [{bot_token_file: FILE, chat_id: 1}]`},
		{name: "telegram", setting: "chat_id_file", conf: `telegram_configs: [{bot_token: token, chat_id_file: FILE}]`},
		{name: "victorops", setting: "api_key_file", conf: `victorops_configs: [{api_key_file: FILE, routing_key: key}]`},
		{name: "wechat", setting: "api_secret_file", conf: `wechat_configs: [{api_secret_file: FILE, corp_id: id}]`},
		// Global settings are copied into each receiver when the
		// configuration is loaded.
		{name: "global slack", setting: "api_url_file", global: `slack_api_url_file: FILE`, conf: `slack_configs: [{}]`},
		{name: "global smtp", setting: "auth_password_file", global: `smtp_auth_password_file: FILE`, conf: `email_configs: [{to: a@example.com, from: b@example.com, smarthost: localhost:25}]`},
		// A receiver with an app token posts to the app URL and never reads
		// the global api_url_file it inherits.
		{name: "global slack with app token", setting: "api_url_file", global: `slack_api_url_file: FILE`, conf: `slack_configs: [{app_token: token}]`, unused: true},
	} {
		t.Run(tc.name+"/"+tc.setting, func(t *testing.T) {
			build := func(path string) error {
				path = strconv.Quote(path)
				conf, err := config.Load("global: {" + strings.ReplaceAll(tc.global, "FILE", path) + "}\n" +
					"route: {receiver: test}\n" +
					"receivers: [{name: test, " + strings.ReplaceAll(tc.conf, "FILE", path) + "}]\n")
				require.NoError(t, err)
				_, err = BuildReceiverIntegrations(conf.Receivers[0], nil, nil)
				return err
			}

			missing := filepath.Join(t.TempDir(), "missing")
			if tc.unused {
				require.NoError(t, build(missing))
				return
			}
			require.ErrorContains(t, build(missing), "failed to read "+tc.setting)
			require.ErrorContains(t, build(t.TempDir()), "is a directory")

			present := filepath.Join(t.TempDir(), "present")
			require.NoError(t, os.WriteFile(present, []byte("1"), 0o600))
			require.NoError(t, build(present))
		})
	}
}
