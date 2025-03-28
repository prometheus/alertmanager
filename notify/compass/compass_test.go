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

package compass

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestCompassRetry(t *testing.T) {
	notifier, err := New(
		&config.CompassConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestCompassRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	user := "user"
	key := "key"
	notifier, err := New(
		&config.CompassConfig{
			APIURL:     &config.URL{URL: u},
			APIUser:    user,
			APIKey:     config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, key)
}

func TestGettingCompassApikeyFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	user := "user"
	key := "key"

	f, err := os.CreateTemp("", "compass_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(key)
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.CompassConfig{
			APIURL:     &config.URL{URL: u},
			APIUser:    user,
			APIKeyFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, key)
}

func TestCompass(t *testing.T) {
	u, err := url.Parse("https://compass/api/")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	logger := promslog.NewNopLogger()
	tmpl := test.CreateTmpl(t)

	for _, tc := range []struct {
		title string
		cfg   *config.CompassConfig

		expectedEmptyAlertBody string
		expectedBody           string
	}{
		{
			title: "config without details",
			cfg: &config.CompassConfig{
				NotifierConfig: config.NotifierConfig{
					VSendResolved: true,
				},
				Message:     `{{ .CommonLabels.Message }}`,
				Description: `{{ .CommonLabels.Description }}`,
				Source:      `{{ .CommonLabels.Source }}`,
				Responders: []config.CompassConfigConfigResponder{
					{
						ID:   `{{ .CommonLabels.ResponderID1 }}`,
						Type: `{{ .CommonLabels.ResponderType1 }}`,
					},
					{
						ID:   `{{ .CommonLabels.ResponderID2 }}`,
						Type: `{{ .CommonLabels.ResponderType2 }}`,
					},
				},
				Tags:       `{{ .CommonLabels.Tags }}`,
				Note:       `{{ .CommonLabels.Note }}`,
				Priority:   `{{ .CommonLabels.Priority }}`,
				Entity:     `{{ .CommonLabels.Entity }}`,
				Actions:    `{{ .CommonLabels.Actions }}`,
				APIUser:    `{{ .ExternalURL }}`,
				APIKey:     `{{ .ExternalURL }}`,
				APIURL:     &config.URL{URL: u},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			},
			expectedEmptyAlertBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","source":""}
`,
			expectedBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","source":"http://prometheus","responders":[{"id":"Schedule1","type":"schedule"},{"id":"Escalation2","type":"escalation"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1","entity":"test-domain","actions":["doThis","doThat"],"extraProperties":{"Actions":"doThis,doThat","Description":"description","Entity":"test-domain","Message":"message","Note":"this is a note","Priority":"P1","ResponderID1":"Schedule1","ResponderID2":"Escalation2","ResponderID3":"Team3","ResponderType1":"schedule","ResponderType2":"escalation","ResponderType3":"team","Source":"http://prometheus","Tags":"tag1,tag2"}}
`,
		},
		{
			title: "config with details",
			cfg: &config.CompassConfig{
				NotifierConfig: config.NotifierConfig{
					VSendResolved: true,
				},
				Message:     `{{ .CommonLabels.Message }}`,
				Description: `{{ .CommonLabels.Description }}`,
				Source:      `{{ .CommonLabels.Source }}`,
				Responders: []config.CompassConfigConfigResponder{
					{
						ID:   `{{ .CommonLabels.ResponderID1 }}`,
						Type: `{{ .CommonLabels.ResponderType1 }}`,
					},
					{
						ID:   `{{ .CommonLabels.ResponderID2 }}`,
						Type: `{{ .CommonLabels.ResponderType2 }}`,
					},
				},
				Tags:     `{{ .CommonLabels.Tags }}`,
				Note:     `{{ .CommonLabels.Note }}`,
				Priority: `{{ .CommonLabels.Priority }}`,
				Entity:   `{{ .CommonLabels.Entity }}`,
				Actions:  `{{ .CommonLabels.Actions }}`,
				ExtraProperties: map[string]string{
					"Description": `adjusted {{ .CommonLabels.Description }}`,
				},
				APIUser:    `{{ .ExternalURL }}`,
				APIKey:     `{{ .ExternalURL }}`,
				APIURL:     &config.URL{URL: u},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			},
			expectedEmptyAlertBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","source":"","extraProperties":{"Description":"adjusted "}}
`,
			expectedBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","source":"http://prometheus","responders":[{"id":"Schedule1","type":"schedule"},{"id":"Escalation2","type":"escalation"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1","entity":"test-domain","actions":["doThis","doThat"],"extraProperties":{"Actions":"doThis,doThat","Description":"adjusted description","Entity":"test-domain","Message":"message","Note":"this is a note","Priority":"P1","ResponderID1":"Schedule1","ResponderID2":"Escalation2","ResponderID3":"Team3","ResponderType1":"schedule","ResponderType2":"escalation","ResponderType3":"team","Source":"http://prometheus","Tags":"tag1,tag2"}}
`,
		},
		{
			title: "config with multiple teams",
			cfg: &config.CompassConfig{
				NotifierConfig: config.NotifierConfig{
					VSendResolved: true,
				},
				Message:     `{{ .CommonLabels.Message }}`,
				Description: `{{ .CommonLabels.Description }}`,
				Source:      `{{ .CommonLabels.Source }}`,
				Responders: []config.CompassConfigConfigResponder{
					{
						ID:   `{{ .CommonLabels.ResponderID3 }}`,
						Type: `{{ .CommonLabels.ResponderType3 }}`,
					},
				},
				Tags:     `{{ .CommonLabels.Tags }}`,
				Note:     `{{ .CommonLabels.Note }}`,
				Priority: `{{ .CommonLabels.Priority }}`,
				ExtraProperties: map[string]string{
					"Description": `adjusted {{ .CommonLabels.Description }}`,
				},
				APIUser:    `{{ .ExternalURL }}`,
				APIKey:     `{{ .ExternalURL }}`,
				APIURL:     &config.URL{URL: u},
				HTTPConfig: &commoncfg.HTTPClientConfig{},
			},
			expectedEmptyAlertBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","source":"","extraProperties":{"Description":"adjusted "}}
`,
			expectedBody: `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","source":"http://prometheus","responders":[{"id":"Team3","type":"team"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1","extraProperties":{"Actions":"doThis,doThat","Description":"adjusted description","Entity":"test-domain","Message":"message","Note":"this is a note","Priority":"P1","ResponderID1":"Schedule1","ResponderID2":"Escalation2","ResponderID3":"Team3","ResponderType1":"schedule","ResponderType2":"escalation","ResponderType3":"team","Source":"http://prometheus","Tags":"tag1,tag2"}}
`,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			notifier, err := New(tc.cfg, tmpl, logger)
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			expectedURL, _ := url.Parse("https://compass/api/v1/alerts")

			// Empty alert.
			alert1 := &types.Alert{
				Alert: model.Alert{
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}

			req, retry, err := notifier.createRequests(ctx, alert1)
			require.NoError(t, err)
			require.Len(t, req, 1)
			require.True(t, retry)
			require.Equal(t, expectedURL, req[0].URL)
			require.Equal(t, "Basic aHR0cDovL2FtOmh0dHA6Ly9hbQ==", req[0].Header.Get("Authorization"))
			require.Equal(t, tc.expectedEmptyAlertBody, readBody(t, req[0]))

			// Fully defined alert.
			alert2 := &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"Message":        "message",
						"Description":    "description",
						"Source":         "http://prometheus",
						"ResponderID1":   "Schedule1",
						"ResponderType1": "schedule",
						"ResponderID2":   "Escalation2",
						"ResponderType2": "escalation",
						"ResponderID3":   "Team3",
						"ResponderType3": "team",
						"Tags":           "tag1,tag2",
						"Note":           "this is a note",
						"Priority":       "P1",
						"Entity":         "test-domain",
						"Actions":        "doThis,doThat",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}
			req, retry, err = notifier.createRequests(ctx, alert2)
			require.NoError(t, err)
			require.True(t, retry)
			require.Len(t, req, 1)
			require.Equal(t, tc.expectedBody, readBody(t, req[0]))

			// Broken API Key Template.
			tc.cfg.APIKey = "{{ kaput "
			_, _, err = notifier.createRequests(ctx, alert2)
			require.Error(t, err)
			require.Equal(t, "templating error: template: :1: function \"kaput\" not defined", err.Error())
		})
	}
}

func TestCompassWithUpdate(t *testing.T) {
	u, err := url.Parse("https://test-compass-url")
	require.NoError(t, err)
	tmpl := test.CreateTmpl(t)
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	compassConfigWithUpdate := config.CompassConfig{
		Message:      `{{ .CommonLabels.Message }}`,
		Description:  `{{ .CommonLabels.Description }}`,
		UpdateAlerts: true,
		APIUser:      "test-api-user",
		APIKey:       "test-api-key",
		APIURL:       &config.URL{URL: u},
		HTTPConfig:   &commoncfg.HTTPClientConfig{},
	}
	notifierWithUpdate, err := New(&compassConfigWithUpdate, tmpl, promslog.NewNopLogger())
	alert := &types.Alert{
		Alert: model.Alert{
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
			Labels: model.LabelSet{
				"Message":     "new message",
				"Description": "new description",
			},
		},
	}
	require.NoError(t, err)
	requests, retry, err := notifierWithUpdate.createRequests(ctx, alert)
	require.NoError(t, err)
	require.True(t, retry)
	require.Len(t, requests, 3)

	body0 := readBody(t, requests[0])
	body1 := readBody(t, requests[1])
	body2 := readBody(t, requests[2])
	key, _ := notify.ExtractGroupKey(ctx)
	alias := key.Hash()

	require.Equal(t, "https://test-compass-url/v1/alerts", requests[0].URL.String())
	require.NotEmpty(t, body0)

	require.Equal(t, requests[1].URL.String(), fmt.Sprintf("https://test-compass-url/v1/alerts/%s/message?identifierType=alias", alias))
	require.Equal(t, `{"message":"new message"}
`, body1)
	require.Equal(t, requests[2].URL.String(), fmt.Sprintf("https://test-compass-url/v1/alerts/%s/description?identifierType=alias", alias))
	require.Equal(t, `{"description":"new description"}
`, body2)
}

func TestCompassApiKeyFile(t *testing.T) {
	u, err := url.Parse("https://test-compass-url")
	require.NoError(t, err)
	tmpl := test.CreateTmpl(t)
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	compassConfigWithUpdate := config.CompassConfig{
		APIUser:    "test-api-user",
		APIKeyFile: `./api_key_file`,
		APIURL:     &config.URL{URL: u},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
	}
	notifierWithUpdate, err := New(&compassConfigWithUpdate, tmpl, promslog.NewNopLogger())

	require.NoError(t, err)
	requests, _, err := notifierWithUpdate.createRequests(ctx)
	require.NoError(t, err)
	require.Equal(t, "Basic dGVzdC1hcGktdXNlcjpteV9zZWNyZXRfYXBpX2tleQ==", requests[0].Header.Get("Authorization"))
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return string(body)
}
