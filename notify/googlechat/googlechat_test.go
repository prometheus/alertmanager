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

package googlechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

// This is a test URL that has been modified to not be valid.
var testGoogleChatURL, _ = url.Parse("https://chat.googleapis.com/v1/spaces/XXXXXX/messages?key=YYYYYYY&token=ZZZZZZZ")

func TestGoogleChatRetry(t *testing.T) {
	notifier, err := New(
		&config.GoogleChatConfig{
			URL:        &config.SecretURL{URL: testGoogleChatURL},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	t.Run("test retry status code", func(t *testing.T) {
		retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
		for statusCode, expected := range test.RetryTests(retryCodes) {
			actual, _ := notifier.retrier.Check(statusCode, nil)
			require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
		}
	})
}

func TestGoogleChatRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.GoogleChatConfig{
			URL:        &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestGoogleChatReadingURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "webhook_url")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.GoogleChatConfig{
			URLFile:    f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestGooglechatTemplating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		out := make(map[string]interface{})
		err := dec.Decode(&out)
		if err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	for _, tc := range []struct {
		title string
		cfg   *config.GoogleChatConfig

		retry  bool
		errMsg string
	}{
		{
			title: "default message",
			cfg: &config.GoogleChatConfig{
				Message: `{{ template "googlechat.default.message" . }}`,
			},
			retry: false,
		},
		{
			title: "message with templating errors",
			cfg: &config.GoogleChatConfig{
				Message: "{{ ",
			},
			errMsg: "template: :1: unclosed action",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.URL = &config.SecretURL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), log.NewNopLogger())
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

func TestGooglechatCardMode(t *testing.T) {
	var receivedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		receivedPayload = make(map[string]interface{})
		if err := dec.Decode(&receivedPayload); err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	cfg := &config.GoogleChatConfig{
		URL:          &config.SecretURL{URL: u},
		HTTPConfig:   &commoncfg.HTTPClientConfig{},
		Message:      "Preview text",
		CardTitle:    "Alert: High CPU",
		CardSubtitle: "Critical | Firing 1",
		CardImageURL: "https://oodle.ai/img/logo.svg",
		CardMessage:  "CPU usage exceeded threshold",
		CardDetails:  "Severity: Critical\nThreshold: 95%\nMetric Value: 98.2%",
		CardLabels:   "cluster: prod-us-east-1\nnamespace: api-server",
		CardActions:  "View Alert|https://app.oodle.ai/alerts/123\nAI Insights|https://app.oodle.ai/insights/123",
	}

	pd, err := New(cfg, test.CreateTmpl(t), log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "test-group")

	ok, err := pd.Notify(ctx, []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "HighCPU"},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}...)
	require.NoError(t, err)
	require.False(t, ok)

	// Verify cardsV2 key exists and no duplicate text line
	require.Contains(t, receivedPayload, "cardsV2")
	require.NotContains(t, receivedPayload, "text")

	// Verify card structure
	cardsV2, ok := receivedPayload["cardsV2"].([]interface{})
	require.True(t, ok)
	require.Len(t, cardsV2, 1)

	cardEntry, ok := cardsV2[0].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, cardEntry, "cardId")
	require.Contains(t, cardEntry, "card")

	card, ok := cardEntry["card"].(map[string]interface{})
	require.True(t, ok)

	// Verify header
	header, ok := card["header"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "Alert: High CPU", header["title"])
	require.Equal(t, "Critical | Firing 1", header["subtitle"])
	require.Equal(t, "https://oodle.ai/img/logo.svg", header["imageUrl"])

	// Verify sections exist: message, labels, details, actions
	sections, ok := card["sections"].([]interface{})
	require.True(t, ok)
	require.Len(t, sections, 4)
}

func TestGooglechatTextFallback(t *testing.T) {
	var receivedPayload map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		receivedPayload = make(map[string]interface{})
		if err := dec.Decode(&receivedPayload); err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	cfg := &config.GoogleChatConfig{
		URL:        &config.SecretURL{URL: u},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
		Message:    "Plain text alert",
	}

	pd, err := New(cfg, test.CreateTmpl(t), log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "test-group")

	ok, err := pd.Notify(ctx, []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "TestAlert"},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}...)
	require.NoError(t, err)
	require.False(t, ok)

	// Text mode: no cardsV2
	require.NotContains(t, receivedPayload, "cardsV2")
	require.Equal(t, "Plain text alert", receivedPayload["text"])
}

