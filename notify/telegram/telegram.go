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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	commoncfg "github.com/prometheus/common/config"
	"golang.org/x/net/html"
	"gopkg.in/telebot.v3"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

const (
	// Telegram rejects formatted input over 1 << 15 bytes before entity parsing.
	maxMessageLenBytes = 1 << 15
	// Telegram supports up to 4096 characters after entity parsing.
	// See https://core.telegram.org/bots/api#sendmessage.
	maxMessageLenRunes = 4096
	// Rich messages support up to 32768 characters of text and have no
	// separate limit on the formatted input.
	// See https://core.telegram.org/bots/api#rich-message-limits.
	maxRichMessageLenRunes = 32768
)

// htmlWhitespaceRun matches a run of HTML whitespace, which rich messages
// collapse to one space outside <pre>, as a browser does.
var htmlWhitespaceRun = regexp.MustCompile(`[ \t\n\f\r]+`)

// richHTMLEntities decodes the named entities that rich messages support
// beyond the four telegramHTMLEntityLen knows, so each counts as one rune.
// See https://core.telegram.org/bots/api#rich-html-style.
var richHTMLEntities = strings.NewReplacer(
	"&apos;", "'", "&nbsp;", "\u00a0", "&hellip;", "\u2026", "&mdash;", "\u2014", "&ndash;", "\u2013",
	"&lsquo;", "\u2018", "&rsquo;", "\u2019", "&ldquo;", "\u201c", "&rdquo;", "\u201d",
)

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

func (n *Notifier) Notify(ctx context.Context, alrt ...*alert.Alert) notify.NotifyVerdict {
	key, ok := notify.GroupKey(ctx)
	if !ok {
		return notify.Unrecoverable(fmt.Errorf("group key missing"), notify.DefaultReason)
	}

	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	var (
		err         error
		data        = notify.GetTemplateData(ctx, n.tmpl, alrt, logger)
		tmpl        = notify.TmplText(n.tmpl, data, &err)
		messageText string
		truncated   bool
	)

	switch n.conf.ParseMode {
	case "HTML":
		tmpl = notify.TmplHTML(n.tmpl, data, &err)
		messageText = tmpl(n.conf.Message)
		if err != nil {
			return notify.Unrecoverable(err, notify.DefaultReason)
		}
		var tooLong bool
		if n.conf.RichMessage {
			tooLong = richHTMLTextRuneCount(messageText) > maxRichMessageLenRunes
		} else {
			tooLong = len(messageText) > maxMessageLenBytes || htmlTextRuneCount(messageText) > maxMessageLenRunes
		}
		if tooLong {
			messageText = `Alertmanager notification could not be sent: message length exceeds Telegram limits.
			Please check the template used for producing the message content.`
		}
	default:
		messageText, truncated = notify.TruncateInRunes(tmpl(n.conf.Message), maxMessageLenRunes)
		if err != nil {
			return notify.Unrecoverable(err, notify.DefaultReason)
		}
		if truncated {
			logger.Warn("Truncated message", "max_runes", maxMessageLenRunes)
		}
	}

	n.client.Token, err = n.getBotToken()
	if err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}

	chatID, err := n.getChatID()
	if err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}

	var message *telebot.Message
	if n.conf.RichMessage {
		message, err = n.sendRichMessage(chatID, messageText)
	} else {
		message, err = n.client.Send(telebot.ChatID(chatID), messageText, &telebot.SendOptions{
			DisableNotification:   n.conf.DisableNotifications,
			DisableWebPagePreview: true,
			ThreadID:              n.conf.MessageThreadID,
			ParseMode:             n.conf.ParseMode,
		})
	}
	if err != nil {
		if n.conf.Timeout > 0 && errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("configured telegram timeout reached (%s)", n.conf.Timeout)
		}
		err = notify.RedactURL(err)
		return notify.Retry(0, err, failureReason(err))
	}
	logger.Debug("Telegram message successfully published", "message_id", message.ID, "chat_id", message.Chat.ID)

	return notify.Success()
}

