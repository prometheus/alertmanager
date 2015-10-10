package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	// "github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

const contentTypeJSON = "application/json"

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

type Email struct {
	conf *config.EmailConfig
}

func NewEmail(ec *config.EmailConfig) *Email {
	return &Email{conf: ec}
}

func (n *Email) auth(mechs string) (smtp.Auth, *tls.Config, error) {
	username := os.Getenv("SMTP_AUTH_USERNAME")

	for _, mech := range strings.Split(mechs, " ") {
		switch mech {
		case "CRAM-MD5":
			secret := os.Getenv("SMTP_AUTH_SECRET")
			if secret == "" {
				continue
			}
			return smtp.CRAMMD5Auth(username, secret), nil, nil

		case "PLAIN":
			password := os.Getenv("SMTP_AUTH_PASSWORD")
			if password == "" {
				continue
			}
			identity := os.Getenv("SMTP_AUTH_IDENTITY")

			// We need to know the hostname for both auth and TLS.
			host, _, err := net.SplitHostPort(n.conf.Smarthost)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid address: %s", err)
			}
			var (
				auth = smtp.PlainAuth(identity, username, password, host)
				cfg  = &tls.Config{ServerName: host}
			)
			return auth, cfg, nil
		}
	}
	return nil, nil, nil
}

func (n *Email) Notify(ctx context.Context, as ...*types.Alert) error {
	// Connect to the SMTP smarthost.
	c, err := smtp.Dial(n.conf.Smarthost)
	if err != nil {
		return err
	}
	defer c.Quit()

	if ok, mech := c.Extension("AUTH"); ok {
		auth, tlsConf, err := n.auth(mech)
		if err != nil {
			return err
		}
		if tlsConf != nil {
			if err := c.StartTLS(tlsConf); err != nil {
				return fmt.Errorf("starttls failed: %s", err)
			}
		}
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("%T failed: %s", auth, err)
			}
		}
	}

	c.Mail(n.conf.Sender)
	c.Rcpt(n.conf.Email)

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	// TODO(fabxc): do a multipart write that considers the plain template.
	return tmpl.ExecuteTemplate(wc, n.conf.Templates.HTML, struct {
		Alerts model.Alerts
		From   string
		To     string
		Date   string
	}{
		Alerts: types.Alerts(as...),
		From:   n.conf.Sender,
		To:     n.conf.Email,
		Date:   time.Now().Format(time.RFC1123Z),
	})
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
	alerts := types.Alerts(as...)
	var (
		color  = n.conf.ColorResolved
		status = string(alerts.Status())
	)
	if alerts.HasFiring() {
		color = n.conf.ColorFiring
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

func SetTemplate(t *template.Template) {
	tmpl = t
}
