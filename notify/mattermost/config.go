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

package mattermost

import (
	"errors"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

var DefaultMattermostConfig = MattermostConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Username:  `{{ template "mattermost.default.username" . }}`,
	Color:     `{{ template "mattermost.default.color" . }}`,
	Text:      `{{ template "mattermost.default.text" . }}`,
	Title:     `{{ template "mattermost.default.title" . }}`,
	TitleLink: `{{ template "mattermost.default.titlelink" . }}`,
	Fallback:  `{{ template "mattermost.default.fallback" . }}`,
}

// MattermostPriority defines the priority for a mattermost notification.
type MattermostPriority struct {
	Priority                string `yaml:"priority,omitempty" json:"priority,omitempty"`
	RequestedAck            bool   `yaml:"requested_ack,omitempty" json:"requested_ack,omitempty"`
	PersistentNotifications bool   `yaml:"persistent_notifications,omitempty" json:"persistent_notifications,omitempty"`
}

// MattermostProps defines additional properties for a mattermost notification.
// Only 'card' property takes effect now.
type MattermostProps struct {
	Card string `yaml:"card,omitempty" json:"card,omitempty"`
}

// MattermostField configures a single Mattermost field for Slack compatibility.
// See https://developers.mattermost.com/integrate/reference/message-attachments/#fields for more information.
type MattermostField struct {
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	Short *bool  `yaml:"short,omitempty" json:"short,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for MattermostField.
func (c *MattermostField) UnmarshalYAML(unmarshal func(any) error) error {
	type plain MattermostField
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Title == "" {
		return errors.New("missing title in Mattermost field configuration")
	}
	if c.Value == "" {
		return errors.New("missing value in Mattermost field configuration")
	}
	return nil
}

// MattermostAttachment defines an attachment for a Mattermost notification.
// See https://developers.mattermost.com/integrate/reference/message-attachments/#fields for more information.
type MattermostAttachment struct {
	Fallback   string             `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	Color      string             `yaml:"color,omitempty" json:"color,omitempty"`
	Pretext    string             `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text       string             `yaml:"text,omitempty" json:"text,omitempty"`
	AuthorName string             `yaml:"author_name,omitempty" json:"author_name,omitempty"`
	AuthorLink string             `yaml:"author_link,omitempty" json:"author_link,omitempty"`
	AuthorIcon string             `yaml:"author_icon,omitempty" json:"author_icon,omitempty"`
	Title      string             `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink  string             `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Fields     []*MattermostField `yaml:"fields,omitempty" json:"fields,omitempty"`
	ThumbURL   string             `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	Footer     string             `yaml:"footer,omitempty" json:"footer,omitempty"`
	FooterIcon string             `yaml:"footer_icon,omitempty" json:"footer_icon,omitempty"`
	ImageURL   string             `yaml:"image_url,omitempty" json:"image_url,omitempty"`
}

// MattermostConfig configures notifications via Mattermost.
// See https://developers.mattermost.com/integrate/webhooks/incoming/ for more information.
type MattermostConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig     *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
	WebhookURL     *amcommoncfg.SecretURL      `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	WebhookURLFile string                      `yaml:"webhook_url_file,omitempty" json:"webhook_url_file,omitempty"`

	Channel  string `yaml:"channel,omitempty" json:"channel,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	Text        string                  `yaml:"text,omitempty" json:"text,omitempty"`
	Fallback    string                  `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	Color       string                  `yaml:"color,omitempty" json:"color,omitempty"`
	Pretext     string                  `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	AuthorName  string                  `yaml:"author_name,omitempty" json:"author_name,omitempty"`
	AuthorLink  string                  `yaml:"author_link,omitempty" json:"author_link,omitempty"`
	AuthorIcon  string                  `yaml:"author_icon,omitempty" json:"author_icon,omitempty"`
	Title       string                  `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string                  `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Fields      []*MattermostField      `yaml:"fields,omitempty" json:"fields,omitempty"`
	ThumbURL    string                  `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	Footer      string                  `yaml:"footer,omitempty" json:"footer,omitempty"`
	FooterIcon  string                  `yaml:"footer_icon,omitempty" json:"footer_icon,omitempty"`
	ImageURL    string                  `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	IconURL     string                  `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	IconEmoji   string                  `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	Attachments []*MattermostAttachment `yaml:"attachments,omitempty" json:"attachments,omitempty"`
	Type        string                  `yaml:"type,omitempty" json:"type,omitempty"`
	Props       *MattermostProps        `yaml:"props,omitempty" json:"props,omitempty"`
	Priority    *MattermostPriority     `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *MattermostConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultMattermostConfig
	type plain MattermostConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.WebhookURL != nil && len(c.WebhookURLFile) > 0 {
		return errors.New("at most one of webhook_url & webhook_url_file must be configured")
	}

	return nil
}
