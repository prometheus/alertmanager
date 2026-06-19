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

package googlechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

type Notifier struct {
	conf    *config.GoogleChatConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

func New(conf *config.GoogleChatConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "googlechat", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:    conf,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
	}, nil
}

// Text-only message payload (legacy format).
type textMessage struct {
	Text string `json:"text"`
}

// Cards v2 payload types.
// https://developers.google.com/workspace/chat/api/reference/rest/v1/cards

type cardPayload struct {
	Text    string   `json:"text,omitempty"`
	CardsV2 []cardV2 `json:"cardsV2,omitempty"`
}

type cardV2 struct {
	CardID string   `json:"cardId"`
	Card   cardBody `json:"card"`
}

type cardBody struct {
	Header   *cardHeader `json:"header,omitempty"`
	Sections []section   `json:"sections,omitempty"`
}

type cardHeader struct {
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle,omitempty"`
	ImageURL     string `json:"imageUrl,omitempty"`
	ImageType    string `json:"imageType,omitempty"`
	ImageAltText string `json:"imageAltText,omitempty"`
}

type section struct {
	Header                    string   `json:"header,omitempty"`
	Widgets                   []widget `json:"widgets"`
	Collapsible               bool     `json:"collapsible,omitempty"`
	UncollapsibleWidgetsCount int      `json:"uncollapsibleWidgetsCount,omitempty"`
}

type widget struct {
	DecoratedText *decoratedText `json:"decoratedText,omitempty"`
	ButtonList    *buttonList    `json:"buttonList,omitempty"`
	TextParagraph *textParagraph `json:"textParagraph,omitempty"`
	Divider       *divider       `json:"divider,omitempty"`
}

type decoratedText struct {
	TopLabel string `json:"topLabel,omitempty"`
	Text     string `json:"text"`
	WrapText bool   `json:"wrapText,omitempty"`
}

type textParagraph struct {
	Text string `json:"text"`
}

type buttonList struct {
	Buttons []button `json:"buttons"`
}

type button struct {
	Text    string  `json:"text"`
	OnClick onClick `json:"onClick"`
}

type onClick struct {
	OpenLink openLink `json:"openLink"`
}

type openLink struct {
	URL string `json:"url"`
}

type divider struct{}

func buildCard(title, subtitle, imageURL, message, details, labels, actions string) *cardBody {
	card := &cardBody{}

	if title != "" {
		card.Header = &cardHeader{
			Title:    title,
			Subtitle: subtitle,
		}
		if imageURL != "" {
			card.Header.ImageURL = imageURL
			card.Header.ImageType = "CIRCLE"
			card.Header.ImageAltText = "Alert"
		}
	}

	if message != "" {
		card.Sections = append(card.Sections, section{
			Widgets: []widget{{TextParagraph: &textParagraph{Text: message}}},
		})
	}

	if labels != "" {
		widgets := parseKeyValueWidgets(labels)
		if len(widgets) > 0 {
			card.Sections = append(card.Sections, section{
				Header:                    "Labels",
				Widgets:                   widgets,
				Collapsible:               true,
				UncollapsibleWidgetsCount: 2,
			})
		}
	}

	if details != "" {
		widgets := parseKeyValueWidgets(details)
		if len(widgets) > 0 {
			card.Sections = append(card.Sections, section{
				Header:                    "Alert Details",
				Widgets:                   widgets,
				Collapsible:               true,
				UncollapsibleWidgetsCount: 0,
			})
		}
	}

	if actions != "" {
		buttons := parseActionButtons(actions)
		if len(buttons) > 0 {
			card.Sections = append(card.Sections, section{
				Widgets: []widget{{ButtonList: &buttonList{Buttons: buttons}}},
			})
		}
	}

	return card
}

func parseKeyValueWidgets(s string) []widget {
	var widgets []widget
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		widgets = append(widgets, widget{
			DecoratedText: &decoratedText{
				TopLabel: strings.TrimSpace(parts[0]),
				Text:     strings.TrimSpace(parts[1]),
				WrapText: true,
			},
		})
	}
	return widgets
}

func parseActionButtons(s string) []button {
	var buttons []button
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		label := strings.TrimSpace(parts[0])
		link := strings.TrimSpace(parts[1])
		if label == "" || link == "" {
			continue
		}
		buttons = append(buttons, button{
			Text:    label,
			OnClick: onClick{OpenLink: openLink{URL: link}},
		})
	}
	return buttons
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}

	var (
		data = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl = notify.TmplText(n.tmpl, data, &err)
	)

	message := tmpl(n.conf.Message)

	cardTitle := tmpl(n.conf.CardTitle)
	cardSubtitle := tmpl(n.conf.CardSubtitle)
	cardImageURL := tmpl(n.conf.CardImageURL)
	cardMessage := tmpl(n.conf.CardMessage)
	cardDetails := tmpl(n.conf.CardDetails)
	cardLabels := tmpl(n.conf.CardLabels)
	cardActions := tmpl(n.conf.CardActions)

	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	card := buildCard(cardTitle, cardSubtitle, cardImageURL, cardMessage, cardDetails, cardLabels, cardActions)
	if card.Header != nil || len(card.Sections) > 0 {
		payload := cardPayload{
			CardsV2: []cardV2{
				{
					CardID: key.Hash(),
					Card:   *card,
				},
			},
		}
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			return false, err
		}
	} else {
		if err := json.NewEncoder(&buf).Encode(textMessage{Text: message}); err != nil {
			return false, err
		}
	}

	var webhookURL string
	if n.conf.URL != nil {
		webhookURL = n.conf.URL.String()
	} else {
		content, err := os.ReadFile(n.conf.URLFile)
		if err != nil {
			return false, fmt.Errorf("read url_file: %w", err)
		}
		webhookURL = strings.TrimSpace(string(content))
	}

	// https://developers.google.com/chat/how-tos/webhooks#start_a_message_thread
	u, err := url.Parse(webhookURL)
	if err != nil {
		return false, fmt.Errorf("unable to parse googlechat url: %w", err)
	}
	q := u.Query()
	if n.conf.Threading {
		q.Set("threadKey", key.Hash())
		q.Set("messageReplyOption", "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
	}
	u.RawQuery = q.Encode()
	webhookURL = u.String()

	resp, err := notify.PostJSON(ctx, n.client, webhookURL, &buf)
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
