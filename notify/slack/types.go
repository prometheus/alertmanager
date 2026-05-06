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
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/slack/internal/apiurl"
	"github.com/prometheus/alertmanager/template"
)

// Notifier sends alerts to Slack using SlackConfig (API URL, message_strategy, etc.),
// Templates, and an HTTP client. postJSONFunc is swappable for tests.
type Notifier struct {
	conf        *config.SlackConfig
	tmpl        *template.Template
	logger      *slog.Logger
	client      *http.Client
	retrier     *notify.Retrier
	urlResolver *apiurl.Resolver

	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// request is the JSON body for chat.postMessage and chat.update (and threaded variants).
type request struct {
	Channel         string       `json:"channel,omitempty"`
	Timestamp       string       `json:"ts,omitempty"`
	ThreadTimestamp string       `json:"thread_ts,omitempty"`
	Username        string       `json:"username,omitempty"`
	IconEmoji       string       `json:"icon_emoji,omitempty"`
	IconURL         string       `json:"icon_url,omitempty"`
	LinkNames       bool         `json:"link_names,omitempty"`
	Text            string       `json:"text,omitempty"`
	Attachments     []attachment `json:"attachments"`
}

// attachment is used to display a richly formatted message block.
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

// reactionRequest is the request payload for Slack's reactions.add API.
type reactionRequest struct {
	Channel   string `json:"channel"`
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
}

// slackResponse represents the response from Slack API.
type slackResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Timestamp string `json:"ts,omitempty"`
}

// slackResponseOpts configures interpretation of Slack JSON bodies after a 2xx HTTP status.
type slackResponseOpts struct {
	// IgnoreAPIErrors lists Slack JSON "error" codes treated as success when ok is false
	// (e.g. already_reacted from reactions.add).
	IgnoreAPIErrors []string
}

// treatsSlackErrorAsSuccess reports whether code is a non-fatal API outcome.
func (o slackResponseOpts) treatsSlackErrorAsSuccess(code string) bool {
	return slices.Contains(o.IgnoreAPIErrors, code)
}

// threadSummaryHeaderContent holds the computed parent summary line for thread mode
// when use_summary_header is true (transition text, title, color, notify reason).
type threadSummaryHeaderContent struct {
	transitions string
	title       string
	color       string
	reason      notify.NotifyReason
	reasonOk    bool
}
