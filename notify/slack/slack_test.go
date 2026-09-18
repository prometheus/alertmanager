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

package slack

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

func TestSlackRetry(t *testing.T) {
	notifier, err := New(
		&config.SlackConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "error on status %d", statusCode)
	}
}

func TestSlackRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.SlackConfig{
			APIURL:     &amcommoncfg.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestGettingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "slack_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String())
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestTrimmingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "slack_test_newline")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestNotifier_Notify_WithReason(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedReason notify.Reason
		expectedErr    string
		expectedRetry  bool
		noError        bool
	}{
		{
			name:           "with a 4xx status code",
			statusCode:     http.StatusUnauthorized,
			expectedReason: notify.AuthErrorReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 401",
		},
		{
			name:           "with a 5xx status code",
			statusCode:     http.StatusInternalServerError,
			expectedReason: notify.ServerErrorReason,
			expectedRetry:  true,
			expectedErr:    "unexpected status code 500",
		},
		{
			name:           "with a 3xx status code",
			statusCode:     http.StatusTemporaryRedirect,
			expectedReason: notify.DefaultReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 307",
		},
		{
			name:           "with a 1xx status code",
			statusCode:     http.StatusSwitchingProtocols,
			expectedReason: notify.DefaultReason,
			expectedRetry:  false,
			expectedErr:    "unexpected status code 101",
		},
		{
			name:           "2xx response with invalid JSON",
			statusCode:     http.StatusOK,
			responseBody:   `{"not valid json"}`,
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  true,
			expectedErr:    "could not unmarshal",
		},
		{
			name:           "2xx response with a JSON error",
			statusCode:     http.StatusOK,
			responseBody:   `{"ok":false,"error":"error_message"}`,
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  false,
			expectedErr:    "error response from Slack: error_message",
		},
		{
			name:           "2xx response with a plaintext error",
			statusCode:     http.StatusOK,
			responseBody:   "no_channel",
			expectedReason: notify.ClientErrorReason,
			expectedRetry:  false,
			expectedErr:    "error response from Slack: no_channel",
		},
		{
			name:           "with a 429 status code",
			statusCode:     http.StatusTooManyRequests,
			expectedReason: notify.RateLimitedReason,
			expectedRetry:  true,
			expectedErr:    "unexpected status code 429",
		},
		{
			name:         "successful JSON response",
			statusCode:   http.StatusOK,
			responseBody: `{"ok":true}`,
			noError:      true,
		},
		{
			name:         "successful plaintext response",
			statusCode:   http.StatusOK,
			responseBody: "ok",
			noError:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiurl, _ := url.Parse("https://slack.com/post.Message")
			notifier, err := New(
				&config.SlackConfig{
					NotifierConfig: amcommoncfg.NotifierConfig{},
					HTTPConfig:     &commoncfg.HTTPClientConfig{},
					APIURL:         &amcommoncfg.SecretURL{URL: apiurl},
					Channel:        "channelname",
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)

			notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
				resp := httptest.NewRecorder()
				if strings.HasPrefix(tt.responseBody, "{") {
					resp.Header().Add("Content-Type", "application/json; charset=utf-8")
				}
				resp.WriteHeader(tt.statusCode)
				resp.WriteString(tt.responseBody)
				return resp.Result(), nil
			}
			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			alert1 := &types.Alert{
				Alert: model.Alert{
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}
			retry, err := notifier.Notify(ctx, alert1)
			require.Equal(t, tt.expectedRetry, retry)
			if tt.noError {
				require.NoError(t, err)
			} else {
				var reasonError *notify.ErrorWithReason
				require.ErrorAs(t, err, &reasonError)
				require.Equal(t, tt.expectedReason, reasonError.Reason)
				require.Contains(t, err.Error(), tt.expectedErr)
				require.Contains(t, err.Error(), "channelname")
			}
		})
	}
}

