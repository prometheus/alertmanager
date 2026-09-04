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

package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	commoncfg "github.com/prometheus/common/config"
	"golang.org/x/net/html"
	"gopkg.in/telebot.v3"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Telegram supports up to 4096 characters after entity parsing.
// See https://core.telegram.org/bots/api#sendmessage.
const maxMessageLenRunes = 4096

// Notifier implements a Notifier for telegram notifications.
type Notifier struct {
	conf    *TelegramConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *telebot.Bot
	retrier *notify.Retrier
}

// New returns a new Telegram notification handler.
func New(conf *TelegramConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	httpclient, err := notify.NewClientWithTracing(*conf.HTTPConfig, "telegram", httpOpts...)
	if err != nil {
		return nil, err
	}

	if conf.Timeout > 0 {
		httpclient.Timeout = conf.Timeout
	}

	client, err := createTelegramClient(conf.APIUrl.String(), conf.ParseMode, httpclient)
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

func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	key, ok := notify.GroupKey(ctx)
	if !ok {
		return false, fmt.Errorf("group key missing")
	}

	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	var (
		err         error
		data        = notify.GetTemplateData(ctx, n.tmpl, alert, logger)
		tmpl        = notify.TmplText(n.tmpl, data, &err)
		messageText string
		truncated   bool
	)

	switch n.conf.ParseMode {
	case "HTML":
		tmpl = notify.TmplHTML(n.tmpl, data, &err)
		messageText = tmpl(n.conf.Message)
		if err != nil {
			return false, err
		}
		if htmlTextRuneCount(messageText) > maxMessageLenRunes {
			messageText = `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`
		}
	default:
		messageText, truncated = notify.TruncateInRunes(tmpl(n.conf.Message), maxMessageLenRunes)
		if err != nil {
			return false, err
		}
		if truncated {
			logger.Warn("Truncated message", "max_runes", maxMessageLenRunes)
		}
	}

	n.client.Token, err = n.getBotToken()
	if err != nil {
		return true, err
	}

	chatID, err := n.getChatID()
	if err != nil {
		return true, err
	}

	message, err := n.client.Send(telebot.ChatID(chatID), messageText, &telebot.SendOptions{
		DisableNotification:   n.conf.DisableNotifications,
		DisableWebPagePreview: true,
		ThreadID:              n.conf.MessageThreadID,
		ParseMode:             n.conf.ParseMode,
	})
	if err != nil {
		if n.conf.Timeout > 0 && errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("configured telegram timeout reached (%s)", n.conf.Timeout)
		}
		return true, wrapWithFailureReason(notify.RedactURL(err))
	}
	logger.Debug("Telegram message successfully published", "message_id", message.ID, "chat_id", message.Chat.ID)

	return false, nil
}

// wrapWithFailureReason classifies errors returned by the Telegram Bot API so
// that the failed notifications metric is labeled with the failure reason.
// Errors that telebot does not surface in a structured form are returned as-is
// and fall back to the default reason.
func wrapWithFailureReason(err error) error {
	if _, ok := errors.AsType[telebot.FloodError](err); ok {
		return notify.NewErrorWithReason(notify.RateLimitedReason, err)
	}
	if apiErr, ok := errors.AsType[*telebot.Error](err); ok {
		return notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(apiErr.Code), err)
	}
	return err
}

// htmlTextRuneCount excludes HTML markup and attribute values such as link targets.
func htmlTextRuneCount(message string) int {
	tokenizer := html.NewTokenizer(strings.NewReader(message))
	count := 0

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if len(tokenizer.Raw()) > 0 {
				return utf8.RuneCountInString(message)
			}
			return count
		case html.CommentToken, html.DoctypeToken:
			return utf8.RuneCountInString(message)
		case html.TextToken:
			count += utf8.RuneCount(tokenizer.Text())
		}
	}
}

func createTelegramClient(apiURL, parseMode string, httpClient *http.Client) (*telebot.Bot, error) {
	bot, err := telebot.NewBot(telebot.Settings{
		URL:       apiURL,
		ParseMode: parseMode,
		Client:    httpClient,
		Offline:   true,
	})
	if err != nil {
		return nil, err
	}

	return bot, nil
}

func (n *Notifier) getBotToken() (string, error) {
	if len(n.conf.BotTokenFile) > 0 {
		content, err := os.ReadFile(n.conf.BotTokenFile)
		if err != nil {
			return "", fmt.Errorf("could not read %s: %w", n.conf.BotTokenFile, err)
		}
		return strings.TrimSpace(string(content)), nil
	}
	return string(n.conf.BotToken), nil
}

func (n *Notifier) getChatID() (int64, error) {
	if len(n.conf.ChatIDFile) > 0 {
		content, err := os.ReadFile(n.conf.ChatIDFile)
		if err != nil {
			return 0, fmt.Errorf("could not read %s: %w", n.conf.ChatIDFile, err)
		}
		chatID, err := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("could not parse chat_id from %s: %w", n.conf.ChatIDFile, err)
		}
		return chatID, nil
	}
	return n.conf.ChatID, nil
}
