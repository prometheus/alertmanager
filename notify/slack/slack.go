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

package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

// https://api.slack.com/reference/messaging/attachments#legacy_fields - 1024, no units given, assuming runes or characters.
const maxTitleLenRunes = 1024

// New returns a new Slack notification handler.
func New(c *SlackConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "slack", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:         c,
		tmpl:         t,
		logger:       l,
		client:       client,
		retrier:      &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
		postJSONFunc: notify.PostJSON,
	}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*alert.Alert) notify.NotifyVerdict {
	var err error
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)
	var markdownIn []string

	if len(n.conf.MrkdwnIn) == 0 {
		markdownIn = []string{"fallback", "pretext", "text"}
	} else {
		markdownIn = n.conf.MrkdwnIn
	}

	title, truncated := notify.TruncateInRunes(tmplText(n.conf.Title), maxTitleLenRunes)
	if truncated {
		logger.Warn("Truncated title", "max_runes", maxTitleLenRunes)
	}
	att := &attachment{
		Title:      title,
		TitleLink:  tmplText(n.conf.TitleLink),
		Pretext:    tmplText(n.conf.Pretext),
		Text:       tmplText(n.conf.Text),
		Fallback:   tmplText(n.conf.Fallback),
		CallbackID: tmplText(n.conf.CallbackID),
		ImageURL:   tmplText(n.conf.ImageURL),
		ThumbURL:   tmplText(n.conf.ThumbURL),
		Footer:     tmplText(n.conf.Footer),
		Color:      tmplText(n.conf.Color),
		MrkdwnIn:   markdownIn,
	}

	numFields := len(n.conf.Fields)
	if numFields > 0 {
		fields := make([]SlackField, numFields)
		for index, field := range n.conf.Fields {
			// Check if short was defined for the field otherwise fallback to the global setting
			var short bool
			if field.Short != nil {
				short = *field.Short
			} else {
				short = n.conf.ShortFields
			}

			// Rebuild the field by executing any templates and setting the new value for short
			fields[index] = SlackField{
				Title: tmplText(field.Title),
				Value: tmplText(field.Value),
				Short: &short,
			}
		}
		att.Fields = fields
	}

	numActions := len(n.conf.Actions)
	if numActions > 0 {
		actions := make([]SlackAction, numActions)
		for index, action := range n.conf.Actions {
			slackAction := SlackAction{
				Type:  tmplText(action.Type),
				Text:  tmplText(action.Text),
				URL:   tmplText(action.URL),
				Style: tmplText(action.Style),
				Name:  tmplText(action.Name),
				Value: tmplText(action.Value),
			}

			if action.ConfirmField != nil {
				slackAction.ConfirmField = &SlackConfirmationField{
					Title:       tmplText(action.ConfirmField.Title),
					Text:        tmplText(action.ConfirmField.Text),
					OkText:      tmplText(action.ConfirmField.OkText),
					DismissText: tmplText(action.ConfirmField.DismissText),
				}
			}

			actions[index] = slackAction
		}
		att.Actions = actions
	}

	var u string
	if n.conf.APIURL != nil {
		u = n.conf.APIURL.String()
	} else {
		content, err := os.ReadFile(n.conf.APIURLFile)
		if err != nil {
			return notify.Unrecoverable(err, notify.DefaultReason)
		}
		u = strings.TrimSpace(string(content))
	}

	if n.conf.Timeout > 0 {
		postCtx, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured slack timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = postCtx
	}

	msg := message{
		Username:    tmplText(n.conf.Username),
		IconEmoji:   tmplText(n.conf.IconEmoji),
		IconURL:     tmplText(n.conf.IconURL),
		LinkNames:   n.conf.LinkNames,
		Text:        tmplText(n.conf.MessageText),
		Attachments: []attachment{*att},
	}

	// If a notification for this alert group has already been sent, `update_message`
	// edits the initial message instead of sending a new one and `post_updates_to_thread`
	// posts the notification as a reply in the initial message's thread. With both set,
	// the initial message is also posted as the first reply in its own thread.
	var store *nflog.Store
	var threadTs, channelId string

	if n.conf.UpdateMessage || n.conf.PostUpdatesToThread {
		var ok bool
		store, ok = notify.NflogStore(ctx)
		if !ok {
			logger.Warn("cannot create NflogStore, updatable and threaded messages will be disabled.")
		} else {
			threadTs, _ = store.GetStr("threadTs")
			channelId, _ = store.GetStr("channelId")
			logger.Debug("attempt recovering threadTs and channelId of the initial message", "threadTs", threadTs, "channelId", channelId)
		}
	}

	if threadTs == "" || channelId == "" {
		if verdict := n.postRequest(ctx, u, &request{message: msg, Channel: tmplText(n.conf.Channel)}, store); verdict.Err() != nil {
			return verdict
		}
		// With both options set, later updates overwrite the initial message in place and
		// its original content would be lost. Keep it by posting a copy as the first reply.
		if !n.conf.UpdateMessage || !n.conf.PostUpdatesToThread || store == nil {
			return notify.Success()
		}
		threadTs, _ = store.GetStr("threadTs")
		channelId, _ = store.GetStr("channelId")
		if threadTs == "" || channelId == "" {
			logger.Warn("threadTs or channelId missing after posting initial message, cannot copy it to its thread")
			return notify.Success()
		}
		logger.Debug("copying initial message to its thread", "threadTs", threadTs, "channelId", channelId)
		copyReq := &request{message: msg, Channel: channelId, ThreadTimestamp: threadTs}
		return n.postRequest(ctx, u, copyReq, nil)
	}

	// Requests targeting the initial message get no store, so its identifiers are never overwritten.
	if n.conf.UpdateMessage {
		logger.Debug("updating previously sent message", "threadTs", threadTs, "channelId", channelId)
		updateReq := &request{message: msg, Channel: channelId, Timestamp: threadTs}
		if verdict := n.postRequest(ctx, "https://slack.com/api/chat.update", updateReq, nil); verdict.Err() != nil {
			return verdict
		}
	}
	if n.conf.PostUpdatesToThread {
		logger.Debug("posting to thread of previously sent message", "threadTs", threadTs, "channelId", channelId)
		threadReq := &request{message: msg, Channel: channelId, ThreadTimestamp: threadTs}
		return n.postRequest(ctx, u, threadReq, nil)
	}

	return notify.Success()
}

