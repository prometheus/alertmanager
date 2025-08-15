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

package zeustelegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"

	commoncfg "github.com/prometheus/common/config"

	"io"
	"log/slog"
	"net/http"
	"strings"
)


// Notifier implements a Notifier for telegram notifications.
type Notifier struct {
	conf    *config.ZeusTelegramConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// New returns a new ZeusTelegram notification handler.
func New(conf *config.ZeusTelegramConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "zeustelegram", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier {
		conf:    conf,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{},
		postJSONFunc: notify.PostJSON,
	}, nil
}

type zeusTelegramMessage struct {
	SensitiveData             []string `json:"sensitive_data"`
	SensitiveDataRegexPattern string   `json:"sensitive_data_regex_pattern"`
	EventId					  string   `json:"event_id"`
	EventStatus				  string   `json:"event_status"`
	Severity			      string   `json:"severity"`
	Sender			          string   `json:"sender"`
	BotToken                  string   `json:"bot_token"`
	ChatID                    int64    `json:"chat_id"`
	Subject                   string   `json:"subject"`
	Text                      string   `json:"text"`
	ParseMode                 string   `json:"parse_mode"`
}

func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}
	n.logger.Debug("Extracted group key", "key", key)
	data := notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if n.conf.ParseMode == "HTML" {
		tmpl = notify.TmplHTML(n.tmpl, data, &err)
	}
	var (
		apiUrl = strings.TrimSpace(tmpl(n.conf.APIUrl.String()))
		sensitiveData = n.conf.SensitiveData
		sensitiveDataRegexPattern = tmpl(n.conf.SensitiveDataRegexPattern)
		eventId	 = tmpl(n.conf.EventId)
		eventStatus	 = tmpl(n.conf.EventStatus)
		severity = tmpl(n.conf.Severity)
		sender = tmpl(n.conf.Sender)
		botToken = tmpl(n.conf.BotToken)
		chatId = n.conf.ChatID
		subject = tmpl(n.conf.Subject)
		text = tmpl(n.conf.Text)
		parseMode = tmpl(n.conf.ParseMode)
	)
	zeusTelegramMessageBody := zeusTelegramMessage {
		SensitiveData:             sensitiveData,
		SensitiveDataRegexPattern: sensitiveDataRegexPattern,
		EventId:   				   eventId,
		EventStatus:			   eventStatus,
		Severity:			       severity,
		Sender:  		           sender,
		BotToken:                  botToken,
		ChatID:                    chatId,
		Subject:                   subject,
		Text:                      text,
		ParseMode:                 parseMode,
	}
	var bodyAsBuffers bytes.Buffer
	if err = json.NewEncoder(&bodyAsBuffers).Encode(zeusTelegramMessageBody); err != nil {
		return false, err
	}
	response, err := n.postJSONFunc(ctx, n.client, apiUrl, &bodyAsBuffers)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	responseBody, err := getResponseBodyAsString(response)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	if response.StatusCode/100 != 2 {
		shouldRetry, err := n.retrier.Check(response.StatusCode, nil)
		if err != nil {
			return true, err
		}
		return shouldRetry, fmt.Errorf("unexpected status code %d: %s", response.StatusCode, responseBody)
	}
	n.logger.Debug("ZeusTelegram response: " + responseBody)
	defer notify.Drain(response)
	return false, err
}

func getResponseBodyAsString(resp *http.Response) (string, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, resp.Body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return buf.String(), nil
}
