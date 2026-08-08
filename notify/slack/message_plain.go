// Copyright The Prometheus Authors
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
	"log/slog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
)

// https://api.slack.com/reference/messaging/attachments#legacy_fields - 1024, no units given, assuming runes or characters.
const maxTitleLenRunes = 1024

// composePlainRequest builds the current attachment-based Slack payload.
func composePlainRequest(c *config.SlackConfig, tmplText func(string) string, logger *slog.Logger) *request {
	var markdownIn []string
	if len(c.MrkdwnIn) == 0 {
		markdownIn = []string{"fallback", "pretext", "text"}
	} else {
		markdownIn = c.MrkdwnIn
	}

	title, truncated := notify.TruncateInRunes(tmplText(c.Title), maxTitleLenRunes)
	if truncated {
		logger.Warn("Truncated title", "max_runes", maxTitleLenRunes)
	}

	att := attachment{
		Title:      title,
		TitleLink:  tmplText(c.TitleLink),
		Pretext:    tmplText(c.Pretext),
		Text:       tmplText(c.Text),
		Fallback:   tmplText(c.Fallback),
		CallbackID: tmplText(c.CallbackID),
		ImageURL:   tmplText(c.ImageURL),
		ThumbURL:   tmplText(c.ThumbURL),
		Footer:     tmplText(c.Footer),
		Color:      tmplText(c.Color),
		MrkdwnIn:   markdownIn,
	}

	numFields := len(c.Fields)
	if numFields > 0 {
		fields := make([]config.SlackField, numFields)
		for index, field := range c.Fields {
			// Check if short was defined for the field otherwise fallback to the global setting.
			var short bool
			if field.Short != nil {
				short = *field.Short
			} else {
				short = c.ShortFields
			}

			// Rebuild the field by executing templates and preserving short semantics.
			fields[index] = config.SlackField{
				Title: tmplText(field.Title),
				Value: tmplText(field.Value),
				Short: &short,
			}
		}
		att.Fields = fields
	}

	numActions := len(c.Actions)
	if numActions > 0 {
		actions := make([]config.SlackAction, numActions)
		for index, action := range c.Actions {
			slackAction := config.SlackAction{
				Type:  tmplText(action.Type),
				Text:  tmplText(action.Text),
				URL:   tmplText(action.URL),
				Style: tmplText(action.Style),
				Name:  tmplText(action.Name),
				Value: tmplText(action.Value),
			}

			if action.ConfirmField != nil {
				slackAction.ConfirmField = &config.SlackConfirmationField{
					Title:       tmplText(action.ConfirmField.Title),
					Text:        tmplText(action.ConfirmField.Text),
					OkText:      tmplText(action.ConfirmField.OkText),
					DismissText: tmplText(action.ConfirmField.DismissText),
				}
			}

			actions[index] = slackAction
		}
		att.Actions = actions
	}

	return &request{
		Channel:     tmplText(c.Channel),
		Username:    tmplText(c.Username),
		IconEmoji:   tmplText(c.IconEmoji),
		IconURL:     tmplText(c.IconURL),
		LinkNames:   c.LinkNames,
		Text:        tmplText(c.MessageText),
		Attachments: []attachment{att},
	}
}