// postRequest encodes and sends a single request to the Slack API, classifies
// errors as retriable or not, and hands the response to slackResponseHandler.
func (n *Notifier) postRequest(ctx context.Context, u string, req *request, store *nflog.Store) notify.NotifyVerdict {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}

	resp, err := n.postJSONFunc(ctx, n.client, u, &buf)
	received := time.Now()
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", err, context.Cause(ctx))
		}
		return notify.Retry(0, notify.RedactURL(err), notify.DefaultReason)
	}
	defer notify.Drain(resp)

	// Use a retrier to generate an error message for non-200 responses and
	// classify them as retriable or not.
	retry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		reason := notify.GetFailureReasonFromStatusCode(resp.StatusCode)
		if retry {
			retryAfter := notify.ParseRetryAfter(resp.Header, received)
			if retryAfter > 0 {
				n.logger.Warn("Rate limited by Slack, delaying retry", "retry_after_secs", retryAfter.Seconds())
			}
			return notify.Retry(retryAfter, err, reason)
		}
		return notify.Unrecoverable(err, reason)
	}

	retry, err = n.slackResponseHandler(resp, store)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		if retry {
			return notify.Retry(0, err, notify.ClientErrorReason)
		}
		return notify.Unrecoverable(err, notify.ClientErrorReason)
	}
	return notify.Success()
}

// slackResponseHandler parses the response body of the request, handles retryable errors
// and saves the response timestamp and channelId to nflog.
func (n *Notifier) slackResponseHandler(resp *http.Response, store *nflog.Store) (bool, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, fmt.Errorf("could not read response body: %w", err)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return checkTextResponseError(body)
	}
	var data slackResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return true, fmt.Errorf("could not unmarshal JSON response %q: %w", string(body), err)
	}
	if !data.OK {
		return false, fmt.Errorf("error response from Slack: %s", data.Error)
	}
	// If store, TS and Channel are set, store the threadTS and channelId
	if store != nil && data.Timestamp != "" && data.Channel != "" {
		store.SetStr("threadTs", data.Timestamp)
		store.SetStr("channelId", data.Channel)
		n.logger.Debug("stored threadTs and channelId", "threadTs", data.Timestamp, "channelId", data.Channel)
	}
	return false, nil
}

// checkTextResponseError classifies plaintext responses from Slack.
// A plaintext (non-JSON) response is successful if it's a string "ok".
// This is typically a response for an Incoming Webhook
// (https://api.slack.com/messaging/webhooks#handling_errors)
func checkTextResponseError(body []byte) (bool, error) {
	if !bytes.Equal(body, []byte("ok")) {
		return false, fmt.Errorf("received an error response from Slack: %s", string(body))
	}
	return false, nil
}
