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

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// https://api.slack.com/reference/messaging/attachments#legacy_fields - 1024, no units given, assuming runes or characters.
const maxTitleLenRunes = 1024

// Notifier implements a Notifier for Slack notifications.
type Notifier struct {
	conf          *config.SlackConfig
	tmpl          *template.Template
	logger        *slog.Logger
	client        *http.Client
	retrier       *notify.Retrier
	metadataStore *notify.MetadataStore

	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// New returns a new Slack notification handler.
func New(c *config.SlackConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "slack", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:          c,
		tmpl:          t,
		logger:        l,
		client:        client,
		retrier:       &notify.Retrier{},
		metadataStore: notify.NewMetadataStore(),
		postJSONFunc:  notify.PostJSON,
	}, nil
}

// request is the request for sending a slack notification.
type request struct {
	Channel     string       `json:"channel,omitempty"`
	Username    string       `json:"username,omitempty"`
	IconEmoji   string       `json:"icon_emoji,omitempty"`
	IconURL     string       `json:"icon_url,omitempty"`
	LinkNames   bool         `json:"link_names,omitempty"`
	Attachments []attachment `json:"attachments"`
	// Timestamp is used for updating existing messages (chat.update API)
	Timestamp string `json:"ts,omitempty"`
}

// slackResponse represents the response from Slack API.
type slackResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Channel string `json:"channel,omitempty"`
	TS      string `json:"ts,omitempty"` // Message timestamp, used for updates
}

// attachment is used to display a richly-formatted message block.
type attachment struct {
	Title      string               `json:"title,omitempty"`
	TitleLink  string               `json:"title_link,omitempty"`
	Pretext    string               `json:"pretext,omitempty"`
	Text       string               `json:"text"`
	Fallback   string               `json:"fallback"`
	CallbackID string               `json:"callback_id"`
	Fields     []config.SlackField  `json:"fields,omitempty"`
	Actions    []config.SlackAction `json:"actions,omitempty"`
	ImageURL   string               `json:"image_url,omitempty"`
	ThumbURL   string               `json:"thumb_url,omitempty"`
	Footer     string               `json:"footer"`
	Color      string               `json:"color,omitempty"`
	MrkdwnIn   []string             `json:"mrkdwn_in,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	// Extract group key and receiver name for message tracking
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	receiverName, ok := notify.ReceiverName(ctx)
	if !ok {
		n.logger.Warn("receiver name missing from context")
	}

	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
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
		n.logger.Warn("Truncated title", "key", key, "max_runes", maxTitleLenRunes)
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
		Attachments: []attachment{*att},
	}
	if err != nil {
		return false, err
	}

	// Check for existing message if update_message is enabled
	var existingTS string
	if n.conf.UpdateMessage && receiverName != "" {
		// Create a simple receiver representation for metadata lookup
		simpleReceiver := &nflogpb.Receiver{
			GroupName:   receiverName,
			Integration: "slack",
			Idx:         0,
		}

		n.logger.Debug("checking for existing message",
			"key", key,
			"receiver", receiverName,
			"update_message", n.conf.UpdateMessage)

		if metadata, ok := n.metadataStore.Get(simpleReceiver, key.String()); ok {
			if ts, exists := metadata["message_ts"]; exists && ts != "" {
				existingTS = ts
				req.Timestamp = ts

				// For chat.update, we need to use channel ID, not channel name
				if channelID, ok := metadata["channel_id"]; ok && channelID != "" {
					req.Channel = channelID
					n.logger.Debug("FOUND existing Slack message - will UPDATE",
						"key", key,
						"receiver", receiverName,
						"message_ts", ts,
						"channel_id", channelID)
				} else {
					n.logger.Debug("FOUND message but no channel_id, using channel name",
						"key", key,
						"message_ts", ts)
				}
			} else {
				n.logger.Debug("metadata exists but no message_ts", "metadata", metadata)
			}
		} else {
			n.logger.Debug("no existing message found - will create NEW",
				"key", key,
				"receiver", receiverName)
		}
	} else {
		n.logger.Debug("update disabled or no receiver",
			"update_message", n.conf.UpdateMessage,
			"receiver", receiverName)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
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

	// Use chat.update endpoint if we're updating an existing message
	if existingTS != "" && n.conf.UpdateMessage {
		// Replace chat.postMessage with chat.update for updates
		u = strings.Replace(u, "chat.postMessage", "chat.update", 1)
		n.logger.Debug("using chat.update endpoint for message update", "url", u)
	}

	if n.conf.Timeout > 0 {
		postCtx, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured slack timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = postCtx
	}

	resp, err := n.postJSONFunc(ctx, n.client, u, &buf)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", err, context.Cause(ctx))
		}
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// Read response body once for all checks
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, fmt.Errorf("channel %q: failed to read response body: %w", req.Channel, err)
	}

	// Use a retrier to generate an error message for non-200 responses and
	// classify them as retriable or not.
	retry, err := n.retrier.Check(resp.StatusCode, bytes.NewReader(body))
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	// Slack web API might return errors with a 200 response code.
	// https://slack.dev/node-slack-sdk/web-api#handle-errors
	retry, err = checkJSONResponseErrorFromBody(body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		return retry, notify.NewErrorWithReason(notify.ClientErrorReason, err)
	}

	// Save message timestamp and channel ID for future updates if update_message is enabled
	if n.conf.UpdateMessage && receiverName != "" {
		if slackResp, err := extractSlackResponseFromBody(body); err == nil && slackResp.TS != "" {
			simpleReceiver := &nflogpb.Receiver{
				GroupName:   receiverName,
				Integration: "slack",
				Idx:         0,
			}

			metadata := map[string]string{
				"message_ts": slackResp.TS,
				"channel":    req.Channel,
			}

			// Save channel ID for future updates (required by chat.update)
			if slackResp.Channel != "" {
				metadata["channel_id"] = slackResp.Channel
			}

			n.metadataStore.Set(simpleReceiver, key.String(), metadata)

			n.logger.Debug("saved Slack message_ts for future updates",
				"key", key,
				"receiver", receiverName,
				"message_ts", slackResp.TS,
				"channel_id", slackResp.Channel,
				"is_update", existingTS != "")
		}
	}

	return retry, nil
}

// extractSlackResponseFromBody extracts the full Slack response from body.
func extractSlackResponseFromBody(body []byte) (*slackResponse, error) {
	var slackResp slackResponse
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Slack response: %w", err)
	}

	if !slackResp.OK {
		return nil, fmt.Errorf("slack API returned error: %s", slackResp.Error)
	}

	return &slackResp, nil
}

// checkJSONResponseErrorFromBody classifies JSON responses from body bytes.
func checkJSONResponseErrorFromBody(body []byte) (bool, error) {
	var data slackResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return true, fmt.Errorf("could not unmarshal JSON response %q: %w", string(body), err)
	}
	if !data.OK {
		return false, fmt.Errorf("error response from Slack: %s", data.Error)
	}
	return false, nil
}
