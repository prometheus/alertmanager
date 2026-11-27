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
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

const (
	// Keys under which the initial post is stored in the notification log.
	storeKeyPostID    = "postId"
	storeKeyChannelID = "channelId"

	postsPath = "/api/v4/posts"
)

// apiProps are the props of a Mattermost post.
// https://developers.mattermost.com/integrate/reference/message-attachments/
type apiProps struct {
	Attachments []attachment `json:"attachments,omitempty"`
	Card        string       `json:"card,omitempty"`
}

// createPostRequest is the body of POST /api/v4/posts.
type createPostRequest struct {
	ChannelID string   `json:"channel_id"`
	Message   string   `json:"message,omitempty"`
	Props     apiProps `json:"props"`
}

// patchPostRequest is the body of PUT /api/v4/posts/{id}/patch.
type patchPostRequest struct {
	Message string   `json:"message"`
	Props   apiProps `json:"props"`
}

// postResponse is the subset of the Mattermost post object that is used.
type postResponse struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// notifyAPI sends the notification through the Mattermost REST API.
func (n *Notifier) notifyAPI(ctx context.Context, data *template.Data) notify.NotifyVerdict {
	var err error
	req := n.createRequest(notify.TmplText(n.tmpl, data, &err))
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	if err := n.sanitizeRequest(ctx, req); err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}

	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	logger := n.logger.With("group_key", key)

	props := apiProps{Attachments: req.Attachments}
	if req.Props != nil {
		props.Card = req.Props.Card
	}
	base := strings.TrimRight(n.conf.APIURL.String(), "/")

	// If a post for this alert group has already been created, update_message
	// edits it instead of creating a new one.
	var (
		store             *nflog.Store
		postID, channelID string
	)
	if n.conf.UpdateMessage {
		var ok bool
		store, ok = notify.NflogStore(ctx)
		if !ok {
			logger.Warn("cannot create NflogStore, updatable messages will be disabled.")
		} else {
			postID, _ = store.GetStr(storeKeyPostID)
			channelID, _ = store.GetStr(storeKeyChannelID)
			logger.Debug("attempt recovering the initial post", "post_id", postID, "channel_id", channelID)
		}
	}

	if postID != "" && channelID == n.conf.ChannelID {
		logger.Debug("updating previously sent post", "post_id", postID)
		patchURL := base + postsPath + "/" + url.PathEscape(postID) + "/patch"
		_, verdict := n.doAPIRequest(ctx, http.MethodPut, patchURL, patchPostRequest{Message: req.Text, Props: props})
		return verdict
	}

	res, verdict := n.doAPIRequest(ctx, http.MethodPost, base+postsPath, createPostRequest{
		ChannelID: n.conf.ChannelID,
		Message:   req.Text,
		Props:     props,
	})
	if verdict.Err() != nil {
		return verdict
	}
	if store != nil {
		if res.ID == "" {
			logger.Warn("Mattermost response has no post id, the post cannot be updated later")
		} else {
			store.SetStr(storeKeyPostID, res.ID)
			store.SetStr(storeKeyChannelID, cmp.Or(res.ChannelID, n.conf.ChannelID))
			logger.Debug("stored post_id and channel_id", "post_id", res.ID)
		}
	}
	return verdict
}

// doAPIRequest sends a JSON request to the Mattermost API and classifies
// failures as retriable or not. The returned verdict has no error on success.
func (n *Notifier) doAPIRequest(ctx context.Context, method, u string, payload any) (*postResponse, notify.NotifyVerdict) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return nil, notify.Unrecoverable(err, notify.DefaultReason)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, u, &buf)
	if err != nil {
		return nil, notify.Unrecoverable(notify.RedactURL(err), notify.DefaultReason)
	}
	httpReq.Header.Set("User-Agent", notify.UserAgentHeader)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(httpReq)
	if err != nil {
		return nil, notify.Retry(0, notify.RedactURL(err), notify.DefaultReason)
	}
	defer notify.Drain(resp)

	retry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", n.conf.ChannelID, err)
		reason := notify.GetFailureReasonFromStatusCode(resp.StatusCode)
		if retry {
			return nil, notify.Retry(0, err, reason)
		}
		return nil, notify.Unrecoverable(err, reason)
	}

	// The post has been sent at this point, so an unreadable body is not an error.
	var out postResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		n.logger.Warn("Could not decode Mattermost response", "err", err)
	}
	n.logger.Debug("Message sent to Mattermost API successfully", "method", method, "status", resp.StatusCode)
	return &out, notify.Success()
}
