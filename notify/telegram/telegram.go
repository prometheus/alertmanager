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
	"net/http"

	"github.com/go-kit/log"
	"gopkg.in/telebot.v3"

	"github.com/prometheus/alertmanager/template"

	"github.com/go-kit/log/level"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
)

// Notifier implements a Notifier for telegram notifications.
type Notifier struct {
	conf    *config.TelegramConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *telebot.Bot
	retrier *notify.Retrier
}

// New returns a new Telegram notification handler.
func New(conf *config.TelegramConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	httpclient, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "telegram", httpOpts...)
	if err != nil {
		return nil, err
	}

	client, err := createTelegramClient(conf.BotToken, conf.APIUrl.String(), conf.ParseMode, httpclient)
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
	var (
		err  error
		data = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl = notify.TmplText(n.tmpl, data, &err)
	)

	// Telegram supports 4096 chars max
	messageText, truncated := notify.Truncate(tmpl(n.conf.Message), 4096)
	if truncated {
		level.Debug(n.logger).Log("msg", "truncated message", "truncated_message", messageText)
	}

	message, err := n.client.Send(telebot.ChatID(n.conf.ChatID), messageText, &telebot.SendOptions{
		DisableNotification:   n.conf.DisableNotifications,
		DisableWebPagePreview: true,
	})
	if err != nil {
		return true, err
	}
	level.Debug(n.logger).Log("msg", "Telegram message successfully published", "message_id", message.ID, "chat_id", message.Chat.ID)

	return false, nil
}

func createTelegramClient(token config.Secret, apiUrl, parseMode string, httpClient *http.Client) (*telebot.Bot, error) {
	secret := string(token)
	bot, err := telebot.NewBot(telebot.Settings{
		Token:     secret,
		URL:       apiUrl,
		ParseMode: parseMode,
		Client:    httpClient,
		Offline:   true,
	})

	if err != nil {
		return nil, err
	}

	return bot, nil
}
