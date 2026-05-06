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
	"strings"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/slack/internal/apiurl"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// https://api.slack.com/reference/messaging/attachments#legacy_fields - 1024, no units given, assuming runes or characters.
const maxTitleLenRunes = 1024

// nflog store keys for persisting Slack-specific state across notifications.
const (
	storeKeyThreadTs    = "threadTs"
	storeKeyChannelId   = "channelId"
	storeKeyTransitions = "transitions"
)

// New builds a Slack Notifier with tracing enabled on the configured HTTP client.
func New(c *config.SlackConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "slack", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:         c,
		tmpl:         t,
		logger:       l,
		client:       client,
		retrier:      &notify.Retrier{},
		urlResolver:  apiurl.NewResolver(c.APIURL, c.APIURLFile),
		postJSONFunc: notify.PostJSON,
	}, nil
}

// Notify implements the Notifier interface. It expands templates, builds the Slack
// payload, and sends it (or updates an existing message / thread) based on
// message_strategy and nflog state. The returned bool is true when the delivery
// should be retried (e.g. transport or retryable HTTP errors).
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
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
		fields := make([]config.SlackField, numFields)
		for index, field := range n.conf.Fields {
			// Check if short was defined for the field otherwise fallback to the global setting
			var short bool
			if field.Short != nil {
				short = *field.Short
			} else {
				short = n.conf.ShortFields
			}

			// Rebuild the field by executing any templates and setting the new value for short
			fields[index] = config.SlackField{
				Title: tmplText(field.Title),
				Value: tmplText(field.Value),
				Short: &short,
			}
		}
		att.Fields = fields
	}

	numActions := len(n.conf.Actions)
	if numActions > 0 {
		actions := make([]config.SlackAction, numActions)
		for index, action := range n.conf.Actions {
			slackAction := config.SlackAction{
				Type:  tmplText(action.Type),
				Text:  tmplText(action.Text),
				URL:   tmplText(action.URL),
				Style: tmplText(action.Style),
				Name:  tmplText(action.Name),
				Value: tmplText(action.Value),
			}

			if action.ConfirmField != nil {
				slackAction.ConfirmField = &config.SlackConfirmationField{
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

	req := &request{
		Channel:     tmplText(n.conf.Channel),
		Username:    tmplText(n.conf.Username),
		IconEmoji:   tmplText(n.conf.IconEmoji),
		IconURL:     tmplText(n.conf.IconURL),
		LinkNames:   n.conf.LinkNames,
		Text:        tmplText(n.conf.MessageText),
		Attachments: []attachment{*att},
	}
	// tmplText is notify.TmplText(..., &err): every field execution appends template errors
	// into the same err. Check here so we never call Slack after a failed template render.
	if err != nil {
		return false, err
	}

	u, err := n.urlResolver.URLForMethod("")
	if err != nil {
		return false, err
	}

	if n.conf.Timeout > 0 {
		postCtx, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured slack timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = postCtx
	}

	var store *nflog.Store

	if n.conf.HasStrategyThatUpdatesParent() {
		var ok bool
		store, ok = notify.NflogStore(ctx)
		if !ok {
			logger.Warn("cannot create NflogStore, updatable/threaded messages will be disabled.")
		} else if store == nil {
			logger.Warn("NflogStore is nil, updatable/threaded messages will be disabled.")
		} else {
			// If message_strategy is "update", edit the API endpoint and payload to update
			// the existing notification instead of sending a new one.
			if n.conf.HasUpdateStrategy() {
				threadTs, _ := store.GetStr(storeKeyThreadTs)
				channelId, _ := store.GetStr(storeKeyChannelId)
				logger.Debug("attempt recovering threadTs and channelId to update an existing message", storeKeyThreadTs, threadTs, storeKeyChannelId, channelId)
				if threadTs != "" && channelId != "" {
					updateURL, err := n.urlResolver.URLForMethod("chat.update")
					if err != nil {
						return false, err
					}
					u = updateURL
					req.Timestamp = threadTs
					req.Channel = channelId
					logger.Debug("updating previously sent message", storeKeyThreadTs, threadTs, storeKeyChannelId, channelId)
				}
			} else if n.conf.HasThreadStrategy() {
				// If message_strategy is "thread", there are two modes controlled by the flag use_summary_header.
				if n.conf.UseSummaryHeaderInThread() {
					return n.handleThreadedSummaryHeaderMode(ctx, data, tmplText, &err, store, u, req, logger)
				}
				return n.handleThreadedDirectMode(ctx, store, req, u, logger)
			}
		}
	}

	// Default path: post the message directly (for "new" and "update" strategies, or when thread strategy falls
	// through due to missing nflog store).
	return n.postAndHandle(ctx, u, req.Channel, req, store, slackResponseOpts{})
}

// postAndHandle JSON-encodes payload, POSTs it to u, applies HTTP retry classification,
// Then parses the Slack body. channel is only used in error messages. When store is
// non-nil and the response is successful JSON with ts/channel, persistResponseState may
// Persist nflog keys for update/thread strategies. opts.IgnoreAPIErrors lists Slack
// JSON error codes treated as success (e.g. already_reacted for reactions.add).
func (n *Notifier) postAndHandle(ctx context.Context, u, channel string, payload any, store *nflog.Store, opts slackResponseOpts) (bool, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return false, fmt.Errorf("encode slack request: %w", err)
	}

	resp, err := n.postJSONFunc(ctx, n.client, u, &buf)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", err, context.Cause(ctx))
		}
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// Use a retrier to generate an error message for non-200 responses and
	// classify them as retriable or not.
	retry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", channel, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	data, retry, err := readAndParseSlackResponse(resp, opts)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", channel, err)
		return retry, notify.NewErrorWithReason(notify.ClientErrorReason, err)
	}

	n.persistResponseState(store, data)

	return false, nil
}

