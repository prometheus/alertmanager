// Copyright 2022 Prometheus Team
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

package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestTelegramUnmarshal(t *testing.T) {
	in := `
  chat_id: 1234
  bot_token: secret
  message_thread_id: 1357
  api_url: https://api.telegram.org
`
	var c TelegramConfig
	err := yaml.Unmarshal([]byte(in), &c)
	require.NoError(t, err)

	require.Equal(t, "https://api.telegram.org", c.APIUrl.String())
	require.Equal(t, commoncfg.Secret("secret"), c.BotToken)
	require.Equal(t, int64(1234), c.ChatID)
	require.Equal(t, 1357, c.MessageThreadID)
	require.Equal(t, "HTML", c.ParseMode)
}

func TestTelegramRetry(t *testing.T) {
	// Fake url for testing purposes
	fakeURL := amcommoncfg.URL{
		URL: &url.URL{
			Scheme: "https",
			Host:   "FAKE_API",
		},
	}
	notifier, err := New(
		&TelegramConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIUrl:     &fakeURL,
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

func TestTelegramNotify(t *testing.T) {
	token := "secret"
	htmlLinkWithSize := func(size int) string {
		const prefix = `<a href="https://example.com/`
		const suffix = `">Open</a>`

		return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
	}
	longHTMLLink := `<a href="https://example.com/` + strings.Repeat("x", maxMessageLenRunes) + `">Open</a>`
	htmlAtInputLimit := htmlLinkWithSize(1 << 15)
	htmlOverInputLimit := htmlLinkWithSize(1<<15 + 1)
	longMalformedHTML := `<a href="` + strings.Repeat("x", maxMessageLenRunes)
	longMalformedHTMLTemplate := fmt.Sprintf(`{{ %q | safeHtml }}`, longMalformedHTML)
	longMalformedComment := `<!--` + strings.Repeat("x", maxMessageLenRunes)
	longMalformedCommentTemplate := fmt.Sprintf(`{{ %q | safeHtml }}`, longMalformedComment)
	longMalformedDoctype := `<!DOCTYPE ` + strings.Repeat("x", maxMessageLenRunes)
	longMalformedDoctypeTemplate := fmt.Sprintf(`{{ %q | safeHtml }}`, longMalformedDoctype)

	fileWithToken, err := os.CreateTemp(t.TempDir(), "telegram-bot-token")
	require.NoError(t, err, "creating temp file failed")
	_, err = fileWithToken.WriteString(token)
	require.NoError(t, err, "writing to temp file failed")

	for _, tc := range []struct {
		name    string
		cfg     TelegramConfig
		expText string
	}{
		{
			name: "No escaping by default",
			cfg: TelegramConfig{
				Message:    "<code>x < y</code>",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: "<code>x < y</code>",
		},
		{
			name: "Characters escaped in HTML mode",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    "<code>x < y</code>",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: "<code>x &lt; y</code>",
		},
		{
			name: "Bot token from file",
			cfg: TelegramConfig{
				Message:      "test",
				HTTPConfig:   &commoncfg.HTTPClientConfig{},
				BotTokenFile: fileWithToken.Name(),
			},
			expText: "test",
		},
		{
			name: "HTML mode with too-large message",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    strings.Repeat("x", 5000),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "HTML mode ignores link targets when enforcing length limit",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    longHTMLLink,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: longHTMLLink,
		},
		{
			name: "HTML mode accepts raw input at Telegram limit",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    htmlAtInputLimit,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: htmlAtInputLimit,
		},
		{
			name: "HTML mode falls back for raw input over Telegram limit",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    htmlOverInputLimit,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "HTML mode falls back for too-large malformed tag",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    longMalformedHTMLTemplate,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "HTML mode falls back for too-large malformed comment",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    longMalformedCommentTemplate,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "HTML mode falls back for too-large malformed doctype",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    longMalformedDoctypeTemplate,
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`,
		},
		{
			name: "Default mode with too-large message",
			cfg: TelegramConfig{
				Message:    strings.Repeat("y", 5000),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("y", maxMessageLenRunes-1) + "…",
		},
		{
			name: "HTML mode with message smaller than limit",
			cfg: TelegramConfig{
				ParseMode:  "HTML",
				Message:    strings.Repeat("a", 100),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("a", 100),
		},
		{
			name: "Default mode with message smaller than limit",
			cfg: TelegramConfig{
				Message:    strings.Repeat("b", 100),
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
			},
			expText: strings.Repeat("b", 100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/bot"+token+"/sendMessage", r.URL.Path)
				var err error
				out, err = io.ReadAll(r.Body)
				require.NoError(t, err)
				w.Write([]byte(`{"ok":true,"result":{"chat":{}}}`))
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			tc.cfg.APIUrl = &amcommoncfg.URL{URL: u}

			notifier, err := New(&tc.cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "1")

			verdict := notifier.Notify(ctx, []*alert.Alert{
				alert.New(model.Alert{
					Labels: model.LabelSet{
						"lbl1": "val1",
						"lbl3": "val3",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				}, time.Time{}, false),
			}...)
			require.False(t, verdict.ShouldRetry())
			require.NoError(t, verdict.Err())

			req := map[string]string{}
			err = json.Unmarshal(out, &req)
			require.NoError(t, err)
			require.Equal(t, tc.expText, req["text"])
		})
	}
}

func TestHTMLTextRuneCount(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{
		{input: `&lt;`, want: 1},
		{input: `&gt;`, want: 1},
		{input: `&amp;`, want: 1},
		{input: `&quot;`, want: 1},
		{input: `&#39;`, want: 1},
		{input: `&#x27;`, want: 1},
		{input: `&#128293;`, want: 1},
		{input: `&#x1F525;`, want: 1},
		{input: `&nbsp;`, want: 6},
		{input: `&mdash;`, want: 7},
		{input: `&apos;`, want: 6},
		{input: `&copy;`, want: 6},
		{input: `&hellip;`, want: 8},
		{input: `&nbspa`, want: 6},
		{input: `&nbsp `, want: 6},
		{input: `&ampa`, want: 5},
		{input: `&amp `, want: 2},
		{input: `&#0;`, want: 4},
		{input: `&#1114112;`, want: 10},
		{input: `a & b`, want: 5},
	} {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.want, htmlTextRuneCount(tc.input))
		})
	}
}

func TestTelegramNotifyFailureReason(t *testing.T) {
	token := "secret"

	for _, tc := range []struct {
		name           string
		response       string
		expectedReason notify.Reason
	}{
		{
			name:           "Unauthorized is classified as auth error",
			response:       `{"ok":false,"error_code":401,"description":"Unauthorized"}`,
			expectedReason: notify.AuthErrorReason,
		},
		{
			name:           "Blocked by user is classified as auth error",
			response:       `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`,
			expectedReason: notify.AuthErrorReason,
		},
		{
			name:           "Chat not found is classified as client error",
			response:       `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			expectedReason: notify.ClientErrorReason,
		},
		{
			name:           "Internal server error is classified as server error",
			response:       `{"ok":false,"error_code":500,"description":"Internal Server Error"}`,
			expectedReason: notify.ServerErrorReason,
		},
		{
			name:           "Flood control is classified as rate limited",
			response:       `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 5","parameters":{"retry_after":5}}`,
			expectedReason: notify.RateLimitedReason,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.response))
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			cfg := TelegramConfig{
				Message:    "test",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
				APIUrl:     &amcommoncfg.URL{URL: u},
			}

			notifier, err := New(&cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = notify.WithGroupKey(ctx, "1")

			verdict := notifier.Notify(ctx, []*alert.Alert{
				alert.New(model.Alert{
					Labels:   model.LabelSet{"lbl1": "val1"},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				}, time.Time{}, false),
			}...)
			require.True(t, verdict.ShouldRetry())
			require.Error(t, verdict.Err())
			require.Equal(t, tc.expectedReason, verdict.Reason())
		})
	}
}

