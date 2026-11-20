// Copyright 2019 Prometheus Team
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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestVictorOpsCustomFields(t *testing.T) {
	logger := promslog.NewNopLogger()
	tmpl := test.CreateTmpl(t)

	url, err := url.Parse("http://nowhere.com")

	require.NoError(t, err, "unexpected error parsing mock url")

	conf := &config.VictorOpsConfig{
		APIKey:            `12345`,
		APIURL:            &amcommoncfg.URL{URL: url},
		EntityDisplayName: `{{ .CommonLabels.Message }}`,
		StateMessage:      `{{ .CommonLabels.Message }}`,
		RoutingKey:        `test`,
		MessageType:       ``,
		MonitoringTool:    `AM`,
		CustomFields: map[string]string{
			"Field_A": "{{ .CommonLabels.Message }}",
		},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
	}

	notifier, err := New(conf, tmpl, logger)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"Message": "message",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	msg, err := notifier.createVictorOpsPayload(ctx, alert)
	require.NoError(t, err)

	var m map[string]string
	err = json.Unmarshal(msg.Bytes(), &m)

	require.NoError(t, err)

	// Verify that a custom field was added to the payload and templatized.
	require.Equal(t, "message", m["Field_A"])
}

func TestVictorOpsRetry(t *testing.T) {
	notifier, err := New(
		&config.VictorOpsConfig{
			APIKey:     commoncfg.Secret("secret"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)
	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "error on status %d", statusCode)
	}
}

func TestVictorOpsRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.VictorOpsConfig{
			APIURL:     &amcommoncfg.URL{URL: u},
			APIKey:     commoncfg.Secret(secret),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestVictorOpsReadingApiKeyFromFile(t *testing.T) {
	key := "key"
	f, err := os.CreateTemp(t.TempDir(), "victorops_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(key)
	require.NoError(t, err, "writing to temp file failed")

	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.VictorOpsConfig{
			APIURL:     &amcommoncfg.URL{URL: u},
			APIKeyFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, key)
}

func TestVictorOpsTemplating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		out := make(map[string]any)
		err := dec.Decode(&out)
		if err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	tests := []struct {
		name   string
		cfg    *config.VictorOpsConfig
		errMsg string
	}{
		{
			name: "default valid templates",
			cfg:  &config.VictorOpsConfig{},
		},
		{
			name: "invalid message_type",
			cfg: &config.VictorOpsConfig{
				MessageType: "{{ .CommonLabels.alertname }",
			},
			errMsg: "templating error",
		},
		{
			name: "invalid entity_display_name",
			cfg: &config.VictorOpsConfig{
				EntityDisplayName: "{{ .CommonLabels.alertname }",
			},
			errMsg: "templating error",
		},
		{
			name: "invalid state_message",
			cfg: &config.VictorOpsConfig{
				StateMessage: "{{ .CommonLabels.alertname }",
			},
			errMsg: "templating error",
		},
		{
			name: "invalid monitoring tool",
			cfg: &config.VictorOpsConfig{
				MonitoringTool: "{{ .CommonLabels.alertname }",
			},
			errMsg: "templating error",
		},
		{
			name: "invalid routing_key",
			cfg: &config.VictorOpsConfig{
				RoutingKey: "{{ .CommonLabels.alertname }",
			},
			errMsg: "templating error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			tc.cfg.APIURL = &amcommoncfg.URL{URL: u}
			tc.cfg.APIKey = "test"
			vo, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)
			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			_, err = vo.Notify(ctx, []*types.Alert{
				{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Contains(t, err.Error(), tc.errMsg)
			}
		})
	}
}
