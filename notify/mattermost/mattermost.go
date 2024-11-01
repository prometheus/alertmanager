package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
)

// Mattermost supports 16383 chars max.
// https://developers.mattermost.com/integrate/webhooks/incoming/#tips-and-best-practices
const maxTextLenRunes = 16383

// Notifier implements a Notifier for Mattermost notifications.
type Notifier struct {
	conf    *config.MattermostConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier

	postJSONFunc func(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error)
}

// New returns a new Mattermost notifier.
func New(c *config.MattermostConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "mattermost", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:         c,
		tmpl:         t,
		logger:       l,
		client:       client,
		retrier:      &notify.Retrier{},
		postJSONFunc: notify.PostJSON,
	}, nil
}

// request is the request for sending a Mattermost notification.
// https://developers.mattermost.com/integrate/webhooks/incoming/#parameters
type request struct {
	Text        string                     `json:"text"`
	Channel     string                     `json:"channel,omitempty"`
	Username    string                     `json:"username,omitempty"`
	IconURL     string                     `json:"icon_url,omitempty"`
	IconEmoji   string                     `json:"icon_emoji,omitempty"`
	Attachments []attachment               `json:"attachments,omitempty"`
	Type        string                     `json:"type,omitempty"`
	Props       *config.MattermostProps    `json:"props,omitempty"`
	Priority    *config.MattermostPriority `json:"priority,omitempty"`
}

// attachment is used to display a richly-formatted message block for compatibility with Slack.
// https://developers.mattermost.com/integrate/reference/message-attachments/
type attachment struct {
	Fallback   string                   `json:"fallback,omitempty"`
	Color      string                   `json:"color,omitempty"`
	Pretext    string                   `json:"pretext,omitempty"`
	Text       string                   `json:"text,omitempty"`
	AuthorName string                   `json:"author_name,omitempty"`
	AuthorLink string                   `json:"author_link,omitempty"`
	AuthorIcon string                   `json:"author_icon,omitempty"`
	Title      string                   `json:"title,omitempty"`
	TitleLink  string                   `json:"title_link,omitempty"`
	Fields     []config.MattermostField `json:"fields,omitempty"`
	ThumbURL   string                   `json:"thumb_url,omitempty"`
	Footer     string                   `json:"footer,omitempty"`
	FooterIcon string                   `json:"footer_icon,omitempty"`
	ImageURL   string                   `json:"image_url,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	var (
		err  error
		url  string
		data = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
	)

	if n.conf.WebhookURL != nil {
		url = n.conf.WebhookURL.String()
	} else {
		content, err := os.ReadFile(n.conf.WebhookURLFile)
		if err != nil {
			return false, err
		}
		url = strings.TrimSpace(string(content))
	}
	if url == "" {
		return false, errors.New("webhook url missing")
	}

	req := n.createRequest(notify.TmplText(n.tmpl, data, &err))
	if err != nil {
		return false, err
	}
	err = n.sanitizeRequest(ctx, req)
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	resp, err := n.postJSONFunc(ctx, n.client, url, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	// Use a retrier to generate an error message for non-200 responses and
	// classify them as retriable or not.
	retry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		err = fmt.Errorf("channel %q: %w", req.Channel, err)
		return retry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	level.Debug(n.logger).Log(
		"msg", "Message sent to Mattermost successfully",
		"status", resp.StatusCode)

	return false, nil
}

func (n *Notifier) createRequest(tmpl func(string) string) *request {
	req := &request{
		Text:      tmpl(n.conf.Text),
		Channel:   tmpl(n.conf.Channel),
		Username:  tmpl(n.conf.Username),
		IconURL:   tmpl(n.conf.IconURL),
		IconEmoji: tmpl(n.conf.IconEmoji),
		Type:      tmpl(n.conf.Type),
	}

	if n.conf.Priority != nil && n.conf.Priority.Priority != "" {
		req.Priority = &config.MattermostPriority{
			Priority:                tmpl(n.conf.Priority.Priority),
			RequestedAck:            n.conf.Priority.RequestedAck,
			PersistentNotifications: n.conf.Priority.PersistentNotifications,
		}
	}

	if n.conf.Props != nil && n.conf.Props.Card != "" {
		req.Props = &config.MattermostProps{
			Card: tmpl(n.conf.Props.Card),
		}
	}

	lenAtt := len(n.conf.Attachments)
	if lenAtt > 0 {
		req.Attachments = make([]attachment, lenAtt)
		for idxAtt, cfgAtt := range n.conf.Attachments {
			att := attachment{
				Fallback:   tmpl(cfgAtt.Fallback),
				Color:      tmpl(cfgAtt.Color),
				Pretext:    tmpl(cfgAtt.Pretext),
				Text:       tmpl(cfgAtt.Text),
				AuthorName: tmpl(cfgAtt.AuthorName),
				AuthorLink: tmpl(cfgAtt.AuthorLink),
				AuthorIcon: tmpl(cfgAtt.AuthorIcon),
				Title:      tmpl(cfgAtt.Title),
				TitleLink:  tmpl(cfgAtt.TitleLink),
				ThumbURL:   tmpl(cfgAtt.ThumbURL),
				Footer:     tmpl(cfgAtt.Footer),
				FooterIcon: tmpl(cfgAtt.FooterIcon),
				ImageURL:   tmpl(cfgAtt.ImageURL),
			}

			lenFields := len(cfgAtt.Fields)
			if lenFields > 0 {
				att.Fields = make([]config.MattermostField, lenFields)
				for idxField, field := range cfgAtt.Fields {
					att.Fields[idxField] = config.MattermostField{
						Title: tmpl(field.Title),
						Value: tmpl(field.Value),
						Short: field.Short,
					}
				}
			}

			req.Attachments[idxAtt] = att
		}
	}

	return req
}

func (n *Notifier) sanitizeRequest(ctx context.Context, r *request) error {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return err
	}

	// Truncate the text if it's too long.
	text, truncated := notify.TruncateInRunes(r.Text, maxTextLenRunes)
	if truncated {
		level.Warn(n.logger).Log(
			"msg", "Truncated text",
			"key", key,
			"max_runes", maxTextLenRunes)
		r.Text = text
	}

	if r.Priority == nil {
		return nil
	}

	// Check priority
	const (
		priorityUrgent    = "urgent"
		priorityImportant = "important"
		priorityStandard  = "standard"
	)

	switch strings.ToLower(r.Priority.Priority) {
	case priorityUrgent, priorityImportant, priorityStandard:
		r.Priority.Priority = strings.ToLower(r.Priority.Priority)
	default:
		level.Warn(n.logger).Log(
			"msg", "Priority is set to standard due to invalid value",
			"key", key,
			"priority", r.Priority.Priority)
		r.Priority.Priority = priorityStandard
	}

	// Check RequestedAck flag
	if r.Priority.RequestedAck && r.Priority.Priority == priorityStandard {
		level.Warn(n.logger).Log(
			"msg", "RequestedAck is set to false due to priority is standard",
			"key", key,
		)
		r.Priority.RequestedAck = false
	}

	// Check PersistentNotifications flag
	if r.Priority.PersistentNotifications && r.Priority.Priority != priorityUrgent {
		level.Warn(n.logger).Log(
			"msg", "PersistentNotifications is set to false due to priority is not urgent",
			"key", key,
		)
		r.Priority.PersistentNotifications = false
	}

	return nil
}
