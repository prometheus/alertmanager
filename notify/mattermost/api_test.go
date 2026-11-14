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
	"sync"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

type capturedRequest struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

// newAPITestServer returns a fake Mattermost API that records requests and
// answers with the given status and body.
func newAPITestServer(t *testing.T, status int, respBody string) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var (
		mu       sync.Mutex
		captured []capturedRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		mu.Lock()
		captured = append(captured, capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   body,
		})
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), captured...)
	}
}

func newAPITestNotifier(t *testing.T, srvURL string, updateMessage bool) *Notifier {
	t.Helper()
	u, err := url.Parse(srvURL)
	require.NoError(t, err)

	cfg := DefaultMattermostConfig
	cfg.APIURL = &amcommoncfg.URL{URL: u}
	cfg.ChannelID = "chan123"
	cfg.UpdateMessage = updateMessage
	cfg.Text = "Test Text"
	cfg.HTTPConfig = &commoncfg.HTTPClientConfig{
		Authorization: &commoncfg.Authorization{Type: "Bearer", Credentials: "bot-token"},
	}

	notifier, err := New(&cfg, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)
	return notifier
}

func newAPITestCtx(store *nflog.Store) context.Context {
	ctx := notify.WithGroupKey(context.Background(), "test-group-key")
	if store != nil {
		ctx = notify.WithNflogStore(ctx, store)
	}
	return ctx
}

func testAlert() *alert.Alert {
	return alert.New(model.Alert{
		Labels:   model.LabelSet{"lbl1": "val1"},
		StartsAt: time.Now(),
		EndsAt:   time.Now().Add(time.Hour),
	}, time.Time{}, false)
}

func TestMattermostAPI(t *testing.T) {
	t.Parallel()

	t.Run("first notification creates a post and stores its ids", func(t *testing.T) {
		t.Parallel()
		srv, requests := newAPITestServer(t, http.StatusCreated, `{"id":"post1","channel_id":"chan123"}`)
		notifier := newAPITestNotifier(t, srv.URL, true)
		store := nflog.NewStore(nil)

		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())

		reqs := requests()
		require.Len(t, reqs, 1)
		require.Equal(t, http.MethodPost, reqs[0].method)
		require.Equal(t, "/api/v4/posts", reqs[0].path)
		require.Equal(t, "Bearer bot-token", reqs[0].auth)
		require.Equal(t, "chan123", reqs[0].body["channel_id"])
		props, ok := reqs[0].body["props"].(map[string]any)
		require.True(t, ok)
		require.Contains(t, props, "attachments")

		postID, _ := store.GetStr("postId")
		channelID, _ := store.GetStr("channelId")
		require.Equal(t, "post1", postID)
		require.Equal(t, "chan123", channelID)
	})

	t.Run("subsequent notification patches the stored post", func(t *testing.T) {
		t.Parallel()
		srv, requests := newAPITestServer(t, http.StatusOK, `{"id":"post1","channel_id":"chan123"}`)
		notifier := newAPITestNotifier(t, srv.URL, true)
		store := nflog.NewStore(nil)
		store.SetStr("postId", "post1")
		store.SetStr("channelId", "chan123")

		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())

		reqs := requests()
		require.Len(t, reqs, 1)
		require.Equal(t, http.MethodPut, reqs[0].method)
		require.Equal(t, "/api/v4/posts/post1/patch", reqs[0].path)
		require.NotContains(t, reqs[0].body, "channel_id")
		props, ok := reqs[0].body["props"].(map[string]any)
		require.True(t, ok)
		require.Contains(t, props, "attachments")

		postID, _ := store.GetStr("postId")
		require.Equal(t, "post1", postID)
	})

	t.Run("stored post of another channel is not updated", func(t *testing.T) {
		t.Parallel()
		srv, requests := newAPITestServer(t, http.StatusCreated, `{"id":"post2","channel_id":"chan123"}`)
		notifier := newAPITestNotifier(t, srv.URL, true)
		store := nflog.NewStore(nil)
		store.SetStr("postId", "post1")
		store.SetStr("channelId", "otherchan")

		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())

		reqs := requests()
		require.Len(t, reqs, 1)
		require.Equal(t, http.MethodPost, reqs[0].method)
	})

	t.Run("without update_message every notification creates a post", func(t *testing.T) {
		t.Parallel()
		srv, requests := newAPITestServer(t, http.StatusCreated, `{"id":"post1","channel_id":"chan123"}`)
		notifier := newAPITestNotifier(t, srv.URL, false)
		store := nflog.NewStore(nil)
		store.SetStr("postId", "post1")
		store.SetStr("channelId", "chan123")

		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())
		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())

		reqs := requests()
		require.Len(t, reqs, 2)
		for _, r := range reqs {
			require.Equal(t, http.MethodPost, r.method)
			require.Equal(t, "/api/v4/posts", r.path)
		}
		postID, _ := store.GetStr("postId")
		require.Equal(t, "post1", postID, "store must not be touched without update_message")
	})

	t.Run("update_message without a store still creates a post", func(t *testing.T) {
		t.Parallel()
		srv, requests := newAPITestServer(t, http.StatusCreated, `{"id":"post1","channel_id":"chan123"}`)
		notifier := newAPITestNotifier(t, srv.URL, true)

		require.NoError(t, notifier.Notify(newAPITestCtx(nil), testAlert()).Err())
		require.Len(t, requests(), 1)
	})

	t.Run("response without id stores nothing", func(t *testing.T) {
		t.Parallel()
		srv, _ := newAPITestServer(t, http.StatusCreated, `{}`)
		notifier := newAPITestNotifier(t, srv.URL, true)
		store := nflog.NewStore(nil)

		require.NoError(t, notifier.Notify(newAPITestCtx(store), testAlert()).Err())

		postID, ok := store.GetStr("postId")
		require.False(t, ok)
		require.Empty(t, postID)
	})

	t.Run("patch error is returned and not retried on 404", func(t *testing.T) {
		t.Parallel()
		srv, _ := newAPITestServer(t, http.StatusNotFound, `{"message":"not found"}`)
		notifier := newAPITestNotifier(t, srv.URL, true)
		store := nflog.NewStore(nil)
		store.SetStr("postId", "post1")
		store.SetStr("channelId", "chan123")

		verdict := notifier.Notify(newAPITestCtx(store), testAlert())
		require.Error(t, verdict.Err())
		require.False(t, verdict.ShouldRetry())
	})

	t.Run("server error is retried", func(t *testing.T) {
		t.Parallel()
		srv, _ := newAPITestServer(t, http.StatusInternalServerError, `{}`)
		notifier := newAPITestNotifier(t, srv.URL, true)

		verdict := notifier.Notify(newAPITestCtx(nflog.NewStore(nil)), testAlert())
		require.Error(t, verdict.Err())
		require.True(t, verdict.ShouldRetry())
	})
}