// sendRichMessage sends an HTML message through sendRichMessage, which telebot
// does not implement. Errors come from telebot's response parsing, so they are
// classified the same way as errors from Send.
func (n *Notifier) sendRichMessage(chatID int64, html string) (*telebot.Message, error) {
	params := map[string]any{
		"chat_id":      chatID,
		"rich_message": map[string]string{"html": html},
	}
	if n.conf.DisableNotifications {
		params["disable_notification"] = true
	}
	if n.conf.MessageThreadID != 0 {
		params["message_thread_id"] = n.conf.MessageThreadID
	}

	data, err := n.client.Raw("sendRichMessage", params)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result *telebot.Message `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("could not decode sendRichMessage response: %w", err)
	}
	if resp.Result == nil || resp.Result.Chat == nil {
		return nil, errors.New("sendRichMessage response has no message")
	}
	return resp.Result, nil
}

// failureReason classifies errors returned by the Telegram Bot API so that the
// failed notifications metric is labeled with the failure reason. Errors that
// telebot does not surface in a structured form fall back to the default reason.
func failureReason(err error) notify.Reason {
	if _, ok := errors.AsType[telebot.FloodError](err); ok {
		return notify.RateLimitedReason
	}
	if apiErr, ok := errors.AsType[*telebot.Error](err); ok {
		return notify.GetFailureReasonFromStatusCode(apiErr.Code)
	}
	return notify.DefaultReason
}

// htmlTextRuneCount excludes HTML markup and attribute values such as link targets.
func htmlTextRuneCount(message string) int {
	return countHTMLTextRunes(message, false)
}

// richHTMLTextRuneCount counts as Telegram counts a rich message: like
// htmlTextRuneCount, with each run of whitespace outside <pre> as one rune,
// the rich named entities as one rune, and custom emoji alternative text.
func richHTMLTextRuneCount(message string) int {
	return countHTMLTextRunes(message, true)
}

func countHTMLTextRunes(message string, rich bool) int {
	tokenizer := html.NewTokenizer(strings.NewReader(message))
	count := 0
	preDepth := 0

	for {
		switch tt := tokenizer.Next(); tt {
		case html.ErrorToken:
			if len(tokenizer.Raw()) > 0 {
				return utf8.RuneCountInString(message)
			}
			return count
		case html.CommentToken, html.DoctypeToken:
			return utf8.RuneCountInString(message)
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			switch {
			case string(name) == "pre" && tt == html.StartTagToken:
				preDepth++
			case rich && string(name) == "img" && hasAttr:
				count += customEmojiAltRuneCount(tokenizer)
			}
		case html.EndTagToken:
			if name, _ := tokenizer.TagName(); string(name) == "pre" && preDepth > 0 {
				preDepth--
			}
		case html.TextToken:
			// tokenizer.Text applies HTML5 entity decoding, while Telegram
			// supports a smaller set. Use raw text so unsupported entities
			// remain literal.
			text := tokenizer.Raw()
			if rich {
				if preDepth == 0 {
					text = htmlWhitespaceRun.ReplaceAll(text, []byte(" "))
				}
				text = []byte(richHTMLEntities.Replace(string(text)))
			}
			count += telegramHTMLTextRuneCount(text)
		}
	}
}

// customEmojiAltRuneCount returns the length of a custom emoji's alternative
// text, which counts toward the rich message limit; other images count nothing.
// See https://core.telegram.org/bots/api#rich-message-limits.
func customEmojiAltRuneCount(tokenizer *html.Tokenizer) int {
	var src, alt string
	for more := true; more; {
		var key, val []byte
		key, val, more = tokenizer.TagAttr()
		switch string(key) {
		case "src":
			src = string(val)
		case "alt":
			alt = string(val)
		}
	}
	if !strings.HasPrefix(src, "tg://emoji") {
		return 0
	}
	return utf8.RuneCountInString(alt)
}

// telegramHTMLTextRuneCount counts visible runes in a raw HTML text token.
// Telegram-supported entities count as one rune; other input remains literal.
func telegramHTMLTextRuneCount(text []byte) int {
	count := 0
	for len(text) > 0 {
		if text[0] == '&' {
			if entityLen := telegramHTMLEntityLen(text); entityLen > 0 {
				count++
				text = text[entityLen:]
				continue
			}
		}

		_, size := utf8.DecodeRune(text)
		count++
		text = text[size:]
	}
	return count
}

// telegramHTMLEntityLen returns the byte length of an entity Telegram would
// decode at the start of text, or zero if Telegram would render it literally.
// Telegram accepts four named entities and valid decimal or hexadecimal numeric
// entities, with or without a trailing semicolon.
// This mirrors TDLib's decode_html_entity implementation:
// https://github.com/tdlib/td/blob/d1085f9cebc5a62379991ae1652673954f229c1f/td/telegram/MessageEntity.cpp#L3240-L3286
func telegramHTMLEntityLen(text []byte) int {
	if len(text) < 2 || text[0] != '&' {
		return 0
	}

	end := 1
	if text[end] != '#' {
		for end < len(text) && (text[end] >= 'a' && text[end] <= 'z' || text[end] >= 'A' && text[end] <= 'Z') {
			end++
		}
		switch string(text[1:end]) {
		case "lt", "gt", "amp", "quot":
			if end < len(text) && text[end] == ';' {
				end++
			}
			return end
		default:
			return 0
		}
	}

	end++
	base := 10
	if end < len(text) && text[end] == 'x' {
		base = 16
		end++
	}
	digitsStart := end
	for end < len(text) {
		c := text[end]
		if c >= '0' && c <= '9' || base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			end++
			continue
		}
		break
	}
	code, err := strconv.ParseUint(string(text[digitsStart:end]), base, 32)
	if err != nil || code == 0 || code >= utf8.MaxRune || end >= 10 {
		return 0
	}
	if end < len(text) && text[end] == ';' {
		end++
	}
	return end
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