// TestTelegramNotifyRedactURL verifies that notify.RedactURL is applied to the
// error returned by client.Send, so the bot token is never exposed in logs.
func TestTelegramNotifyRedactURL(t *testing.T) {
	token := "secret"

	t.Run("transport error redacts URL", func(t *testing.T) {
		// Point at a closed server so telebot returns a *url.Error with the full URL.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		u, _ := url.Parse(srv.URL)
		srv.Close() // closed immediately — next dial will fail

		notifier, err := New(
			&TelegramConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				APIUrl:     &amcommoncfg.URL{URL: u},
				BotToken:   commoncfg.Secret(token),
			},
			test.CreateTmpl(t),
			promslog.NewNopLogger(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = notify.WithGroupKey(ctx, "1")

		verdict := notifier.Notify(ctx, alert.New(model.Alert{Labels: model.LabelSet{"alertname": "test"}}, time.Time{}, false))
		require.True(t, verdict.ShouldRetry())
		require.Error(t, verdict.Err())
		// The token must not appear in the error string.
		require.NotContains(t, verdict.Err().Error(), token, "bot token leaked in transport error")
		// The URL should be redacted.
		require.Contains(t, verdict.Err().Error(), "<redacted>")
	})

	t.Run("Telegram API error passes through without token", func(t *testing.T) {
		// Return a Telegram API error response — telebot wraps this as its own
		// error type (not *url.Error), so RedactURL is a no-op, but the token
		// is not in the error string either.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found","error_code":400}`))
		}))
		defer srv.Close()
		u, _ := url.Parse(srv.URL)

		notifier, err := New(
			&TelegramConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				APIUrl:     &amcommoncfg.URL{URL: u},
				BotToken:   commoncfg.Secret(token),
			},
			test.CreateTmpl(t),
			promslog.NewNopLogger(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = notify.WithGroupKey(ctx, "1")

		verdict := notifier.Notify(ctx, alert.New(model.Alert{Labels: model.LabelSet{"alertname": "test"}}, time.Time{}, false))
		require.True(t, verdict.ShouldRetry())
		require.Error(t, verdict.Err())
		require.NotContains(t, verdict.Err().Error(), token, "bot token leaked in API error")
	})
}

