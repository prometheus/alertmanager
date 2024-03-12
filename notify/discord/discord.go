// Copyright 2021 Prometheus Team
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

package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 256 characters or runes.
	maxTitleLenRunes = 256
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 4096 characters or runes.
	maxDescriptionLenRunes = 4096
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 25 fields per embed
	maxFieldsPerEmbed = 25
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 256 characters or runes
	maxFieldNameLenRunes = 256
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 1024 characters or runes
	maxFieldValueLenRunes = 1024
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits - 256 characters or runes
	maxEmbedAuthorNameLenRunes = 256
)

const (
	colorRed   = 0x992D22
	colorGreen = 0x2ECC71
	colorGrey  = 0x95A5A6
)

// Notifier implements a Notifier for Discord notifications.
type Notifier struct {
	conf       *config.DiscordConfig
	tmpl       *template.Template
	logger     log.Logger
	client     *http.Client
	retrier    *notify.Retrier
	webhookURL *config.SecretURL
}

// New returns a new Discord notifier.
func New(c *config.DiscordConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "discord", httpOpts...)
	if err != nil {
		return nil, err
	}
	n := &Notifier{
		conf:       c,
		tmpl:       t,
		logger:     l,
		client:     client,
		retrier:    &notify.Retrier{},
		webhookURL: c.WebhookURL,
	}
	return n, nil
}

type webhook struct {
	Content   string         `json:"content"`
	Embeds    []webhookEmbed `json:"embeds"`
	Username  string         `json:"username"`
	AvatarURL string         `json:"avatar_url"`
}

type webhookEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	URL         string              `json:"url"`
	Color       int                 `json:"color"`
	Fields      []webhookEmbedField `json:"fields"`
	Footer      webhookEmbedFooter  `json:"footer"`
	Timestamp   time.Time           `json:"timestamp"`
}

type webhookEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type webhookEmbedFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	level.Debug(n.logger).Log("incident", key)

	alerts := types.Alerts(as...)

	for _, alert := range alerts {
		data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmpl := notify.TmplText(n.tmpl, data, &err)
		if err != nil {
			return false, err
		}

		author, truncated := notify.TruncateInRunes(tmpl(n.conf.BotUsername), maxEmbedAuthorNameLenRunes)
		if err != nil {
			return false, err
		}
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated author name", "key", key, "max_runes", maxEmbedAuthorNameLenRunes)
		}
		title, truncated := notify.TruncateInRunes(tmpl(n.conf.Title), maxTitleLenRunes)
		if err != nil {
			return false, err
		}
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated title", "key", key, "max_runes", maxTitleLenRunes)
		}
		description, truncated := notify.TruncateInRunes(tmpl(n.conf.Message), maxDescriptionLenRunes)
		if err != nil {
			return false, err
		}
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated message", "key", key, "max_runes", maxDescriptionLenRunes)
		}

		w := webhook{
			Username:  author,
			AvatarURL: tmpl(n.conf.BotIconURL),
		}

		var color int
		var timestamp time.Time

		switch alert.Status() {
		case model.AlertFiring:
			color = colorRed
			timestamp = alert.StartsAt
		case model.AlertResolved:
			color = colorGreen
			timestamp = alert.EndsAt
		default:
			color = colorGrey
			timestamp = time.Now()
		}

		var fields []webhookEmbedField

		if !n.conf.SkipFields {
			labelCount := 0
			for labelName, labelValue := range alert.Labels {
				if labelCount >= maxFieldsPerEmbed {
					level.Warn(n.logger).Log("msg", "Truncated Fields", "key", key, "max_entries", maxFieldsPerEmbed)
					break
				}

				label, truncated := notify.TruncateInRunes(string(labelName), maxFieldNameLenRunes)
				if err != nil {
					return false, err
				}
				if truncated {
					level.Warn(n.logger).Log("msg", "Truncated field name", "key", key, "max_runes", maxFieldNameLenRunes)
				}
				value, truncated := notify.TruncateInRunes(string(labelValue), maxFieldValueLenRunes)
				if err != nil {
					return false, err
				}
				if truncated {
					level.Warn(n.logger).Log("msg", "Truncated field value", "key", key, "max_runes", maxFieldValueLenRunes)
				}

				fields = append(fields, webhookEmbedField{
					Name:   label,
					Value:  value,
					Inline: true,
				})

				labelCount++
			}
		}

		w.Embeds = append(w.Embeds, webhookEmbed{
			Title:       title,
			Description: description,
			Color:       color,
			Fields:      fields,
			Timestamp:   timestamp,
			URL:         tmpl(n.conf.TitleURL),
			Footer: webhookEmbedFooter{
				Text:    alert.Fingerprint().String(),
				IconURL: tmpl(n.conf.BotIconURL),
			},
		})

		var url string
		if n.conf.WebhookURL != nil {
			url = n.conf.WebhookURL.String()
		} else {
			content, err := os.ReadFile(n.conf.WebhookURLFile)
			if err != nil {
				return false, fmt.Errorf("read webhook_url_file: %w", err)
			}
			url = strings.TrimSpace(string(content))
		}

		var payload bytes.Buffer
		if err = json.NewEncoder(&payload).Encode(w); err != nil {
			return false, err
		}

		resp, err := notify.PostJSON(ctx, n.client, url, &payload)
		if err != nil {
			return true, notify.RedactURL(err)
		}

		shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
		if err != nil {
			return shouldRetry, err
		}
	}

	return false, nil
}
