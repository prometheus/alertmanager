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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
	"github.com/prometheus/alertmanager/types"
)

func TestSlackThreadMessage(t *testing.T) {
	var capturedRequests []request
	var capturedURLs []string

	apiurl, _ := url.Parse("https://slack.com/api/chat.postMessage")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		b, _ := io.ReadAll(body)
		var req request
		require.NoError(t, json.Unmarshal(b, &req))
		capturedRequests = append(capturedRequests, req)
		capturedURLs = append(capturedURLs, u)

		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusOK)
		resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
		return resp.Result(), nil
	}

	alert1 := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}

	// First notification: should produce 2 calls (summary parent + first reply).
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	store := nflog.NewStore(nil)
	ctx = notify.WithNflogStore(ctx, store)
	ctx = notify.WithNotificationReason(ctx, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx, alert1)
	require.NoError(t, err)

	require.Len(t, capturedRequests, 2, "first notification: parent summary + reply")
	require.Empty(t, capturedRequests[0].Text, "parent summary should have no text, only attachment")
	require.Empty(t, capturedRequests[0].ThreadTimestamp, "parent should NOT have thread_ts")
	require.Equal(t, "1700000000.000100", capturedRequests[1].ThreadTimestamp, "first reply should thread into parent")

	parentTs, found := store.GetStr("threadTs")
	require.True(t, found)
	require.Equal(t, "1700000000.000100", parentTs)

	// Second notification: should produce 2 calls (reply + parent update).
	capturedRequests = nil
	capturedURLs = nil

	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "1")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonNewAlertsInGroup)

	_, err = notifier.Notify(ctx2, alert1)
	require.NoError(t, err)

	require.Len(t, capturedRequests, 2, "second notification: reply + parent update")
	require.Equal(t, "1700000000.000100", capturedRequests[0].ThreadTimestamp, "reply should thread into parent")
	require.Contains(t, capturedURLs[1], "/chat.update", "second call should update parent")

	storedTs, _ := store.GetStr("threadTs")
	require.Equal(t, "1700000000.000100", storedTs, "threadTs should still reference the original parent")
}

// TestSlackThreadSummaryHeaderTransitionsNotDuplicatedOnRetry ensures transition
// history is only written after a full successful notify. If the threaded reply
// fails while the parent message already succeeded, a subsequent notify must not
// append the same transition twice.
func TestSlackThreadSummaryHeaderTransitionsNotDuplicatedOnRetry(t *testing.T) {
	apiurl, err := url.Parse("https://slack.com/api/chat.postMessage")
	require.NoError(t, err)
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	var replyAttempts int
	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		b, errRead := io.ReadAll(body)
		require.NoError(t, errRead)
		var req request
		require.NoError(t, json.Unmarshal(b, &req))

		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")

		if req.ThreadTimestamp == "" {
			resp.WriteHeader(http.StatusOK)
			_, errWrite := resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
			require.NoError(t, errWrite)
			return resp.Result(), nil
		}

		replyAttempts++
		if replyAttempts == 1 {
			resp.WriteHeader(http.StatusOK)
			_, errWrite := resp.WriteString(`{"ok":false,"error":"ratelimited"}`)
			require.NoError(t, errWrite)
			return resp.Result(), nil
		}
		resp.WriteHeader(http.StatusOK)
		_, errWrite := resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000200"}`)
		require.NoError(t, errWrite)
		return resp.Result(), nil
	}

	alert := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}
	store := nflog.NewStore(nil)

	ctx1 := context.Background()
	ctx1 = notify.WithGroupKey(ctx1, "group-thread-retry")
	ctx1 = notify.WithNflogStore(ctx1, store)
	ctx1 = notify.WithNotificationReason(ctx1, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx1, alert)
	require.Error(t, err)
	_, ok := store.GetStr("transitions")
	require.False(t, ok, "transitions must not be stored when reply fails")

	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "group-thread-retry")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx2, alert)
	require.NoError(t, err)
	transitions, ok := store.GetStr("transitions")
	require.True(t, ok)
	require.Equal(t, "FIRING", transitions)
	require.Equal(t, 2, replyAttempts)
}

