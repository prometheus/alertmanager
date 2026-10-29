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

package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

const testRoomID = "!abc123:example.org"

// sentEvent is what the test homeserver received for one message.
type sentEvent struct {
	method, path, authorization string
	content                     map[string]any
}

// homeserver records the messages it receives and answers each with status
// and body.
func homeserver(t *testing.T, status int, body string) (*url.URL, func() []sentEvent) {
	t.Helper()
	var (
		mu   sync.Mutex
		sent []sentEvent
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := sentEvent{method: r.Method, path: r.URL.Path, authorization: r.Header.Get("Authorization")}
		if err := json.NewDecoder(r.Body).Decode(&event.content); err != nil {
			t.Errorf("decode the event content: %v", err)
		}
		mu.Lock()
		sent = append(sent, event)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return u, func() []sentEvent {
		mu.Lock()
		defer mu.Unlock()
		return append([]sentEvent(nil), sent...)
	}
}

func newNotifier(t *testing.T, cfg *MatrixConfig) *Notifier {
	t.Helper()
	cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
	if cfg.Message == "" {
		cfg.Message = DefaultMatrixConfig.Message
	}
	if cfg.MsgType == "" {
		cfg.MsgType = DefaultMatrixConfig.MsgType
	}
	if cfg.RoomID == "" {
		cfg.RoomID = testRoomID
	}
	n, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)
	return n
}

func notifyOne(t *testing.T, n *Notifier) notify.NotifyVerdict {
	t.Helper()
	ctx := notify.WithGroupKey(context.Background(), "1")
	return n.Notify(ctx, alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "DiskFull", "room": testRoomID},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(time.Hour),
	}, time.Time{}, false))
}

// TestMatrixRetry checks that the notifier retries on 5xx and on HTTP 429,
// and treats every other status code as a permanent failure.
func TestMatrixRetry(t *testing.T) {
	t.Parallel()
	u, _ := url.Parse("https://matrix.example.org")
	n := newNotifier(t, &MatrixConfig{HomeserverURL: &amcommoncfg.URL{URL: u}, AccessToken: "secret"})
	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := n.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "retry - error on status %d", statusCode)
	}
}

func TestMatrixSendsTheMessageToTheRoom(t *testing.T) {
	t.Parallel()
	u, sent := homeserver(t, http.StatusOK, `{"event_id":"$event"}`)
	n := newNotifier(t, &MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		AccessToken:   "secret",
		MsgType:       MsgTypeNotice,
	})

	for range 2 {
		verdict := notifyOne(t, n)
		require.NoError(t, verdict.Err())
	}

	events := sent()
	require.Len(t, events, 2)
	prefix := "/_matrix/client/v3/rooms/" + testRoomID + "/send/m.room.message/"
	for _, event := range events {
		require.Equal(t, http.MethodPut, event.method)
		require.True(t, strings.HasPrefix(event.path, prefix), "path %q", event.path)
		require.Equal(t, "Bearer secret", event.authorization)
		require.Equal(t, MsgTypeNotice, event.content["msgtype"])
		require.Contains(t, event.content["body"], "DiskFull")
		require.NotContains(t, event.content, "format")
		require.NotContains(t, event.content, "formatted_body")
	}
	// A homeserver drops a request that reuses a transaction id.
	require.NotEqual(t, events[0].path, events[1].path)
}

func TestMatrixSendsTheHTMLMessage(t *testing.T) {
	t.Parallel()
	u, sent := homeserver(t, http.StatusOK, `{"event_id":"$event"}`)
	n := newNotifier(t, &MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		AccessToken:   "secret",
		HTMLMessage:   `<b>{{ .CommonLabels.alertname }}</b> & co`,
	})
	require.NoError(t, notifyOne(t, n).Err())

	events := sent()
	require.Len(t, events, 1)
	require.Equal(t, "org.matrix.custom.html", events[0].content["format"])
	require.Equal(t, "<b>DiskFull</b> & co", events[0].content["formatted_body"])
}

