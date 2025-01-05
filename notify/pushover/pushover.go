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

package pushover

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

const (
	// https://pushover.net/api#limits - 250 characters or runes.
	maxTitleLenRunes = 250
	// https://pushover.net/api#limits - 1024 characters or runes.
	maxMessageLenRunes = 1024
	// https://pushover.net/api#limits - 512 characters or runes.
	maxURLLenRunes = 512
	// https://pushover.net/api#priority - 2 is emergency priority.
	emergencyPriority = "2"
)

// Notifier implements a Notifier for Pushover notifications.
type Notifier struct {
	conf           *config.PushoverConfig
	tmpl           *template.Template
	logger         *slog.Logger
	client         *http.Client
	retrier        *notify.Retrier
	apiMessagesURL string // for tests.
	apiReceiptsURL string // for tests.
}

// New returns a new Pushover notifier.
func New(c *config.PushoverConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "pushover", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:           c,
		tmpl:           t,
		logger:         l,
		client:         client,
		retrier:        &notify.Retrier{},
		apiMessagesURL: "https://api.pushover.net/1/messages.json",
		apiReceiptsURL: "https://api.pushover.net/1/receipts/cancel_by_tag",
	}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)

	n.logger.Debug("extracted group key", "key", key)

	var message string
	tmpl := notify.TmplText(n.tmpl, data, &err)
	tmplHTML := notify.TmplHTML(n.tmpl, data, &err)

	var (
		token   string
		userKey string
	)
	if n.conf.Token != "" {
		token = string(n.conf.Token)
	} else {
		content, err := os.ReadFile(n.conf.TokenFile)
		if err != nil {
			return false, fmt.Errorf("read token_file: %w", err)
		}
		token = string(content)
	}
	if n.conf.UserKey != "" {
		userKey = string(n.conf.UserKey)
	} else {
		content, err := os.ReadFile(n.conf.UserKeyFile)
		if err != nil {
			return false, fmt.Errorf("read user_key_file: %w", err)
		}
		userKey = string(content)
	}

	parameters := url.Values{}
	parameters.Add("token", tmpl(token))
	parameters.Add("user", tmpl(userKey))

	var (
		priority    = tmpl(n.conf.Priority)
		alerts      = types.Alerts(as...)
		groupKeyTag = fmt.Sprintf("promAlertGroupKey_%s", key.Hash())
		u           *url.URL
	)

	title, truncated := notify.TruncateInRunes(tmpl(n.conf.Title), maxTitleLenRunes)
	if truncated {
		n.logger.Warn("Truncated title", "incident", key, "max_runes", maxTitleLenRunes)
	}
	parameters.Add("title", title)

	if n.conf.HTML {
		parameters.Add("html", "1")
		message = tmplHTML(n.conf.Message)
	} else {
		message = tmpl(n.conf.Message)
	}

	message, truncated = notify.TruncateInRunes(message, maxMessageLenRunes)
	if truncated {
		n.logger.Warn("Truncated message", "incident", key, "max_runes", maxMessageLenRunes)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		// Pushover rejects empty messages.
		message = "(no details)"
	}
	parameters.Add("message", message)

	supplementaryURL, truncated := notify.TruncateInRunes(tmpl(n.conf.URL), maxURLLenRunes)
	if truncated {
		n.logger.Warn("Truncated URL", "incident", key, "max_runes", maxURLLenRunes)
	}
	parameters.Add("url", supplementaryURL)
	parameters.Add("url_title", tmpl(n.conf.URLTitle))

	parameters.Add("priority", priority)
	parameters.Add("retry", fmt.Sprintf("%d", int64(time.Duration(n.conf.Retry).Seconds())))
	parameters.Add("expire", fmt.Sprintf("%d", int64(time.Duration(n.conf.Expire).Seconds())))
	parameters.Add("device", tmpl(n.conf.Device))
	parameters.Add("sound", tmpl(n.conf.Sound))
	if priority == emergencyPriority {
		parameters.Add("tags", groupKeyTag)
	}
	newttl := int64(time.Duration(n.conf.TTL).Seconds())
	if newttl > 0 {
		parameters.Add("ttl", fmt.Sprintf("%d", newttl))
	}

	u, err = url.Parse(n.apiMessagesURL)
	if err != nil {
		return false, err
	}

	if err != nil {
		return false, err
	}
	shouldRetry, err := n.sendMessage(ctx, key, u, parameters)

	// Notifications sent for firing alerts could be sent with emergency priority, but the resolution notifications
	// might be sent with a different priority. Because of this, and the desire to reduce unnecessary cancel_by_tag
	// call again Pushover, we only call cancel_by_tag if the priority config value contains.
	if err == nil && strings.Contains(n.conf.Priority, emergencyPriority) && alerts.Status() == model.AlertResolved {
		u, err = url.Parse(fmt.Sprintf("%s/%s.json", n.apiReceiptsURL, groupKeyTag))
		if err != nil {
			return false, err
		}
		shouldRetry, err = n.sendMessage(ctx, key, u, parameters)
	}
	return shouldRetry, err
}

func (n *Notifier) sendMessage(ctx context.Context, key notify.Key, u *url.URL, parameters url.Values) (bool, error) {
	u.RawQuery = parameters.Encode()
	// Don't log the URL as it contains secret data (see #1825).
	n.logger.Debug("Sending message", "incident", key)
	resp, err := notify.PostText(ctx, n.client, u.String(), nil)
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