func TestSlackThreadResolveEmoji(t *testing.T) {
	var capturedURLs []string
	var capturedBodies [][]byte

	apiurl, _ := url.Parse("https://slack.com/api/chat.postMessage")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{VSendResolved: true},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
			ThreadedOptions: &config.SlackThreadedOptions{ResolveEmoji: "white_check_mark"},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		b, _ := io.ReadAll(body)
		capturedURLs = append(capturedURLs, u)
		capturedBodies = append(capturedBodies, b)

		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusOK)
		resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
		return resp.Result(), nil
	}

	alert1 := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}

	// First call: firing alert.
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	store := nflog.NewStore(nil)
	ctx = notify.WithNflogStore(ctx, store)
	ctx = notify.WithNotificationReason(ctx, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx, alert1)
	require.NoError(t, err)
	require.Len(t, capturedURLs, 2, "first notification: parent + reply")

	// Second call: resolved -- reply + update + emoji.
	capturedURLs = nil
	capturedBodies = nil

	resolvedAlert := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(-time.Minute)},
	}

	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "1")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonAllAlertsResolved)

	_, err = notifier.Notify(ctx2, resolvedAlert)
	require.NoError(t, err)

	// Expect 3 calls: reply + parent update + reactions.add.
	require.Len(t, capturedURLs, 3)
	require.Contains(t, capturedURLs[0], "/chat.postMessage")
	require.Contains(t, capturedURLs[1], "/chat.update")
	require.Contains(t, capturedURLs[2], "/reactions.add")

	var reaction reactionRequest
	require.NoError(t, json.Unmarshal(capturedBodies[2], &reaction))
	require.Equal(t, "C123ABC", reaction.Channel)
	require.Equal(t, "1700000000.000100", reaction.Timestamp)
	require.Equal(t, "white_check_mark", reaction.Name)
}

func TestSlackThreadResolveEmojiNotSentWhenFiring(t *testing.T) {
	var capturedURLs []string

	apiurl, _ := url.Parse("https://slack.com/api/chat.postMessage")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
			ThreadedOptions: &config.SlackThreadedOptions{ResolveEmoji: "white_check_mark"},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		capturedURLs = append(capturedURLs, u)
		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusOK)
		resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
		return resp.Result(), nil
	}

	alert1 := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}

	// First notification (firing): parent + reply, no reaction.
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	store := nflog.NewStore(nil)
	ctx = notify.WithNflogStore(ctx, store)
	ctx = notify.WithNotificationReason(ctx, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx, alert1)
	require.NoError(t, err)
	require.Len(t, capturedURLs, 2, "first: parent + reply, no reaction")
	for _, u := range capturedURLs {
		require.NotContains(t, u, "reactions.add")
	}

	// Second notification (firing): reply + update, no reaction.
	capturedURLs = nil
	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "1")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonNewAlertsInGroup)

	_, err = notifier.Notify(ctx2, alert1)
	require.NoError(t, err)
	require.Len(t, capturedURLs, 2, "second: reply + update, no reaction")
	for _, u := range capturedURLs {
		require.NotContains(t, u, "reactions.add")
	}
}