func TestBuildCard(t *testing.T) {
	t.Run("full card", func(t *testing.T) {
		card := buildCard(
			"Alert Title",
			"Critical",
			"https://example.com/icon.png",
			"Something went wrong",
			"Severity: Critical\nThreshold: 95%",
			"cluster: prod\nregion: us-east-1\nservice: api",
			"View|https://example.com\nEdit|https://example.com/edit",
		)

		require.NotNil(t, card.Header)
		require.Equal(t, "Alert Title", card.Header.Title)
		require.Equal(t, "Critical", card.Header.Subtitle)
		require.Equal(t, "https://example.com/icon.png", card.Header.ImageURL)

		// message + details + labels + actions = 4 sections
		require.Len(t, card.Sections, 4)

		// Message section
		require.NotNil(t, card.Sections[0].Widgets[0].TextParagraph)
		require.Equal(t, "Something went wrong", card.Sections[0].Widgets[0].TextParagraph.Text)

		// Labels section (collapsible, before details)
		require.Equal(t, "Labels", card.Sections[1].Header)
		require.True(t, card.Sections[1].Collapsible)
		require.Equal(t, 2, card.Sections[1].UncollapsibleWidgetsCount)
		require.Len(t, card.Sections[1].Widgets, 3)

		// Details section (collapsible)
		require.Equal(t, "Alert Details", card.Sections[2].Header)
		require.True(t, card.Sections[2].Collapsible)
		require.Equal(t, 0, card.Sections[2].UncollapsibleWidgetsCount)
		require.Len(t, card.Sections[2].Widgets, 2)
		require.Equal(t, "Severity", card.Sections[2].Widgets[0].DecoratedText.TopLabel)
		require.Equal(t, "Critical", card.Sections[2].Widgets[0].DecoratedText.Text)

		// Actions section
		require.Len(t, card.Sections[3].Widgets, 1)
		require.NotNil(t, card.Sections[3].Widgets[0].ButtonList)
		require.Len(t, card.Sections[3].Widgets[0].ButtonList.Buttons, 2)
		require.Equal(t, "View", card.Sections[3].Widgets[0].ButtonList.Buttons[0].Text)
		require.Equal(t, "https://example.com", card.Sections[3].Widgets[0].ButtonList.Buttons[0].OnClick.OpenLink.URL)
	})

	t.Run("empty fields produce no sections", func(t *testing.T) {
		card := buildCard("Title", "", "", "", "", "", "")
		require.NotNil(t, card.Header)
		require.Len(t, card.Sections, 0)
	})

	t.Run("handles malformed lines", func(t *testing.T) {
		card := buildCard("", "", "", "", "no-colon-here\n\nvalid: line", "also bad\nk: v", "no-pipe\nLabel|https://url.com")
		// labels first: 1 valid widget
		require.Len(t, card.Sections[0].Widgets, 1)
		require.Equal(t, "k", card.Sections[0].Widgets[0].DecoratedText.TopLabel)
		// details: 1 valid widget
		require.Len(t, card.Sections[1].Widgets, 1)
		require.Equal(t, "valid", card.Sections[1].Widgets[0].DecoratedText.TopLabel)
		// actions: 1 valid button
		require.Len(t, card.Sections[2].Widgets[0].ButtonList.Buttons, 1)
	})

	t.Run("value containing delimiter", func(t *testing.T) {
		card := buildCard("", "", "", "", "url: https://example.com:8080/path", "", "")
		require.Len(t, card.Sections[0].Widgets, 1)
		require.Equal(t, "url", card.Sections[0].Widgets[0].DecoratedText.TopLabel)
		require.Equal(t, "https://example.com:8080/path", card.Sections[0].Widgets[0].DecoratedText.Text)
	})
}
