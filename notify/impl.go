package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

const contentTypeJSON = "application/json"

func Build(confs []*config.NotificationConfig) map[string]Notifier {
	// Create new notifiers. If the type is not implemented yet, fallback
	// to logging notifiers.
	res := map[string]Notifier{}
	for _, nc := range confs {
		var all Notifiers

		for _, wc := range nc.WebhookConfigs {
			all = append(all, &LogNotifier{
				Log:      log.With("notifier", "webhook"),
				Notifier: NewWebhook(wc),
			})
		}
		for range nc.EmailConfigs {
			all = append(all, &LogNotifier{Log: log.With("name", nc.Name)})
		}

		res[nc.Name] = all
	}
	return res
}

type Webhook struct {
	URL string
}

func NewWebhook(conf *config.WebhookConfig) *Webhook {
	return &Webhook{URL: conf.URL}
}

type WebhookMessage struct {
	Version string            `json:"version"`
	Status  model.AlertStatus `json:"status"`
	Alerts  model.Alerts      `json:"alert"`
}

func (w *Webhook) Notify(ctx context.Context, alerts ...*types.Alert) error {
	as := types.Alerts(alerts...)

	msg := &WebhookMessage{
		Version: "1",
		Status:  as.Status(),
		Alerts:  as,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, w.URL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	return nil
}

type Slack struct {
	conf *config.SlackConfig
}

// slackReq is the request for sending a slack notification.
type slackReq struct {
	Channel     string            `json:"channel,omitempty"`
	Attachments []slackAttachment `json:"attachments"`
}

// slackAttachment is used to display a richly-formatted message block.
type slackAttachment struct {
	Title     string `json:"title,omitempty"`
	TitleLink string `json:"title_link,omitempty"`
	Pretext   string `json:"pretext,omitempty"`
	Text      string `json:"text"`
	Fallback  string `json:"fallback"`

	Color    string                 `json:"color,omitempty"`
	MrkdwnIn []string               `json:"mrkdwn_in,omitempty"`
	Fields   []slackAttachmentField `json:"fields,omitempty"`
}

// slackAttachmentField is displayed in a table inside the message attachment.
type slackAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short,omitempty"`
}

func (n *Slack) Notify(ctx context.Context, as ...*types.Alert) error {
	var (
		alerts = types.Alerts(as...)
		color  = n.conf.ColorResolved
		status = string(model.AlertResolved)
	)
	if alerts.HasFiring() {
		color = n.conf.ColorFiring
		status = string(model.AlertFiring)
	}

	var title, link, pretext, text, fallback bytes.Buffer

	if err := tmpl.ExecuteTemplate(&title, n.conf.Templates.Title, alerts); err != nil {
		return err
	}
	if err := tmpl.ExecuteTemplate(&text, n.conf.Templates.Text, alerts); err != nil {
		return err
	}

	attachment := &slackAttachment{
		Title:     title.String(),
		TitleLink: link.String(),
		Pretext:   pretext.String(),
		Text:      text.String(),
		Fallback:  fallback.String(),

		Fields: []slackAttachmentField{{
			Title: "Status",
			Value: status,
			Short: true,
		}},
		Color:    color,
		MrkdwnIn: []string{"fallback", "pretext"},
	}
	req := &slackReq{
		Channel:     n.conf.Channel,
		Attachments: []slackAttachment{*attachment},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, n.conf.URL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	// TODO(fabxc): is 2xx status code really indicator for success for Slack API?
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	return nil
}

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.ParseGlob("templates/*.tmpl"))
}
