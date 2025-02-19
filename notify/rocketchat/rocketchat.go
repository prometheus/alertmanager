// Copyright 2022 Prometheus Team
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

package rocketchat

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
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const maxTitleLenRunes = 1024

type Notifier struct {
	conf    *config.RocketchatConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
	token   string
	tokenID string

	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// PostMessage Payload for postmessage rest API
//
// https://rocket.chat/docs/developer-guides/rest-api/chat/postmessage/
type Attachment struct {
	Title     string                              `json:"title,omitempty"`
	TitleLink string                              `json:"title_link,omitempty"`
	Text      string                              `json:"text,omitempty"`
	ImageURL  string                              `json:"image_url,omitempty"`
	ThumbURL  string                              `json:"thumb_url,omitempty"`
	Color     string                              `json:"color,omitempty"`
	Fields    []config.RocketchatAttachmentField  `json:"fields,omitempty"`
	Actions   []config.RocketchatAttachmentAction `json:"actions,omitempty"`
}

// PostMessage Payload for postmessage rest API
//
// https://rocket.chat/docs/developer-guides/rest-api/chat/postmessage/
type PostMessage struct {
	Channel     string                              `json:"channel,omitempty"`
	Text        string                              `json:"text,omitempty"`
	ParseUrls   bool                                `json:"parseUrls,omitempty"`
	Alias       string                              `json:"alias,omitempty"`
	Emoji       string                              `json:"emoji,omitempty"`
	Avatar      string                              `json:"avatar,omitempty"`
	Attachments []Attachment                        `json:"attachments,omitempty"`
	Actions     []config.RocketchatAttachmentAction `json:"actions,omitempty"`
}

type rocketchatRoundTripper struct {
	wrapped http.RoundTripper
	token   string
	tokenID string
}

func (t *rocketchatRoundTripper) RoundTrip(req *http.Request) (res *http.Response, e error) {
	req.Header.Set("X-Auth-Token", t.token)
	req.Header.Set("X-User-Id", t.tokenID)
	return t.wrapped.RoundTrip(req)
}

// New returns a new Rocketchat notification handler.
func New(c *config.RocketchatConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "rocketchat", httpOpts...)
	if err != nil {
		return nil, err
	}
	token, err := getToken(c)
	if err != nil {
		return nil, err
	}
	tokenID, err := getTokenID(c)
	if err != nil {
		return nil, err
	}

	client.Transport = &rocketchatRoundTripper{wrapped: client.Transport, token: token, tokenID: tokenID}
	return &Notifier{
		conf:         c,
		tmpl:         t,
		logger:       l,
		client:       client,
		retrier:      &notify.Retrier{},
		postJSONFunc: notify.PostJSON,
		token:        token,
		tokenID:      tokenID,
	}, nil
}

func getTokenID(c *config.RocketchatConfig) (string, error) {
	if len(c.TokenIDFile) > 0 {
		content, err := os.ReadFile(c.TokenIDFile)
		if err != nil {
			return "", fmt.Errorf("could not read %s: %w", c.TokenIDFile, err)
		}
		return strings.TrimSpace(string(content)), nil
	}
	return string(*c.TokenID), nil
}

func getToken(c *config.RocketchatConfig) (string, error) {
	if len(c.TokenFile) > 0 {
		content, err := os.ReadFile(c.TokenFile)
		if err != nil {
			return "", fmt.Errorf("could not read %s: %w", c.TokenFile, err)
		}
		return strings.TrimSpace(string(content)), nil
	}
	return string(*c.Token), nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	tmplText := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}
	title := tmplText(n.conf.Title)
	if err != nil {
		return false, err
	}

	title, truncated := notify.TruncateInRunes(title, maxTitleLenRunes)
	if truncated {
		key, err := notify.ExtractGroupKey(ctx)
		if err != nil {
			return false, err
		}
		n.logger.Warn("Truncated title", "key", key, "max_runes", maxTitleLenRunes)
	}
	att := &Attachment{
		Title:     title,
		TitleLink: tmplText(n.conf.TitleLink),
		Text:      tmplText(n.conf.Text),
		ImageURL:  tmplText(n.conf.ImageURL),
		ThumbURL:  tmplText(n.conf.ThumbURL),
		Color:     tmplText(n.conf.Color),
	}
	numFields := len(n.conf.Fields)
	if numFields > 0 {
		fields := make([]config.RocketchatAttachmentField, numFields)
		for index, field := range n.conf.Fields {
			// Check if short was defined for the field otherwise fallback to the global setting
			var short bool
			if field.Short != nil {
				short = *field.Short
			} else {
				short = n.conf.ShortFields
			}

			// Rebuild the field by executing any templates and setting the new value for short
			fields[index] = config.RocketchatAttachmentField{
				Title: tmplText(field.Title),
				Value: tmplText(field.Value),
				Short: &short,
			}
		}
		att.Fields = fields
	}
	numActions := len(n.conf.Actions)
	if numActions > 0 {
		actions := make([]config.RocketchatAttachmentAction, numActions)
		for index, action := range n.conf.Actions {
			actions[index] = config.RocketchatAttachmentAction{
				Type: "button", // Only button type is supported
				Text: tmplText(action.Text),
				URL:  tmplText(action.URL),
				Msg:  tmplText(action.Msg),
			}
		}
		att.Actions = actions
	}

	body := &PostMessage{
		Channel:     tmplText(n.conf.Channel),
		Emoji:       tmplText(n.conf.Emoji),
		Avatar:      tmplText(n.conf.IconURL),
		Attachments: []Attachment{*att},
	}
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return false, err
	}
	url := n.conf.APIURL.JoinPath("api/v1/chat.postMessage").String()
	resp, err := n.postJSONFunc(ctx, n.client, url, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// Use a retrier to generate an error message for non-200 responses and
	// classify them as retriable or not.
	retry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", body.Channel, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}

	// Rocketchat web API might return errors with a 200 response code.
	retry, err = checkResponseError(resp)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", body.Channel, err)
		return retry, notify.NewErrorWithReason(notify.ClientErrorReason, err)
	}

	return retry, nil
}

// checkResponseError parses out the error message from Rocketchat API response.
func checkResponseError(resp *http.Response) (bool, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, fmt.Errorf("could not read response body: %w", err)
	}

	return checkJSONResponseError(body)
}

// checkJSONResponseError classifies JSON responses from Rocketchat.
func checkJSONResponseError(body []byte) (bool, error) {
	// response is for parsing out errors from the JSON response.
	type response struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}

	var data response
	if err := json.Unmarshal(body, &data); err != nil {
		return true, fmt.Errorf("could not unmarshal JSON response %q: %w", string(body), err)
	}
	if !data.Success {
		return false, fmt.Errorf("error response from Rocketchat: %s", data.Error)
	}
	return false, nil
}
