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
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

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
		retrier:      &notify.Retrier{},
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
		data        = notify.GetTemplateData(ctx, n.tmpl, as, logger)
		tmplTextErr error
		tmplText    = notify.TmplText(n.tmpl, data, &tmplTextErr)
	)

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

	useBlockKit := n.conf.BlocKitEnabeld != nil && *n.conf.BlocKitEnabeld
	req := composePlainRequest(n.conf, tmplText, logger)
	payload := any(req)
	channelForError := req.Channel
	var bkPayload map[string]any

	if useBlockKit {
		bkPayload, err = composeBlockKitPayload(n.conf.BlocKitPayload, tmplText(n.conf.Channel), tmplText(n.conf.MessageText), tmplText, &tmplTextErr)
		if err != nil {
			return false, fmt.Errorf("failed to render block kit payload: %w", err)
		}
		payload = bkPayload
		channelForError = tmplText(n.conf.Channel)
	}
	logger.Debug("payload", "payload", payload)

	// If a notification for this alert group has already been sent and `update_message` config is set
	// edit API endpoint and payload to update notification instead of sending a new one.
	var store *nflog.Store

	if n.conf.UpdateMessage {
		var ok bool
		store, ok = notify.NflogStore(ctx)
		if !ok {
			logger.Warn("cannot create NflogStore, updatable messages will be disabled.")
		} else {
			threadTs, _ := store.GetStr("threadTs")
			channelId, _ := store.GetStr("channelId")
			logger.Debug("attempt recovering threadTs and channelId to update an existing message", "threadTs", threadTs, "channelId", channelId)
			if threadTs != "" && channelId != "" {
				u = "https://slack.com/api/chat.update"
				if useBlockKit {
					if err := setBlockKitPayloadStringValue(bkPayload, "ts", threadTs); err != nil {
						return false, fmt.Errorf("cannot set ts in block kit payload: %w", err)
					}
					if err := setBlockKitPayloadStringValue(bkPayload, "channel", channelId); err != nil {
						return false, fmt.Errorf("cannot set channel in block kit payload: %w", err)
					}
					channelForError = channelId
				} else {
					req.Timestamp = threadTs
					req.Channel = channelId
					channelForError = req.Channel
				}
				logger.Debug("updating previously sent message", "threadTs", threadTs, "channelId", channelId)
			}
		}
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return false, err
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
		err = fmt.Errorf("channel %q: %w", channelForError, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	retry, err = n.slackResponseHandler(resp, store)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", channelForError, err)
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
