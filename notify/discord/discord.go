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
	"sort"
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
	// https://discord.com/developers/docs/resources/channel#create-message
	// 2000 characters per message is the limit (not counting embeds)
	maxMessageContentLength = 2000
	// 10 embeds per Message is the maximum
	maxEmbedsPerMessage = 10
	// https://discord.com/developers/docs/resources/channel#embed-object-embed-limits
	// 256 characters or runes for an embed title
	maxTitleLenRunes = 256
	// 4096 characters or runes for an embed description
	maxDescriptionLenRunes = 4096
	// 25 fields per embed
	maxFieldsPerEmbed = 25
	// 256 characters or runes for an embed field-name
	maxFieldNameLenRunes = 256
	// 1024 characters or runes for an embed field-value
	maxFieldValueLenRunes = 1024
	// 256 characters or runes for an embed author name
	maxEmbedAuthorNameLenRunes = 256
	// 6000 characters or runes for the combined sum of characters in all title, description, field.name, field.value, footer.text, and author.name of all embeds
	maxTotalEmbedSize = 6000
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
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Content   string         `json:"content,omitempty"`
	Embeds    []webhookEmbed `json:"embeds"`
}

type webhookEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color"`
	Fields      []webhookEmbedField `json:"fields,omitempty"`
	Footer      webhookEmbedFooter  `json:"footer,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
}

type webhookEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type webhookEmbedFooter struct {
	Text    string `json:"text,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	level.Debug(n.logger).Log("incident", key)

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	author, truncated := notify.TruncateInRunes(tmpl(n.conf.BotUsername), maxEmbedAuthorNameLenRunes)
	if truncated {
		level.Warn(n.logger).Log("msg", "Truncated author name", "key", key, "max_runes", maxEmbedAuthorNameLenRunes)
	}

	alertsOmittedMessage, truncated := notify.TruncateInRunes(tmpl(n.conf.AlertsOmittedMessage), maxMessageContentLength)
	if truncated {
		level.Warn(n.logger).Log("msg", "Truncated alerts omitted message", "key", key, "max_message_length", maxMessageContentLength)
	}

	w := webhook{
		Username:  author,
		AvatarURL: tmpl(n.conf.BotIconURL),
	}

	var alerts = types.Alerts(as...)

	for alertIndex, alert := range alerts {
		if alertIndex == maxEmbedsPerMessage {
			w.Content = alertsOmittedMessage
			break
		}

		title, truncated := notify.TruncateInRunes(tmpl(n.conf.Title), maxTitleLenRunes)
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated title", "key", key, "max_runes", maxTitleLenRunes)
		}

		description, truncated := notify.TruncateInRunes(tmpl(n.conf.Message), maxDescriptionLenRunes)
		if truncated {
			level.Warn(n.logger).Log("msg", "Truncated message", "key", key, "max_runes", maxDescriptionLenRunes)
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
			sortedLabelNames := make([]string, 0, len(alert.Labels))

			for labelName, _ := range alert.Labels {
				sortedLabelNames = append(sortedLabelNames, string(labelName))
			}

			sort.Strings(sortedLabelNames)

			for i, labelName := range sortedLabelNames {
				if i > maxFieldsPerEmbed {
					level.Warn(n.logger).Log("msg", "Truncated Fields", "key", key, "max_entries", maxFieldsPerEmbed)
					break
				}

				labelValue := string(alert.Labels[model.LabelName(labelName)])

				label, truncated := notify.TruncateInRunes(labelName, maxFieldNameLenRunes)
				if truncated {
					level.Warn(n.logger).Log("msg", "Truncated field name", "key", key, "max_runes", maxFieldNameLenRunes)
				}

				value, truncated := notify.TruncateInRunes(labelValue, maxFieldValueLenRunes)
				if truncated {
					level.Warn(n.logger).Log("msg", "Truncated field value", "key", key, "max_runes", maxFieldValueLenRunes)
				}

				fields = append(fields, webhookEmbedField{
					Name:   label,
					Value:  value,
					Inline: true,
				})
			}
		}

		embed := webhookEmbed{
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
		}

		if sumEmbedsTextLength(w.Embeds)+calculateEmbedTextLength(embed) > maxTotalEmbedSize {
			w.Content = alertsOmittedMessage
			break
		}

		w.Embeds = append(w.Embeds, embed)
	}

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

	return false, nil
}

func sumEmbedsTextLength(embeds []webhookEmbed) (sum int) {
	for _, embed := range embeds {
		sum += calculateEmbedTextLength(embed)
	}
	return
}

func calculateEmbedTextLength(embed webhookEmbed) int {
	var fieldLen int
	for _, field := range embed.Fields {
		fieldLen += len(field.Name) + len(field.Value)
	}
	return len(embed.Title) + len(embed.Description) + len(embed.Footer.Text) + fieldLen
}
