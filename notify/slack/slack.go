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

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// https://api.slack.com/reference/messaging/attachments#legacy_fields - 1024, no units given, assuming runes or characters.
const maxTitleLenRunes = 1024

// New returns a new Slack notification handler.
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
		retrier:      &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
		postJSONFunc: notify.PostJSON,
	}, nil
}

// Notify implements the Notifier interface.
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

	var u string
	if n.conf.APIURL != nil {
		u = n.conf.APIURL.String()
	} else {
		content, err := os.ReadFile(n.conf.APIURLFile)
		if err != nil {
			return false, err
		}
		u = strings.TrimSpace(string(content))
	}

	if n.conf.Timeout > 0 {
		postCtx, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured slack timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = postCtx
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

	store := n.nflogStore(ctx, logger)
	parentTS, parentChannel := storedParent(store)
	haveParent := parentTS != "" && parentChannel != ""

	edit := n.conf.UpdateMessage && haveParent
	reply := n.conf.ThreadReplies && haveParent && !skipThreadReply(ctx, edit)

	// Follow-ups must not replace the parent timestamp with a reply's ts.
	var record *nflog.Store
	if !edit && !reply {
		record = store
	}

	firstURL := u
	if edit {
		u = "https://slack.com/api/chat.update"
		req.Timestamp = parentTS
		req.Channel = parentChannel
		logger.Debug("editing existing Slack message", "ts", parentTS, "channel", parentChannel)
	} else if reply {
		req.ThreadTimestamp = parentTS
		req.Channel = parentChannel
		logger.Debug("replying in existing Slack thread", "ts", parentTS, "channel", parentChannel)
	}

	retry, err := n.post(ctx, u, req, record)
	if err != nil {
		return retry, err
	}
	if !edit || !reply {
		return retry, nil
	}

	followUp := *req
	followUp.Timestamp = ""
	followUp.ThreadTimestamp = parentTS
	logger.Debug("adding thread reply after editing parent", "ts", parentTS, "channel", parentChannel)
	return n.post(ctx, firstURL, &followUp, nil)
}

func (n *Notifier) nflogStore(ctx context.Context, logger *slog.Logger) *nflog.Store {
	if !n.conf.UpdateMessage && !n.conf.ThreadReplies {
		return nil
	}
	store, ok := notify.NflogStore(ctx)
	if !ok {
		logger.Warn("nflog store missing; Slack message editing and thread replies disabled")
		return nil
	}
	return store
}

func storedParent(store *nflog.Store) (ts, channel string) {
	if store == nil {
		return "", ""
	}
	ts, _ = store.GetStr("threadTs")
	channel, _ = store.GetStr("channelId")
	if ts == "" || channel == "" {
		return "", ""
	}
	return ts, channel
}

// skipThreadReply reports whether a thread reply would only duplicate a parent
// that update_message is already rewriting. Repeats with thread_replies alone
// still post a reply, otherwise Slack would receive nothing.
func skipThreadReply(ctx context.Context, editingParent bool) bool {
	if !editingParent {
		return false
	}
	reason, ok := notify.NotificationReason(ctx)
	return ok && reason == notify.ReasonRepeatIntervalElapsed
}

func (n *Notifier) post(ctx context.Context, u string, req *request, store *nflog.Store) (bool, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	resp, err := n.postJSONFunc(ctx, n.client, u, &buf)
	received := time.Now()
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
		if resp.StatusCode == http.StatusTooManyRequests {
			if d := notify.ParseRetryAfter(resp.Header, received); d > 0 {
				n.logger.Warn("Rate limited by Slack, waiting before retry", "retry_after_secs", d.Seconds())
				select {
				case <-time.After(d):
				case <-ctx.Done():
				}
			}
		}
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	retry, err = n.slackResponseHandler(resp, store)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		return retry, notify.NewErrorWithReason(notify.ClientErrorReason, err)
	}
	return retry, nil
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
	if store != nil && data.Timestamp != "" && data.Channel != "" {
		store.SetStr("threadTs", data.Timestamp)
		store.SetStr("channelId", data.Channel)
		n.logger.Debug("stored Slack message identity", "ts", data.Timestamp, "channel", data.Channel)
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
