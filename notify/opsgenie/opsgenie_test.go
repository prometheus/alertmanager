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

package opsgenie

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestOpsGenieRetry(t *testing.T) {
	notifier, err := New(
		&config.OpsGenieConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestOpsGenieRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	key := "key"
	notifier, err := New(
		&config.OpsGenieConfig{
			APIURL:     &config.URL{URL: u},
			APIKey:     config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, key)
}

func TestOpsGenie(t *testing.T) {
	u, err := url.Parse("https://opsgenie/api")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	logger := log.NewNopLogger()
	tmpl := test.CreateTmpl(t)

	for _, tc := range []struct {
		title string
		cfg   *config.OpsGenieConfig

		expectedEmptyAlertBody string
		expectedBody           string
	}{
		{
			title: "config without details",
			cfg: &config.OpsGenieConfig{
				NotifierConfig: config.NotifierConfig{
					VSendResolved: true,
				},
				Message:     `{{ .CommonLabels.Message }}`,
				Description: `{{ .CommonLabels.Description }}`,
				Source:      `{{ .CommonLabels.Source }}`,
				Responders: []config.OpsGenieConfigResponder{
					{
						Name: `{{ .CommonLabels.ResponderName1 }}`,
						Type: `{{ .CommonLabels.ResponderType1 }}`,
					},
					{
						Name: `{{ .CommonLabels.ResponderName2 }}`,
						Type: `{{ .CommonLabels.ResponderType2 }}`,
					},
				},
				Tags:       `{{ .CommonLabels.Tags }}`,
				Note:       `{{ .CommonLabels.Note }}`,
				Priority:   `{{ .CommonLabels.Priority }}`,
				APIKey:     `{{ .ExternalURL }}`,
				APIURL:     &config.URL{URL: u},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			},
			expectedEmptyAlertBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","details":{},"source":""}
`,
			expectedBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","details":{"Description":"description","Message":"message","Note":"this is a note","Priority":"P1","ResponderName1":"TeamA","ResponderName2":"EscalationA","ResponderType1":"team","ResponderType2":"escalation","Source":"http://prometheus","Tags":"tag1,tag2"},"source":"http://prometheus","responders":[{"name":"TeamA","type":"team"},{"name":"EscalationA","type":"escalation"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1"}
`,
		},
		{
			title: "config with details",
			cfg: &config.OpsGenieConfig{
				NotifierConfig: config.NotifierConfig{
					VSendResolved: true,
				},
				Message:     `{{ .CommonLabels.Message }}`,
				Description: `{{ .CommonLabels.Description }}`,
				Source:      `{{ .CommonLabels.Source }}`,
				Details: map[string]string{
					"Description": `adjusted {{ .CommonLabels.Description }}`,
				},
				Responders: []config.OpsGenieConfigResponder{
					{
						Name: `{{ .CommonLabels.ResponderName1 }}`,
						Type: `{{ .CommonLabels.ResponderType1 }}`,
					},
					{
						Name: `{{ .CommonLabels.ResponderName2 }}`,
						Type: `{{ .CommonLabels.ResponderType2 }}`,
					},
				},
				Tags:       `{{ .CommonLabels.Tags }}`,
				Note:       `{{ .CommonLabels.Note }}`,
				Priority:   `{{ .CommonLabels.Priority }}`,
				APIKey:     `{{ .ExternalURL }}`,
				APIURL:     &config.URL{URL: u},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			},
			expectedEmptyAlertBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","details":{"Description":"adjusted "},"source":""}
`,
			expectedBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","details":{"Description":"adjusted description","Message":"message","Note":"this is a note","Priority":"P1","ResponderName1":"TeamA","ResponderName2":"EscalationA","ResponderType1":"team","ResponderType2":"escalation","Source":"http://prometheus","Tags":"tag1,tag2"},"source":"http://prometheus","responders":[{"name":"TeamA","type":"team"},{"name":"EscalationA","type":"escalation"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1"}
`,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			notifier, err := New(tc.cfg, tmpl, logger)
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			expectedURL, _ := url.Parse("https://opsgenie/apiv2/alerts")

			// Empty alert.
			alert1 := &types.Alert{
				Alert: model.Alert{
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}

			req, retry, err := notifier.createRequest(ctx, alert1)
			require.NoError(t, err)
			require.Equal(t, true, retry)
			require.Equal(t, expectedURL, req.URL)
			require.Equal(t, "GenieKey http://am", req.Header.Get("Authorization"))
			require.Equal(t, tc.expectedEmptyAlertBody, readBody(t, req))

			// Fully defined alert.
			alert2 := &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"Message":        "message",
						"Description":    "description",
						"Source":         "http://prometheus",
						"ResponderName1": "TeamA",
						"ResponderType1": "team",
						"ResponderName2": "EscalationA",
						"ResponderType2": "escalation",
						"Tags":           "tag1,tag2",
						"Note":           "this is a note",
						"Priority":       "P1",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}
			req, retry, err = notifier.createRequest(ctx, alert2)
			require.NoError(t, err)
			require.Equal(t, true, retry)
			require.Equal(t, tc.expectedBody, readBody(t, req))

			// Broken API Key Template.
			tc.cfg.APIKey = "{{ kaput "
			_, _, err = notifier.createRequest(ctx, alert2)
			require.Error(t, err)
			require.Equal(t, err.Error(), "templating error: template: :1: function \"kaput\" not defined")
		})
	}
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := ioutil.ReadAll(r.Body)
	require.NoError(t, err)
	return string(body)
}
