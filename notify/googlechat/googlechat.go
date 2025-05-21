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

package googlechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

type Notifier struct {
	conf    *config.GoogleChatConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

func New(conf *config.GoogleChatConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "googlechat", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:    conf,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{},
	}, nil
}

// Message represents the structure for sending a
// Text message in Google Chat Webhook endpoint.
// https://developers.google.com/chat/api/guides/message-formats/basic
type Message struct {
	Text string `json:"text"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	var (
		data = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl = notify.TmplText(n.tmpl, data, &err)
	)

	message := tmpl(n.conf.Message)
	if err != nil {
		return false, err
	}

	msg := &Message{
		Text: message,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	var webhookURL string
	if n.conf.URL != nil {
		webhookURL = n.conf.URL.String()
	} else {
		content, err := os.ReadFile(n.conf.URLFile)
		if err != nil {
			return false, fmt.Errorf("read url_file: %w", err)
		}
		webhookURL = strings.TrimSpace(string(content))
	}

	// https://developers.google.com/chat/how-tos/webhooks#start_a_message_thread
	// To post the first message of a thread with a webhook,
	// append the threadKey and messageReplyOption parameters to the webhook URL.
	// Set the threadKey to an arbitrary string, but remember what it is;
	// you'll need to specify it again to post a reply to the thread.
	// https://chat.googleapis.com/v1/spaces/SPACE_ID/messages?threadKey=ARBITRARY_STRING&messageReplyOption=REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD

	u, err := url.Parse(webhookURL)
	if err != nil {
		return false, fmt.Errorf("unable to parse googlechat url: %w", err)
	}
	q := u.Query()
	if n.conf.Threading {
		q.Set("threadKey", key.Hash())
		q.Set("messageReplyOption", "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
	}
	u.RawQuery = q.Encode()
	webhookURL = u.String()

	resp, err := notify.PostJSON(ctx, n.client, webhookURL, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	return shouldRetry, err
}