func TestSlackTimeout(t *testing.T) {
	tests := map[string]struct {
		latency time.Duration
		timeout time.Duration
		wantErr bool
	}{
		"success": {latency: 100 * time.Millisecond, timeout: 120 * time.Millisecond, wantErr: false},
		"error":   {latency: 100 * time.Millisecond, timeout: 80 * time.Millisecond, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			u, _ := url.Parse("https://slack.com/post.Message")
			notifier, err := New(
				&config.SlackConfig{
					NotifierConfig: amcommoncfg.NotifierConfig{},
					HTTPConfig:     &commoncfg.HTTPClientConfig{},
					APIURL:         &amcommoncfg.SecretURL{URL: u},
					Channel:        "channelname",
					Timeout:        tt.timeout,
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)
			notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(tt.latency):
					resp := httptest.NewRecorder()
					resp.Header().Set("Content-Type", "application/json; charset=utf-8")
					resp.WriteHeader(http.StatusOK)
					resp.WriteString(`{"ok":true}`)

					return resp.Result(), nil
				}
			}
			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			alert := &types.Alert{
				Alert: model.Alert{
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}
			_, err = notifier.Notify(ctx, alert)
			require.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestSlackMessageField(t *testing.T) {
	// 1. Setup a fake Slack server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		// 2. VERIFY: Top-level text exists
		if body["text"] != "My Top Level Message" {
			t.Errorf("Expected top-level 'text' to be 'My Top Level Message', got %v", body["text"])
		}

		// 3. VERIFY: Old attachments still exist
		attachments, ok := body["attachments"].([]any)
		if !ok || len(attachments) == 0 {
			t.Errorf("Expected attachments to exist")
		} else {
			first := attachments[0].(map[string]any)
			if first["title"] != "Old Attachment Title" {
				t.Errorf("Expected attachment title 'Old Attachment Title', got %v", first["title"])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// 4. Configure Notifier with BOTH new and old fields
	u, _ := url.Parse(server.URL)
	conf := &config.SlackConfig{
		APIURL:      &amcommoncfg.SecretURL{URL: u},
		MessageText: "My Top Level Message", // Your NEW field
		Title:       "Old Attachment Title", // An OLD field
		Channel:     "#test-channel",
		HTTPConfig:  &commoncfg.HTTPClientConfig{},
	}

	tmpl, err := template.FromGlobs([]string{})
	if err != nil {
		t.Fatal(err)
	}
	tmpl.ExternalURL = u

	logger := slog.New(slog.DiscardHandler)
	notifier, err := New(conf, tmpl, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "test-group-key")

	if _, err := notifier.Notify(ctx); err != nil {
		t.Fatal("Notify failed:", err)
	}
}

func TestNotifier_Notify_RetryAfterSleep(t *testing.T) {
	apiurl, _ := url.Parse("https://slack.com/post.Message")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig: amcommoncfg.NotifierConfig{},
			HTTPConfig:     &commoncfg.HTTPClientConfig{},
			APIURL:         &amcommoncfg.SecretURL{URL: apiurl},
			Channel:        "channelname",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
		resp := httptest.NewRecorder()
		resp.Header().Set("Retry-After", "1")
		resp.WriteHeader(http.StatusTooManyRequests)
		return resp.Result(), nil
	}

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert1 := &types.Alert{
		Alert: model.Alert{
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	start := time.Now()
	retry, err := notifier.Notify(ctx, alert1)
	elapsed := time.Since(start)

	require.True(t, retry)
	require.Error(t, err)
	require.GreaterOrEqual(t, elapsed, 1*time.Second, "should have waited at least 1 second for Retry-After")
}

func TestNotifier_Notify_RetryAfterContextCancelled(t *testing.T) {
	apiurl, _ := url.Parse("https://slack.com/post.Message")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig: amcommoncfg.NotifierConfig{},
			HTTPConfig:     &commoncfg.HTTPClientConfig{},
			APIURL:         &amcommoncfg.SecretURL{URL: apiurl},
			Channel:        "channelname",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
		resp := httptest.NewRecorder()
		resp.Header().Set("Retry-After", "2")
		resp.WriteHeader(http.StatusTooManyRequests)
		return resp.Result(), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = notify.WithGroupKey(ctx, "1")

	// Cancel context after a short delay to interrupt the Retry-After sleep.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	alert1 := &types.Alert{
		Alert: model.Alert{
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	start := time.Now()
	retry, err := notifier.Notify(ctx, alert1)
	elapsed := time.Since(start)

	require.True(t, retry)
	require.Error(t, err)
	require.Less(t, elapsed, 2*time.Second, "should not have waited the full Retry-After duration")
}

func TestSkipThreadReply(t *testing.T) {
	tests := []struct {
		name   string
		edit   bool
		reason notify.NotifyReason
		omit   bool
		want   bool
	}{
		{name: "repeat while editing parent", edit: true, reason: notify.ReasonRepeatIntervalElapsed, want: true},
		{name: "resolved while editing parent", edit: true, reason: notify.ReasonAllAlertsResolved},
		{name: "repeat without parent edit", reason: notify.ReasonRepeatIntervalElapsed},
		{name: "no reason in context while editing", edit: true, omit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if !tt.omit {
				ctx = notify.WithNotificationReason(ctx, tt.reason)
			}
			require.Equal(t, tt.want, skipThreadReply(ctx, tt.edit))
		})
	}
}

type slackCall struct {
	endpoint string
	payload  map[string]any
}

func notifierRecordingCalls(t *testing.T, conf *config.SlackConfig, calls *[]slackCall, respTS string) *Notifier {
	t.Helper()
	u, err := url.Parse("https://slack.com/api/chat.postMessage")
	require.NoError(t, err)
	conf.APIURL = &amcommoncfg.SecretURL{URL: u}
	conf.Channel = "#test-channel"
	conf.HTTPConfig = &commoncfg.HTTPClientConfig{}

	notifier, err := New(conf, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, endpoint string, body io.Reader) (*http.Response, error) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(body).Decode(&payload))
		*calls = append(*calls, slackCall{endpoint: endpoint, payload: payload})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok": true, "channel": "C123", "ts": "` + respTS + `"}`)),
		}, nil
	}
	return notifier
}

func notifyCtx(store *nflog.Store, reason notify.NotifyReason) context.Context {
	ctx := notify.WithGroupKey(context.Background(), "test-group-key")
	ctx = notify.WithNflogStore(ctx, store)
	return notify.WithNotificationReason(ctx, reason)
}

func TestSlackThreadReplies(t *testing.T) {
	parentStore := func() *nflog.Store {
		store := nflog.NewStore(nil)
		store.SetStr("threadTs", "111.222")
		store.SetStr("channelId", "C123")
		return store
	}

	t.Run("first message is posted to the channel and remembered", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{UpdateMessage: true, ThreadReplies: true}, &calls, "111.222")
		store := nflog.NewStore(nil)

		_, err := n.Notify(notifyCtx(store, notify.ReasonFirstNotification))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.Equal(t, "https://slack.com/api/chat.postMessage", calls[0].endpoint)
		require.NotContains(t, calls[0].payload, "ts")
		require.NotContains(t, calls[0].payload, "thread_ts")
		ts, ok := store.GetStr("threadTs")
		require.True(t, ok)
		require.Equal(t, "111.222", ts)
		channel, ok := store.GetStr("channelId")
		require.True(t, ok)
		require.Equal(t, "C123", channel)
	})

	t.Run("resolve edits the parent and adds a reply", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{UpdateMessage: true, ThreadReplies: true}, &calls, "999.999")
		store := parentStore()

		_, err := n.Notify(notifyCtx(store, notify.ReasonAllAlertsResolved))
		require.NoError(t, err)

		require.Len(t, calls, 2)
		require.Equal(t, "https://slack.com/api/chat.update", calls[0].endpoint)
		require.Equal(t, "111.222", calls[0].payload["ts"])
		require.Equal(t, "C123", calls[0].payload["channel"])
		require.NotContains(t, calls[0].payload, "thread_ts")

		require.Equal(t, "https://slack.com/api/chat.postMessage", calls[1].endpoint)
		require.Equal(t, "111.222", calls[1].payload["thread_ts"])
		require.Equal(t, "C123", calls[1].payload["channel"])
		require.NotContains(t, calls[1].payload, "ts")

		ts, _ := store.GetStr("threadTs")
		require.Equal(t, "111.222", ts)
	})

	t.Run("repeat only edits the parent when both options are set", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{UpdateMessage: true, ThreadReplies: true}, &calls, "999.999")
		store := parentStore()

		_, err := n.Notify(notifyCtx(store, notify.ReasonRepeatIntervalElapsed))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.Equal(t, "https://slack.com/api/chat.update", calls[0].endpoint)
		require.Equal(t, "111.222", calls[0].payload["ts"])
		require.NotContains(t, calls[0].payload, "thread_ts")
	})

	t.Run("follow-up without update_message is a thread reply", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{ThreadReplies: true}, &calls, "999.999")
		store := parentStore()

		_, err := n.Notify(notifyCtx(store, notify.ReasonNewAlertsInGroup))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.Equal(t, "https://slack.com/api/chat.postMessage", calls[0].endpoint)
		require.Equal(t, "111.222", calls[0].payload["thread_ts"])
		require.NotContains(t, calls[0].payload, "ts")
		ts, _ := store.GetStr("threadTs")
		require.Equal(t, "111.222", ts)
	})

	t.Run("repeat without update_message still replies", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{ThreadReplies: true}, &calls, "999.999")
		store := parentStore()

		_, err := n.Notify(notifyCtx(store, notify.ReasonRepeatIntervalElapsed))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.Equal(t, "111.222", calls[0].payload["thread_ts"])
	})

	t.Run("update_message does not open a thread", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{UpdateMessage: true}, &calls, "999.999")
		store := parentStore()

		_, err := n.Notify(notifyCtx(store, notify.ReasonNewAlertsInGroup))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.Equal(t, "https://slack.com/api/chat.update", calls[0].endpoint)
		require.NotContains(t, calls[0].payload, "thread_ts")
	})

	t.Run("incomplete stored identity starts a new message", func(t *testing.T) {
		var calls []slackCall
		n := notifierRecordingCalls(t, &config.SlackConfig{ThreadReplies: true}, &calls, "333.444")
		store := nflog.NewStore(nil)
		store.SetStr("threadTs", "111.222")

		_, err := n.Notify(notifyCtx(store, notify.ReasonNewAlertsInGroup))
		require.NoError(t, err)

		require.Len(t, calls, 1)
		require.NotContains(t, calls[0].payload, "thread_ts")
		require.NotContains(t, calls[0].payload, "ts")
		ts, _ := store.GetStr("threadTs")
		require.Equal(t, "333.444", ts)
		channel, _ := store.GetStr("channelId")
		require.Equal(t, "C123", channel)
	})
}

func TestNotifyRejectsChangedAPIURLFile(t *testing.T) {
	urlFile := t.TempDir() + "/api_url"
	require.NoError(t, os.WriteFile(urlFile, []byte("https://hooks.slack.com/services/T/B/X\n"), 0o600))

	conf := &config.SlackConfig{
		APIURLFile:    urlFile,
		ThreadReplies: true,
		Channel:       "#test-channel",
		HTTPConfig:    &commoncfg.HTTPClientConfig{},
	}
	n, err := New(conf, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	called := false
	n.postJSONFunc = func(ctx context.Context, client *http.Client, endpoint string, body io.Reader) (*http.Response, error) {
		called = true
		return nil, nil
	}

	store := nflog.NewStore(nil)
	store.SetStr("threadTs", "111.222")
	store.SetStr("channelId", "C123")

	retry, err := n.Notify(notifyCtx(store, notify.ReasonNewAlertsInGroup))
	require.False(t, retry)
	require.EqualError(t, err, "thread_replies can only be used with bot tokens. api_url must be set to https://slack.com/api/chat.postMessage")
	require.False(t, called)
}