func TestTelegramTimeout(t *testing.T) {
	token := "secret"

	tests := []struct {
		name    string
		latency time.Duration
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "success",
			latency: 100 * time.Millisecond,
			timeout: 120 * time.Millisecond,
			wantErr: false,
		},
		{
			name:    "timeout",
			latency: 100 * time.Millisecond,
			timeout: 80 * time.Millisecond,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/bot"+token+"/sendMessage", r.URL.Path)
				_, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				time.Sleep(tc.latency)
				w.Write([]byte(`{"ok":true,"result":{"chat":{}}}`))
			}))
			defer srv.Close()
			u, _ := url.Parse(srv.URL)

			cfg := &TelegramConfig{
				Message:    "test",
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				BotToken:   commoncfg.Secret(token),
				Timeout:    tc.timeout,
				APIUrl:     &amcommoncfg.URL{URL: u},
			}

			notifier, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			testAlert := alert.New(model.Alert{
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			}, time.Time{}, false)

			err = notifier.Notify(ctx, testAlert).Err()
			require.Equal(t, tc.wantErr, err != nil)
			if tc.wantErr {
				require.EqualError(t, err, fmt.Sprintf("configured telegram timeout reached (%s)", tc.timeout))
			}
		})
	}
}

func TestTelegramUpdateMessage(t *testing.T) {
	type capturedRequest struct {
		path string
		body map[string]string
	}

	newNotifier := func(t *testing.T, update bool, handler http.HandlerFunc) *Notifier {
		t.Helper()
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		notifier, err := New(&TelegramConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{VSendResolved: true},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIUrl:          &amcommoncfg.URL{URL: u},
			BotToken:        "secret",
			ChatID:          42,
			Message:         "{{ .Status }}",
			ParseMode:       "HTML",
			UpdateMessage:   update,
			MessageThreadID: 99,
		}, test.CreateTmpl(t), promslog.NewNopLogger())
		require.NoError(t, err)
		return notifier
	}

	record := func(t *testing.T, reqs *[]capturedRequest, respond func(http.ResponseWriter, capturedRequest)) http.HandlerFunc {
		t.Helper()
		return func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			decoded := map[string]string{}
			require.NoError(t, json.Unmarshal(body, &decoded))
			req := capturedRequest{path: r.URL.Path, body: decoded}
			*reqs = append(*reqs, req)
			respond(w, req)
		}
	}

	okMessage := func(w http.ResponseWriter, id int) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"chat":{"id":42}}}`, id)
	}

	apiError := func(w http.ResponseWriter, description string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"ok":false,"error_code":400,"description":%q}`, description)
	}

	ctxWithStore := func(store *nflog.Store) context.Context {
		ctx := notify.WithGroupKey(context.Background(), "group")
		return notify.WithNflogStore(ctx, store)
	}

	alertAt := func(resolved bool) *alert.Alert {
		a := model.Alert{
			Labels:   model.LabelSet{"alertname": "test"},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		}
		if resolved {
			a.EndsAt = time.Now().Add(-time.Minute)
		}
		return alert.New(a, time.Now(), false)
	}

	t.Run("disabled sends a new message and does not store an id", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, false, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 1)
		}))
		store := nflog.NewStore(nil)

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(false)).Err())

		require.Len(t, reqs, 1)
		require.Equal(t, "99", reqs[0].body["message_thread_id"])
		_, found := store.GetStr(telegramMessageIDKey)
		require.False(t, found)
	})

	t.Run("first notification stores the message id", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 7)
		}))
		store := nflog.NewStore(nil)

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(false)).Err())

		require.Len(t, reqs, 1)
		require.Equal(t, "HTML", reqs[0].body["parse_mode"])
		stored, found := store.GetStr(telegramMessageIDKey)
		require.True(t, found)
		require.Equal(t, "7", stored)
		storedChat, found := store.GetStr(telegramChatIDKey)
		require.True(t, found)
		require.Equal(t, "42", storedChat)
	})

	t.Run("subsequent notification edits the stored message", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/editMessageText")
			okMessage(w, 7)
		}))
		store := nflog.NewStore(nil)
		store.SetStr(telegramMessageIDKey, "7")
		store.SetStr(telegramChatIDKey, "42")

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(true)).Err())

		require.Len(t, reqs, 1)
		require.Equal(t, "7", reqs[0].body["message_id"])
		require.Equal(t, "42", reqs[0].body["chat_id"])
		require.Equal(t, "HTML", reqs[0].body["parse_mode"])
		require.NotContains(t, reqs[0].body, "message_thread_id")
		stored, _ := store.GetStr(telegramMessageIDKey)
		require.Equal(t, "7", stored)
	})

	t.Run("unchanged message is a successful edit", func(t *testing.T) {
		for _, description := range []string{
			"Bad Request: message is not modified",
			"Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message",
		} {
			t.Run(description, func(t *testing.T) {
				var reqs []capturedRequest
				notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
					apiError(w, description)
				}))
				store := nflog.NewStore(nil)
				store.SetStr(telegramMessageIDKey, "7")
				store.SetStr(telegramChatIDKey, "42")

				verdict := notifier.Notify(ctxWithStore(store), alertAt(false))
				require.NoError(t, verdict.Err())
				require.False(t, verdict.ShouldRetry())
				require.Len(t, reqs, 1)
				require.Contains(t, reqs[0].path, "/editMessageText")
				stored, _ := store.GetStr(telegramMessageIDKey)
				require.Equal(t, "7", stored)
			})
		}
	})

	t.Run("deleted message is replaced", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			if strings.Contains(req.path, "/editMessageText") {
				apiError(w, "Bad Request: message to edit not found")
				return
			}
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 9)
		}))
		store := nflog.NewStore(nil)
		store.SetStr(telegramMessageIDKey, "7")
		store.SetStr(telegramChatIDKey, "42")

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(false)).Err())

		require.Len(t, reqs, 2)
		require.Contains(t, reqs[0].path, "/editMessageText")
		require.Contains(t, reqs[1].path, "/sendMessage")
		require.Equal(t, "99", reqs[1].body["message_thread_id"])
		stored, found := store.GetStr(telegramMessageIDKey)
		require.True(t, found)
		require.Equal(t, "9", stored)
		storedChat, found := store.GetStr(telegramChatIDKey)
		require.True(t, found)
		require.Equal(t, "42", storedChat)
	})

	t.Run("chat id change sends a new message", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 8)
		}))
		store := nflog.NewStore(nil)
		store.SetStr(telegramMessageIDKey, "7")
		store.SetStr(telegramChatIDKey, "100")

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(false)).Err())

		require.Len(t, reqs, 1)
		require.Equal(t, "42", reqs[0].body["chat_id"])
		stored, _ := store.GetStr(telegramMessageIDKey)
		require.Equal(t, "8", stored)
		storedChat, _ := store.GetStr(telegramChatIDKey)
		require.Equal(t, "42", storedChat)
	})

	t.Run("message that cannot be edited is replaced", func(t *testing.T) {
		for _, description := range []string{
			"Bad Request: message can't be edited",
			"Bad Request: MESSAGE_ID_INVALID",
		} {
			t.Run(description, func(t *testing.T) {
				var reqs []capturedRequest
				notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
					if strings.Contains(req.path, "/editMessageText") {
						apiError(w, description)
						return
					}
					require.Contains(t, req.path, "/sendMessage")
					okMessage(w, 9)
				}))
				store := nflog.NewStore(nil)
				store.SetStr(telegramMessageIDKey, "7")
				store.SetStr(telegramChatIDKey, "42")

				verdict := notifier.Notify(ctxWithStore(store), alertAt(false))
				require.NoError(t, verdict.Err())
				require.False(t, verdict.ShouldRetry())
				require.Len(t, reqs, 2)
				require.Contains(t, reqs[0].path, "/editMessageText")
				require.Contains(t, reqs[1].path, "/sendMessage")
				stored, _ := store.GetStr(telegramMessageIDKey)
				require.Equal(t, "9", stored)
			})
		}
	})

	t.Run("temporary edit error is retried without a new message", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/editMessageText")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"ok":false,"error_code":500,"description":"Internal Server Error"}`)
		}))
		store := nflog.NewStore(nil)
		store.SetStr(telegramMessageIDKey, "7")
		store.SetStr(telegramChatIDKey, "42")

		verdict := notifier.Notify(ctxWithStore(store), alertAt(false))
		require.Error(t, verdict.Err())
		require.True(t, verdict.ShouldRetry())
		require.Len(t, reqs, 1)
		stored, _ := store.GetStr(telegramMessageIDKey)
		require.Equal(t, "7", stored)
		storedChat, _ := store.GetStr(telegramChatIDKey)
		require.Equal(t, "42", storedChat)
	})

	t.Run("missing notification log sends a new message", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 1)
		}))
		ctx := notify.WithGroupKey(context.Background(), "group")

		require.NoError(t, notifier.Notify(ctx, alertAt(false)).Err())
		require.Len(t, reqs, 1)
	})

	t.Run("invalid stored id sends a new message", func(t *testing.T) {
		var reqs []capturedRequest
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			require.Contains(t, req.path, "/sendMessage")
			okMessage(w, 4)
		}))
		store := nflog.NewStore(nil)
		store.SetStr(telegramMessageIDKey, "nope")

		require.NoError(t, notifier.Notify(ctxWithStore(store), alertAt(false)).Err())

		require.Len(t, reqs, 1)
		stored, _ := store.GetStr(telegramMessageIDKey)
		require.Equal(t, "4", stored)
	})

	t.Run("resolution edits and the next firing starts a new message", func(t *testing.T) {
		var reqs []capturedRequest
		var nextID int
		notifier := newNotifier(t, true, record(t, &reqs, func(w http.ResponseWriter, req capturedRequest) {
			if strings.Contains(req.path, "/sendMessage") {
				nextID++
				okMessage(w, nextID)
				return
			}
			require.Contains(t, req.path, "/editMessageText")
			okMessage(w, 0)
		}))

		nl, err := nflog.New(nflog.Options{Retention: time.Hour, Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		recv := &nflogpb.Receiver{GroupName: "test", Integration: "telegram"}
		dedup := notify.NewDedupStage(notifier.conf, nl, recv)
		setNotifies := notify.NewSetNotifiesStage(nl, recv, featurecontrol.NoopFlags{})

		notifyGroup := func(resolved bool) []capturedRequest {
			t.Helper()
			reqs = nil
			ctx := notify.WithGroupKey(context.Background(), "group")
			ctx = notify.WithRepeatInterval(ctx, time.Hour)
			ctx, alerts, err := dedup.Exec(ctx, promslog.NewNopLogger(), alertAt(resolved))
			require.NoError(t, err)
			require.NoError(t, notifier.Notify(ctx, alerts...).Err())
			_, _, err = setNotifies.Exec(ctx, promslog.NewNopLogger(), alerts...)
			require.NoError(t, err)
			return reqs
		}

		got := notifyGroup(false)
		require.Len(t, got, 1)
		require.Contains(t, got[0].path, "/sendMessage")

		got = notifyGroup(true)
		require.Len(t, got, 1)
		require.Contains(t, got[0].path, "/editMessageText")
		require.Equal(t, "1", got[0].body["message_id"])

		got = notifyGroup(false)
		require.Len(t, got, 1)
		require.Contains(t, got[0].path, "/sendMessage")

		got = notifyGroup(true)
		require.Len(t, got, 1)
		require.Contains(t, got[0].path, "/editMessageText")
		require.Equal(t, "2", got[0].body["message_id"])
	})
}