// persistResponseState persists the threadTs and channelId of a message in nflog. For message_strategy "thread", only
// the first message is saved, so later replies do not replace the thread root.
func (n *Notifier) persistResponseState(store *nflog.Store, data slackResponse) {
	if store == nil || data.Timestamp == "" || data.Channel == "" {
		return
	}
	if n.conf.HasThreadStrategy() {
		parentThreadTs, parentChannelId, parentFound := getStoredParent(store)
		if !parentFound {
			store.SetStr(storeKeyThreadTs, data.Timestamp)
			store.SetStr(storeKeyChannelId, data.Channel)
			n.logger.Debug("stored threadTs and channelId for thread parent", storeKeyThreadTs, data.Timestamp, storeKeyChannelId, data.Channel)
		} else {
			n.logger.Debug("skipping storing reply as thread parent is already stored", storeKeyThreadTs, parentThreadTs, storeKeyChannelId, parentChannelId)
		}
	} else {
		store.SetStr(storeKeyThreadTs, data.Timestamp)
		store.SetStr(storeKeyChannelId, data.Channel)
		n.logger.Debug("stored threadTs and channelId", storeKeyThreadTs, data.Timestamp, storeKeyChannelId, data.Channel)
	}
}

// checkTextResponseError classifies incoming-webhook plaintext responses.
// Success requires body exactly "ok". The bool is the retry hint (always false here).
// See https://api.slack.com/messaging/webhooks#handling_errors
func checkTextResponseError(body []byte) (bool, error) {
	if !bytes.Equal(bytes.TrimSpace(body), []byte("ok")) {
		return false, fmt.Errorf("received an error response from Slack: %s", string(body))
	}
	return false, nil
}

// readAndParseSlackResponse reads the response body. For Content-Type application/json
// it unmarshals slackResponse; ok=false is an error unless data.Error is listed in
// opts.IgnoreAPIErrors. Non-JSON bodies use incoming-webhook plaintext rules (body "ok").
// Retry is true for read/unmarshal failures that may be transient; false for definitive
// Slack API errors (ok=false without ignore) or successful plaintext.
func readAndParseSlackResponse(resp *http.Response, opts slackResponseOpts) (slackResponse, bool, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return slackResponse{}, true, fmt.Errorf("could not read response body: %w", err)
	}
	contentType := strings.TrimSpace(strings.ToLower(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		retry, err := checkTextResponseError(body)
		return slackResponse{}, retry, err
	}
	var data slackResponse
	if err = json.Unmarshal(body, &data); err != nil {
		return slackResponse{}, true, fmt.Errorf("could not unmarshal JSON response %q: %w", string(body), err)
	}
	if !data.OK {
		if opts.treatsSlackErrorAsSuccess(data.Error) {
			return data, false, nil
		}
		return slackResponse{}, false, fmt.Errorf("error response from Slack: %s", data.Error)
	}
	return data, false, nil
}
