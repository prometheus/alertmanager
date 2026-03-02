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

package mattermost

import (
	"context"
	"encoding/json"
	"io"
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

var testWebhookURL, _ = url.Parse("https://mattermost.example.com/hooks/xxxxxxxxxxxxxxxxxxxxxxxxxx")

func TestMattermostRetry(t *testing.T) {
	notifier, err := New(
		&config.MattermostConfig{
			WebhookURL: &amcommoncfg.SecretURL{URL: testWebhookURL},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "retry - error on status %d", statusCode)
	}
}

func TestMattermostTemplating(t *testing.T) {
	// Create a fake HTTP server to simulate the Mattermost webhook
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

	for _, tc := range []struct {
		title string
		cfg   *config.MattermostConfig

		retry  bool
		errMsg string
	}{
		{
			title: "text with default templating",
			cfg:   &config.DefaultMattermostConfig,
			retry: false,
		},
		{
			title: "text with templating errors",
			cfg: &config.MattermostConfig{
				Text: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.WebhookURL = &amcommoncfg.SecretURL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			ok, err := pd.Notify(ctx, []*types.Alert{
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
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
			require.Equal(t, tc.retry, ok)
		})
	}
}

func TestMattermostRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.MattermostConfig{
			WebhookURL: &amcommoncfg.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestMattermostReadingURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "webhook_url")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.MattermostConfig{
			WebhookURLFile: f.Name(),
			HTTPConfig:     &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestMattermost_Notify(t *testing.T) {
	// Create a fake HTTP server to simulate the Mattermost webhook
	var resp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request as a string
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err, "reading request body failed")
		// Store the request body in the response
		resp = string(body)

		w.WriteHeader(http.StatusOK)
	}))

	// Create a temporary file to simulate the WebhookURLFile
	tempFile, err := os.CreateTemp(t.TempDir(), "webhook_url")
	require.NoError(t, err)

	// Write the fake webhook URL to the temp file
	_, err = tempFile.WriteString(srv.URL)
	require.NoError(t, err)

	// Create a context and alerts
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"lbl1": "val1",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}

	type testcase struct {
		name        string
		text        string
		props       *config.MattermostProps
		priority    *config.MattermostPriority
		attachments []*config.MattermostAttachment
		result      string
	}
	tests := []testcase{
		{
			name:   "with text only",
			text:   "Test Text",
			result: "{\"text\":\"Test Text\"}\n",
		},
		{
			name:     "with text and props",
			text:     "Test Text",
			props:    &config.MattermostProps{Card: "Test Card"},
			priority: nil,
			result:   "{\"text\":\"Test Text\",\"props\":{\"card\":\"Test Card\"}}\n",
		},
		{
			name:     "with text and priority standard",
			text:     "Test Text",
			props:    nil,
			priority: &config.MattermostPriority{Priority: "standard", RequestedAck: true, PersistentNotifications: true},
			result:   "{\"text\":\"Test Text\",\"priority\":{\"priority\":\"standard\"}}\n",
		},
		{
			name:     "with text, props and priority",
			text:     "Test Text",
			props:    &config.MattermostProps{Card: "Test Card"},
			priority: &config.MattermostPriority{Priority: "urgent"},
			result:   "{\"text\":\"Test Text\",\"props\":{\"card\":\"Test Card\"},\"priority\":{\"priority\":\"urgent\"}}\n",
		},
		{
			name:   "with empty text - should omit text field",
			text:   "",
			result: "{}\n",
		},
		{
			name: "with empty text and attachments - should omit text field",
			text: "",
			attachments: []*config.MattermostAttachment{
				{
					Title: "Test Attachment",
					Text:  "Attachment Text",
				},
			},
			result: "{\"attachments\":[{\"text\":\"Attachment Text\",\"title\":\"Test Attachment\"}]}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a MattermostConfig with the WebhookURLFile set
			cfg := &config.MattermostConfig{
				WebhookURLFile: tempFile.Name(),
				HTTPConfig:     &commoncfg.HTTPClientConfig{},
				Text:           tc.text,
				Props:          tc.props,
				Priority:       tc.priority,
				Attachments:    tc.attachments,
			}

			// Create a new Mattermost notifier
			notifier, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			// Call the Notify method
			ok, err := notifier.Notify(ctx, alerts...)
			require.NoError(t, err)
			require.False(t, ok)

			require.Equal(t, tc.result, resp)
		})
	}
}