func TestMatrixSendsPlainTextWhenTheHTMLMessageIsTooLong(t *testing.T) {
	t.Parallel()
	u, sent := homeserver(t, http.StatusOK, `{"event_id":"$event"}`)
	n := newNotifier(t, &MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		AccessToken:   "secret",
		HTMLMessage:   "<p>" + strings.Repeat("x", maxBodyBytes) + "</p>",
		Message:       strings.Repeat("y", maxBodyBytes+100),
	})
	require.NoError(t, notifyOne(t, n).Err())

	events := sent()
	require.Len(t, events, 1)
	require.NotContains(t, events[0].content, "formatted_body")
	require.LessOrEqual(t, len(events[0].content["body"].(string)), maxBodyBytes)
}

func TestMatrixReadsTheAccessTokenFile(t *testing.T) {
	t.Parallel()
	u, sent := homeserver(t, http.StatusOK, `{"event_id":"$event"}`)
	file := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(file, []byte("from-file\n"), 0o600))
	n := newNotifier(t, &MatrixConfig{HomeserverURL: &amcommoncfg.URL{URL: u}, AccessTokenFile: file})
	require.NoError(t, notifyOne(t, n).Err())
	require.Equal(t, "Bearer from-file", sent()[0].authorization)
}

func TestMatrixTemplatedRoomID(t *testing.T) {
	t.Parallel()
	u, sent := homeserver(t, http.StatusOK, `{"event_id":"$event"}`)
	n := newNotifier(t, &MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		AccessToken:   "secret",
		RoomID:        "{{ .CommonLabels.room }}",
	})
	require.NoError(t, notifyOne(t, n).Err())
	require.True(t, strings.HasPrefix(sent()[0].path, "/_matrix/client/v3/rooms/"+testRoomID+"/"))

	n = newNotifier(t, &MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		AccessToken:   "secret",
		RoomID:        "{{ .CommonLabels.missing }}",
	})
	verdict := notifyOne(t, n)
	require.ErrorContains(t, verdict.Err(), "is not a room ID")
	require.False(t, verdict.ShouldRetry())
	require.Len(t, sent(), 1)
}

func TestMatrixRateLimit(t *testing.T) {
	t.Parallel()
	u, _ := homeserver(t, http.StatusTooManyRequests, `{"errcode":"M_LIMIT_EXCEEDED","error":"Too many requests","retry_after_ms":1500}`)
	n := newNotifier(t, &MatrixConfig{HomeserverURL: &amcommoncfg.URL{URL: u}, AccessToken: "secret"})

	verdict := notifyOne(t, n)
	require.Error(t, verdict.Err())
	require.True(t, verdict.ShouldRetry())
	require.Equal(t, 1500*time.Millisecond, verdict.Delay())
	require.Equal(t, notify.RateLimitedReason, verdict.Reason())
}

func TestMatrixRetryAfterHeader(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("Retry-After", "3")
	require.Equal(t, 3*time.Second, retryAfter(h, []byte(`{"retry_after_ms":1500}`), time.Now()))
	require.Equal(t, 1500*time.Millisecond, retryAfter(http.Header{}, []byte(`{"retry_after_ms":1500}`), time.Now()))
	require.Zero(t, retryAfter(http.Header{}, []byte(`not json`), time.Now()))
}

func TestMatrixForbiddenIsNotRetried(t *testing.T) {
	t.Parallel()
	u, _ := homeserver(t, http.StatusForbidden, `{"errcode":"M_FORBIDDEN","error":"User not in room"}`)
	n := newNotifier(t, &MatrixConfig{HomeserverURL: &amcommoncfg.URL{URL: u}, AccessToken: "secret"})

	verdict := notifyOne(t, n)
	require.ErrorContains(t, verdict.Err(), "User not in room")
	require.False(t, verdict.ShouldRetry())
}

func TestMatrixRedactedAccessToken(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	const token = "secret_access_token"
	n, err := New(&MatrixConfig{
		HomeserverURL: &amcommoncfg.URL{URL: u},
		HTTPConfig:    &commoncfg.HTTPClientConfig{},
		AccessToken:   commoncfg.Secret(token),
		RoomID:        testRoomID,
		Message:       DefaultMatrixConfig.Message,
		MsgType:       MsgTypeText,
	}, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, n, token)
}
