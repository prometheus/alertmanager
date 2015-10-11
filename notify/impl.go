// Copyright 2015 Prometheus Team
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

package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Build creates a fanout notifier for each notification configuration.
func Build(confs []*config.NotificationConfig, tmpl *template.Template) map[string]Fanout {
	res := map[string]Fanout{}

	for _, nc := range confs {
		var (
			fo  = Fanout{}
			add = func(i int, n Notifier) { fo[fmt.Sprintf("%T/%i", n, i)] = n }
		)

		for i, c := range nc.WebhookConfigs {
			add(i, NewWebhook(c))
		}
		for i, c := range nc.EmailConfigs {
			add(i, NewEmail(c, tmpl))
		}

		res[nc.Name] = fo
	}
	return res
}

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
	tmpl *template.Template
}

func NewEmail(c *config.EmailConfig, t *template.Template) *Email {
	return &Email{conf: c, tmpl: t}
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

	if err := c.Mail(n.conf.Sender); err != nil {
		return err
	}
	if err := c.Rcpt(n.conf.Email); err != nil {
		return err
	}

	log.Debugln("sending mail", n.conf.Email, c)

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	data := struct {
		Alerts model.Alerts
		From   string
		To     string
		Date   string
	}{
		Alerts: types.Alerts(as...),
		From:   n.conf.Sender,
		To:     n.conf.Email,
		Date:   time.Now().Format(time.RFC1123Z),
	}

	if err := n.tmpl.ExecuteText(wc, n.conf.Templates.Header, &data); err != nil {
		return err
	}

	// TODO(fabxc): do a multipart write that considers the plain template.
	return n.tmpl.ExecuteHTML(wc, n.conf.Templates.HTML, &data)
}

type Slack struct {
	conf *config.SlackConfig
	tmpl *template.Template
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

	var err error
	tmpl := func(name string) (s string) {
		if err != nil {
			return
		}
		s, err = n.tmpl.ExecuteHTMLString(name, struct {
			Alerts model.Alerts
		}{
			Alerts: alerts,
		})
		return s
	}

	attachment := &slackAttachment{
		Title:     tmpl(n.conf.Templates.Title),
		TitleLink: tmpl(n.conf.Templates.TitleLink),
		Pretext:   tmpl(n.conf.Templates.Pretext),
		Text:      tmpl(n.conf.Templates.Text),
		Fallback:  tmpl(n.conf.Templates.Fallback),

		Fields: []slackAttachmentField{{
			Title: "Status",
			Value: status,
			Short: true,
		}},
		Color:    color,
		MrkdwnIn: []string{"fallback", "pretext"},
	}
	if err != nil {
		return err
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
