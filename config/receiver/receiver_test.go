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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/notify/webhook"
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

func TestBuildSlackAppTokenIntegrations(t *testing.T) {
	t.Parallel()

	for name, credentials := range map[string]string{
		"inline": "slack_app_token: xoxb-test",
		"file":   "slack_app_token_file: token",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			type request struct {
				path, authorization string
			}
			requests := make(chan request, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- request{r.URL.Path, r.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"ok":true}`)
			}))
			t.Cleanup(server.Close)

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "token"), []byte("xoxb-test\n"), 0o600))
			configFile := filepath.Join(dir, "alertmanager.yml")
			require.NoError(t, os.WriteFile(configFile, fmt.Appendf(nil, `
global:
  %s
route:
  receiver: slack
receivers:
  - name: slack
    slack_configs:
      - channel: alerts
        app_url: %s/chat.postMessage
`, credentials, server.URL), 0o600))
			cfg, err := config.LoadFile(configFile)
			require.NoError(t, err)
			original := cfg.String()
			tmpl := test.CreateTmpl(t)
			ctx := notify.WithGroupKey(t.Context(), "slack-test")

			for range 2 {
				integrations, err := BuildReceiverIntegrations(cfg.Receivers[0], tmpl, nil)
				require.NoError(t, err)
				require.Len(t, integrations, 1)
				require.Equal(t, original, cfg.String())
				require.NoError(t, integrations[0].Notify(ctx).Err())
				require.Equal(t, request{"/chat.postMessage", "Bearer xoxb-test"}, <-requests)
			}
		})
	}
}
