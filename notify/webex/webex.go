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

package webex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for Webex notifications.
type Notifier struct {
	conf    *config.WebexConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new Webex notifier.
func New(c *config.WebexConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "webex", httpOpts...)
	if err != nil {
		return nil, err
	}

	notifier := &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}, CustomDetailsFunc: errDetails}}

	return notifier, nil
}

type WebexMessage struct {
	RoomID        string            `json:"roomId,omitempty"`        // Room ID.
	ToPersonID    string            `json:"toPersonId,omitempty"`    // Person ID (for type=direct).
	ToPersonEmail string            `json:"toPersonEmail,omitempty"` // Person email (for type=direct).
	Text          string            `json:"text,omitempty"`          // Message in plain text format.
	Markdown      string            `json:"markdown,omitempty"`      // Message in markdown format.
	Files         []string          `json:"files,omitempty"`         // File URL array.
	Attachments   []webexAttachment `json:"attachments,omitempty"`   //Attachment Array
}

// Local definition of webexAttachment differs from the config version, as the content output to Webex
// is a JSON object rather than a string representing that JSON object as it is in config (for ease of templating).
type webexAttachment struct {
	ContentType string                 `json:"contentType"`
	Content     map[string]interface{} `json:"content"`
}

// maxMessageSize represents the maximum message body size in bytes.
const maxMessageSize = 7439

// Notify implements the Webex Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	req, retry, err := n.createRequest(ctx, alerts...)
	if err != nil {
		return retry, err
	}
	resp, err := n.client.Do(req.WithContext(ctx))
	if err != nil {
		return true, err
	}
	defer notify.Drain(resp)

	retry, err = n.retrier.Check(resp.StatusCode, resp.Request.Body)
	return retry, err
}

func (n *Notifier) createRequest(ctx context.Context, alerts ...*types.Alert) (*http.Request, bool, error) {
	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return nil, false, err
	}

	data := notify.GetTemplateData(ctx, n.tmpl, alerts, n.logger)
	var tmplErr error
	tmpl := notify.TmplText(n.tmpl, data, &tmplErr)

	// Template and truncate message body
	markdown, truncated := notify.Truncate(tmpl(n.conf.Markdown), maxMessageSize)
	if truncated {
		level.Debug(n.logger).Log("msg", "truncated Markdown message", "truncated_message", markdown, "alert", groupKey)
	}
	text, truncated := notify.Truncate(tmpl(n.conf.Text), maxMessageSize)
	if truncated {
		level.Debug(n.logger).Log("msg", "truncated Text message", "truncated_message", text, "alert", groupKey)
	}

	// Templating for attached file URLs
	var files []string
	for _, file := range n.conf.Files {
		files = append(files, tmpl(file))
	}

	// For card attachments, card definitions are expected to be JSON strings in the config
	// to make templating straightforward.  In the outgoing message, the fields are sent as
	// members of the JSON object rather than as a string value.
	var attachments []webexAttachment
	for _, attachment := range n.conf.Attachments {
		cardJSON := tmpl(attachment.Content)
		card := make(map[string]interface{})
		jsonErr := json.Unmarshal([]byte(cardJSON), &card)
		if jsonErr != nil {
			return nil, false, errors.Wrap(jsonErr, "failed to parse Webex attachment content JSON")
		}

		newAttachment := webexAttachment{
			ContentType: tmpl(attachment.ContentType),
			Content:     card,
		}

		attachments = append(attachments, newAttachment)
	}

	msg := &WebexMessage{
		RoomID:        tmpl(n.conf.RoomID),
		ToPersonID:    tmpl(n.conf.ToPersonID),
		ToPersonEmail: tmpl(n.conf.ToPersonEmail),
		Markdown:      markdown,
		Text:          text,
		Files:         files,
		Attachments:   attachments,
	}

	if tmplErr != nil {
		return nil, false, errors.Wrap(tmplErr, "failed to template")
	}

	postMessageURL := n.conf.APIURL.Copy()
	postMessageURL.Path += "v1/messages"
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return nil, false, err
	}
	req, err := http.NewRequest("POST", postMessageURL.String(), &buf)
	if err != nil {
		return nil, true, err
	}
	req.Header.Set("Authorization", "Bearer "+string(n.conf.APIToken))
	req.Header.Set("User-Agent", notify.UserAgentHeader)
	req.Header.Set("Content-Type", "application/json")

	return req, true, nil

}

func errDetails(status int, body io.Reader) string {
	if status != http.StatusBadRequest || body == nil {
		return ""
	}
	var pgr struct {
		Status  string   `json:"status"`
		Message string   `json:"message"`
		Errors  []string `json:"errors"`
	}
	if err := json.NewDecoder(body).Decode(&pgr); err != nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", pgr.Message, strings.Join(pgr.Errors, ","))
}