func TestSlackThreadDirectMode(t *testing.T) {
	var capturedRequests []request
	var capturedURLs []string
	useSummary := false

	apiurl, _ := url.Parse("https://slack.com/api/chat.postMessage")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
			ThreadedOptions: &config.SlackThreadedOptions{UseSummaryHeader: &useSummary},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		b, _ := io.ReadAll(body)
		var req request
		require.NoError(t, json.Unmarshal(b, &req))
		capturedRequests = append(capturedRequests, req)
		capturedURLs = append(capturedURLs, u)

		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusOK)
		resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
		return resp.Result(), nil
	}

	alert1 := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}

	// First notification: single call -- the alert itself is the parent.
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	store := nflog.NewStore(nil)
	ctx = notify.WithNflogStore(ctx, store)
	ctx = notify.WithNotificationReason(ctx, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx, alert1)
	require.NoError(t, err)

	require.Len(t, capturedRequests, 1, "direct mode: first notification is 1 call")
	require.Empty(t, capturedRequests[0].ThreadTimestamp, "parent should NOT have thread_ts")
	require.NotEmpty(t, capturedRequests[0].Attachments, "direct mode first message should have alert content")

	parentTs, found := store.GetStr("threadTs")
	require.True(t, found)
	require.Equal(t, "1700000000.000100", parentTs)

	// Second notification: single call -- threaded reply, no chat.update.
	capturedRequests = nil
	capturedURLs = nil

	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "1")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonNewAlertsInGroup)

	_, err = notifier.Notify(ctx2, alert1)
	require.NoError(t, err)

	require.Len(t, capturedRequests, 1, "direct mode: second notification is 1 call (reply only)")
	require.Equal(t, "1700000000.000100", capturedRequests[0].ThreadTimestamp, "reply should thread into parent")
	for _, u := range capturedURLs {
		require.NotContains(t, u, "/chat.update", "direct mode should NOT update parent")
	}

	storedTs, _ := store.GetStr("threadTs")
	require.Equal(t, "1700000000.000100", storedTs, "threadTs should still reference the original parent")
}

func TestSlackThreadDirectModeResolveEmoji(t *testing.T) {
	var capturedURLs []string
	var capturedBodies [][]byte
	useSummary := false

	apiurl, _ := url.Parse("https://slack.com/api/chat.postMessage")
	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{VSendResolved: true},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
			ThreadedOptions: &config.SlackThreadedOptions{
				UseSummaryHeader: &useSummary,
				ResolveEmoji:     "white_check_mark",
			},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		b, _ := io.ReadAll(body)
		capturedURLs = append(capturedURLs, u)
		capturedBodies = append(capturedBodies, b)

		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusOK)
		resp.WriteString(`{"ok":true,"channel":"C123ABC","ts":"1700000000.000100"}`)
		return resp.Result(), nil
	}

	alert1 := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
	}

	// First call: firing alert (direct parent).
	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")
	store := nflog.NewStore(nil)
	ctx = notify.WithNflogStore(ctx, store)
	ctx = notify.WithNotificationReason(ctx, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx, alert1)
	require.NoError(t, err)
	require.Len(t, capturedURLs, 1, "direct mode: 1 call for firing")

	// Second call: resolved -- reply + emoji (no chat.update).
	capturedURLs = nil
	capturedBodies = nil

	resolvedAlert := &types.Alert{
		Alert: model.Alert{StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(-time.Minute)},
	}

	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "1")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonAllAlertsResolved)

	_, err = notifier.Notify(ctx2, resolvedAlert)
	require.NoError(t, err)

	require.Len(t, capturedURLs, 2, "direct mode resolve: reply + emoji")
	require.Contains(t, capturedURLs[0], "/chat.postMessage")
	require.Contains(t, capturedURLs[1], "/reactions.add")

	var reaction reactionRequest
	require.NoError(t, json.Unmarshal(capturedBodies[1], &reaction))
	require.Equal(t, "C123ABC", reaction.Channel)
	require.Equal(t, "1700000000.000100", reaction.Timestamp)
	require.Equal(t, "white_check_mark", reaction.Name)
}

func TestBuildTransitionTitle(t *testing.T) {
	tests := []struct {
		transitions string
		alertName   string
		expected    string
	}{
		{"FIRING|RESOLVED", "HighCPU", "FIRING → RESOLVED: HighCPU"},
		{"FIRING|FIRING|RESOLVED", "HighCPU", "FIRING (2) → RESOLVED: HighCPU"},
		{"FIRING|FIRING|PARTIAL RESOLVE|FIRING|RESOLVED", "HighCPU", "FIRING (2) → PARTIAL RESOLVE → FIRING → RESOLVED: HighCPU"},
		{"FIRING|FIRING|FIRING|RESOLVED", "Mem", "FIRING (3) → RESOLVED: Mem"},
		{"FIRING", "X", "FIRING: X"},
		{"", "X", "X"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := buildTransitionTitle(tt.transitions, tt.alertName)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestReadAndParseSlackResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		resp                *http.Response
		opts                slackResponseOpts
		wantRetry           bool
		wantErr             bool
		wantErrSub          string
		wantChannel         string
		wantTS              string
		wantIgnoredSlackErr string
	}{
		{
			name: "json ok",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"channel":"C1","ts":"1.0"}`)),
			},
			wantChannel: "C1",
			wantTS:      "1.0",
		},
		{
			name: "json error",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error":"channel_not_found"}`)),
			},
			wantErr:    true,
			wantErrSub: "channel_not_found",
		},
		{
			name: "json error ignored",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error":"already_reacted"}`)),
			},
			opts:                slackResponseOpts{IgnoreAPIErrors: []string{"already_reacted"}},
			wantIgnoredSlackErr: "already_reacted",
		},
		{
			name: "json error not ignored",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error":"already_reacted"}`)),
			},
			wantErr:    true,
			wantErrSub: "already_reacted",
		},
		{
			name: "plaintext ok",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("ok")),
			},
		},
		{
			name: "plaintext ok with trailing newline",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("ok\n")),
			},
		},
		{
			name: "json content type case insensitive",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"Application/JSON; charset=UTF-8"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"channel":"C1","ts":"9.9"}`)),
			},
			wantChannel: "C1",
			wantTS:      "9.9",
		},
		{
			name: "plaintext error",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("no_channel")),
			},
			wantErr:    true,
			wantErrSub: "no_channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, retry, err := readAndParseSlackResponse(tt.resp, tt.opts)
			require.Equal(t, tt.wantRetry, retry)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrSub != "" {
					require.Contains(t, err.Error(), tt.wantErrSub)
				}
				return
			}
			require.NoError(t, err)
			if tt.wantChannel != "" {
				require.True(t, data.OK)
				require.Equal(t, tt.wantChannel, data.Channel)
				require.Equal(t, tt.wantTS, data.Timestamp)
			}
			if tt.wantIgnoredSlackErr != "" {
				require.False(t, data.OK)
				require.Equal(t, tt.wantIgnoredSlackErr, data.Error)
			}
		})
	}
}

func TestApplyThreadOrUpdateStore_ThreadParentOnce(t *testing.T) {
	store := nflog.NewStore(nil)
	notifier, err := New(
		&config.SlackConfig{
			MessageStrategy: config.SlackMessageStrategyThread,
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.persistResponseState(store, slackResponse{OK: true, Channel: "C1", Timestamp: "1.0"})
	ts, _ := store.GetStr(storeKeyThreadTs)
	require.Equal(t, "1.0", ts)

	notifier.persistResponseState(store, slackResponse{OK: true, Channel: "C1", Timestamp: "2.0"})
	ts2, _ := store.GetStr(storeKeyThreadTs)
	require.Equal(t, "1.0", ts2, "thread strategy must not overwrite parent ts on reply")
}

func TestPostJSONAndHandle_RetrierOnReaction(t *testing.T) {
	apiurl, err := url.Parse("https://slack.com/api/chat.postMessage")
	require.NoError(t, err)
	notifier, err := New(
		&config.SlackConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIURL:     &amcommoncfg.SecretURL{URL: apiurl},
			Channel:    "#c",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	notifier.postJSONFunc = func(ctx context.Context, client *http.Client, u string, body io.Reader) (*http.Response, error) {
		resp := httptest.NewRecorder()
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp.WriteHeader(http.StatusInternalServerError)
		resp.WriteString(`{"ok":false}`)
		return resp.Result(), nil
	}

	retry, err := notifier.postAndHandle(
		context.Background(),
		"https://slack.com/api/reactions.add",
		"#c",
		reactionRequest{Channel: "C", Name: "x", Timestamp: "1"},
		nil,
		slackResponseOpts{IgnoreAPIErrors: []string{"already_reacted"}},
	)
	require.True(t, retry)
	require.Error(t, err)
	var reasonErr *notify.ErrorWithReason
	require.ErrorAs(t, err, &reasonErr)
	require.Equal(t, notify.ServerErrorReason, reasonErr.Reason)
}

// TestSlackThreadLifecycle exercises the full thread lifecycle through a real
// HTTP mock server: fire → fire → resolve, with parent as always-updated summary.
func TestSlackThreadLifecycle(t *testing.T) {
	type capturedCall struct {
		Path string
		Body json.RawMessage
	}
	var (
		mu    sync.Mutex
		calls []capturedCall
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, capturedCall{Path: r.URL.Path, Body: body})
		callNum := len(calls)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		switch r.URL.Path {
		case "/api/chat.postMessage":
			ts := fmt.Sprintf("1700000%03d.000%03d", callNum, callNum*100)
			fmt.Fprintf(w, `{"ok":true,"channel":"C_MOCK_123","ts":"%s"}`, ts)
			t.Logf("mock: chat.postMessage #%d → ts=%s", callNum, ts)
		case "/api/chat.update":
			fmt.Fprint(w, `{"ok":true,"channel":"C_MOCK_123","ts":"1700000001.000100"}`)
			t.Logf("mock: chat.update #%d", callNum)
		case "/api/reactions.add":
			fmt.Fprint(w, `{"ok":true}`)
			t.Logf("mock: reactions.add #%d", callNum)
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	apiurl, err := url.Parse(server.URL + "/api/chat.postMessage")
	require.NoError(t, err)

	notifier, err := New(
		&config.SlackConfig{
			NotifierConfig:  amcommoncfg.NotifierConfig{VSendResolved: true},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			APIURL:          &amcommoncfg.SecretURL{URL: apiurl},
			Channel:         "#alerts",
			MessageStrategy: config.SlackMessageStrategyThread,
			ThreadedOptions: &config.SlackThreadedOptions{
				ResolveEmoji: "white_check_mark",
				SummaryHeader: &config.SlackThreadSummaryHeaderOptions{
					ResolveColor: "#AAAAAA",
				},
			},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	firingAlert := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"alertname": "TestAlert", "severity": "critical"},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	store := nflog.NewStore(nil)

	// --- Step 1: First firing alert → summary parent + first reply ---
	ctx1 := context.Background()
	ctx1 = notify.WithGroupKey(ctx1, "test-group|alerts|{alertname=TestAlert}")
	ctx1 = notify.WithNflogStore(ctx1, store)
	ctx1 = notify.WithNotificationReason(ctx1, notify.ReasonFirstNotification)

	_, err = notifier.Notify(ctx1, firingAlert)
	require.NoError(t, err)

	var parentTs, transitions string
	func() {
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, calls, 2, "step 1: expect 2 calls (summary parent + reply)")
		var parentReq request
		require.NoError(t, json.Unmarshal(calls[0].Body, &parentReq))
		require.Equal(t, "/api/chat.postMessage", calls[0].Path)
		require.Empty(t, parentReq.Text, "parent summary should have no text, only attachment")
		require.Empty(t, parentReq.ThreadTimestamp, "parent should NOT have thread_ts")
		var firstReplyReq request
		require.NoError(t, json.Unmarshal(calls[1].Body, &firstReplyReq))
		require.Equal(t, "/api/chat.postMessage", calls[1].Path)
		require.Equal(t, "1700000001.000100", firstReplyReq.ThreadTimestamp, "first reply should thread into parent")
		parentTs, _ = store.GetStr("threadTs")
		require.Equal(t, "1700000001.000100", parentTs)
		transitions, _ = store.GetStr("transitions")
		require.Equal(t, "FIRING", transitions)
		t.Logf("step 1: parent summary + reply posted, parent ts=%s", parentTs)
	}()

	// --- Step 2: Second firing alert → threaded reply + parent update ---
	ctx2 := context.Background()
	ctx2 = notify.WithGroupKey(ctx2, "test-group|alerts|{alertname=TestAlert}")
	ctx2 = notify.WithNflogStore(ctx2, store)
	ctx2 = notify.WithNotificationReason(ctx2, notify.ReasonNewAlertsInGroup)

	_, err = notifier.Notify(ctx2, firingAlert)
	require.NoError(t, err)

	func() {
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, calls, 4, "step 2: expect 4 calls total (2 parent+reply + 1 reply + 1 update)")
		require.Equal(t, "/api/chat.postMessage", calls[2].Path)
		require.Equal(t, "/api/chat.update", calls[3].Path)
		tr, _ := store.GetStr("transitions")
		require.Equal(t, "FIRING|FIRING", tr)
		var updateReq2 request
		require.NoError(t, json.Unmarshal(calls[3].Body, &updateReq2))
		require.Equal(t, "FIRING (2): TestAlert", updateReq2.Attachments[0].Title, "parent update should reflect current transitions")
		t.Logf("step 2: reply + parent update, transitions=%s", tr)
	}()

	// --- Step 3: All alerts resolved → reply + parent update + emoji ---
	resolvedAlert := &types.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"alertname": "TestAlert", "severity": "critical"},
			StartsAt: time.Now().Add(-time.Hour),
			EndsAt:   time.Now().Add(-time.Minute),
		},
	}

	ctx3 := context.Background()
	ctx3 = notify.WithGroupKey(ctx3, "test-group|alerts|{alertname=TestAlert}")
	ctx3 = notify.WithNflogStore(ctx3, store)
	ctx3 = notify.WithNotificationReason(ctx3, notify.ReasonAllAlertsResolved)

	_, err = notifier.Notify(ctx3, resolvedAlert)
	require.NoError(t, err)

	func() {
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, calls, 7, "step 3: expect 7 calls total (4 prev + reply + update + emoji)")

		tr, _ := store.GetStr("transitions")
		require.Equal(t, "FIRING|FIRING|RESOLVED", tr)

		require.Equal(t, "/api/chat.postMessage", calls[4].Path)
		require.Equal(t, "/api/chat.update", calls[5].Path)
		require.Equal(t, "/api/reactions.add", calls[6].Path)

		var updateReq request
		require.NoError(t, json.Unmarshal(calls[5].Body, &updateReq))
		require.Equal(t, "1700000001.000100", updateReq.Timestamp)
		require.Equal(t, "C_MOCK_123", updateReq.Channel)
		require.Empty(t, updateReq.ThreadTimestamp)
		require.Len(t, updateReq.Attachments, 1)
		require.Equal(t, "#AAAAAA", updateReq.Attachments[0].Color, "parent update should use resolve_color")
		require.Equal(t, "FIRING (2) → RESOLVED: TestAlert", updateReq.Attachments[0].Title, "parent update should have auto-generated transition title")
		t.Logf("step 3: resolved (color=%s, title=%q)", updateReq.Attachments[0].Color, updateReq.Attachments[0].Title)
	}()

	t.Log("--- Thread Lifecycle Summary ---")
	t.Logf("  1. FIRE    → parent summary + reply  (ts=%s)", parentTs)
	t.Logf("  2. FIRE    → reply + parent update    (transitions=FIRING|FIRING)")
	t.Logf("  3. RESOLVE → reply + update + emoji   (title='FIRING (2) → RESOLVED: TestAlert')")
}
