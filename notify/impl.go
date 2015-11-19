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
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"sort"
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

// Build creates a fanout notifier for each receiver.
func Build(confs []*config.Receiver, tmpl *template.Template) map[string]Fanout {
	res := map[string]Fanout{}

	for _, nc := range confs {
		var (
			fo  = Fanout{}
			add = func(i int, n Notifier) { fo[fmt.Sprintf("%T/%d", n, i)] = n }
		)

		for i, c := range nc.WebhookConfigs {
			add(i, NewWebhook(c))
		}
		for i, c := range nc.EmailConfigs {
			add(i, NewEmail(c, tmpl))
		}
		for i, c := range nc.PagerdutyConfigs {
			add(i, NewPagerDuty(c, tmpl))
		}

		res[nc.Name] = fo
	}
	return res
}

const contentTypeJSON = "application/json"

// TemplateData is the data passed to notification templates.
// End-users should not be exposed to Go's type system,
// as this will confuse them and prevent simple things like
// simple equality checks to fail. Map everything to float64/string.
type TemplateData struct {
	Status            string
	Alerts            []TemplateAlert
	AlertCommonLabels map[string]string

	// AlertCommonLabelnames is sorted.
	AlertCommonLabelnames []string
	GroupLabels           map[string]string

	// GroupLabelnames is sorted.
	GroupLabelnames []string
}

// TemplateAlert holds one alert for notification templates.
type TemplateAlert struct {
	Labels      map[string]string
	Annotations map[string]string
}

func generateTemplateData(ctx context.Context, as ...*types.Alert) *TemplateData {
	alerts := types.Alerts(as...)

	groupLabels, ok := GroupLabels(ctx)
	if !ok {
		log.Error("missing group labels")
	}

	data := &TemplateData{
		Status:                string(alerts.Status()),
		Alerts:                make([]TemplateAlert, 0, len(alerts)),
		AlertCommonLabels:     map[string]string{},
		AlertCommonLabelnames: []string{},
		GroupLabels:           map[string]string{},
		GroupLabelnames:       make([]string, 0, len(groupLabels)),
	}

	for _, a := range alerts {
		alert := TemplateAlert{
			Labels:      make(map[string]string, len(a.Labels)),
			Annotations: make(map[string]string, len(a.Annotations)),
		}
		for k, v := range a.Labels {
			alert.Labels[string(k)] = string(v)
		}
		for k, v := range a.Annotations {
			alert.Annotations[string(k)] = string(v)
		}
		data.Alerts = append(data.Alerts, alert)
	}

	sortStart := 0
	for k, v := range groupLabels {
		data.GroupLabels[string(k)] = string(v)

		// Always have the alertname label at the first position.
		if k == model.AlertNameLabel {
			data.GroupLabelnames = append([]string{string(k)}, data.GroupLabelnames...)
			sortStart = 1
		} else {
			data.GroupLabelnames = append(data.GroupLabelnames, string(k))
		}
	}
	sort.Strings(data.GroupLabelnames[sortStart:])

	if len(alerts) >= 1 {
		common := alerts[0].Labels.Clone()
		for _, a := range alerts[1:] {
			for ln, lv := range common {
				if a.Labels[ln] != lv {
					delete(common, ln)
				}
			}
		}
		for k, v := range common {
			data.AlertCommonLabels[string(k)] = string(v)
			data.AlertCommonLabelnames = append(data.AlertCommonLabelnames, string(k))
		}
	}
	sort.Strings(data.AlertCommonLabelnames)

	return data
}

// Webhook implements a Notifier for generic webhooks.
type Webhook struct {
	// The URL to which notifications are sent.
	URL string
}

// NewWebhook returns a new Webhook.
func NewWebhook(conf *config.WebhookConfig) *Webhook {
	return &Webhook{URL: conf.URL}
}

// WebhookMessage defines the JSON object send to webhook endpoints.
type WebhookMessage struct {
	// The protocol version.
	Version string `json:"version"`
	// The alert status. It is firing iff any of the alerts is not resolved.
	Status model.AlertStatus `json:"status"`
	// A batch of alerts.
	Alerts model.Alerts `json:"alert"`
}

// Notify implements the Notifier interface.
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

// Email implements a Notifier for email notifications.
type Email struct {
	conf *config.EmailConfig
	tmpl *template.Template
}

// NewEmail returns a new Email notifier.
func NewEmail(c *config.EmailConfig, t *template.Template) *Email {
	return &Email{conf: c, tmpl: t}
}

// auth resolves a string of authentication mechanisms.
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

// Notify implements the Notifier interface.
func (n *Email) Notify(ctx context.Context, as ...*types.Alert) error {
	data := generateTemplateData(ctx, as...)

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

	from, err := n.tmpl.ExecuteTextString(n.conf.From, data)
	if err != nil {
		return fmt.Errorf("executing from template: %s", err)
	}
	addrs, err := mail.ParseAddressList(from)
	if err != nil {
		return fmt.Errorf("parsing from addresses: %s", err)
	}
	if len(addrs) != 1 {
		return fmt.Errorf("must be exactly one from address")
	}
	if err := c.Mail(addrs[0].Address); err != nil {
		return fmt.Errorf("sending mail from: %s", err)
	}
	to, err := n.tmpl.ExecuteTextString(n.conf.To, data)
	if err != nil {
		return fmt.Errorf("executing to template: %s", err)
	}
	addrs, err = mail.ParseAddressList(to)
	if err != nil {
		return fmt.Errorf("parsing to addresses: %s", err)
	}
	for _, addr := range addrs {
		if err := c.Rcpt(addr.Address); err != nil {
			return fmt.Errorf("sending rcpt to: %s", err)
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	for header, tmpl := range n.conf.Headers {
		value, err := n.tmpl.ExecuteTextString(tmpl, data)
		if err != nil {
			return fmt.Errorf("executing %q header template: %s", header, err)
		}
		fmt.Fprintf(wc, "%s: %s\r\n", header, mime.QEncoding.Encode("utf-8", value))
	}
	fmt.Fprintf(wc, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(wc, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	// TODO: Add some useful headers here, such as URL of the alertmanager
	// and active/resolved.
	fmt.Fprintf(wc, "\r\n")

	// TODO(fabxc): do a multipart write that considers the plain template.
	body, err := n.tmpl.ExecuteHTMLString(n.conf.HTML, data)
	if err != nil {
		return fmt.Errorf("executing email html template: %s", err)
	}
	_, err = io.WriteString(wc, body)
	return err
}

// PagerDuty implements a Notifier for PagerDuty notifications.
type PagerDuty struct {
	conf *config.PagerdutyConfig
	tmpl *template.Template
}

// NewPagerDuty returns a new PagerDuty notifier.
func NewPagerDuty(c *config.PagerdutyConfig, t *template.Template) *PagerDuty {
	return &PagerDuty{conf: c, tmpl: t}
}

const (
	pagerDutyEventTrigger = "trigger"
	pagerDutyEventResolve = "resolve"
)

type pagerDutyMessage struct {
	ServiceKey  string            `json:"service_key"`
	EventType   string            `json:"event_type"`
	Description string            `json:"description"`
	IncidentKey model.Fingerprint `json:"incident_key"`
	Client      string            `json:"client,omitempty"`
	ClientURL   string            `json:"client_url,omitempty"`
	Details     map[string]string `json:"details"`
}

// Notify implements the Notifier interface.
func (n *PagerDuty) Notify(ctx context.Context, as ...*types.Alert) error {
	// http://developer.pagerduty.com/documentation/integration/events/trigger
	alerts := types.Alerts(as...)
	data := generateTemplateData(ctx, as...)

	eventType := pagerDutyEventTrigger
	if alerts.Status() == model.AlertResolved {
		eventType = pagerDutyEventResolve
	}

	key, ok := GroupKey(ctx)
	if !ok {
		return fmt.Errorf("group key missing")
	}

	log.With("incident", key).With("eventType", eventType).Debugln("notifying PagerDuty")

	var err error
	tmpl := func(name string) (s string) {
		if err != nil {
			return
		}
		s, err = n.tmpl.ExecuteTextString(name, data)
		return s
	}
	details := make(map[string]string, len(n.conf.Details))
	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	msg := &pagerDutyMessage{
		ServiceKey:  tmpl(n.conf.ServiceKey),
		EventType:   eventType,
		IncidentKey: key,
		Description: tmpl(n.conf.Description),
		Details:     details,
	}
	if eventType == pagerDutyEventTrigger {
		msg.Client = "Prometheus Alertmanager"
		msg.ClientURL = ""
	}
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, n.conf.URL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}
	return nil
}

// Slack implements a Notifier for Slack notifications.
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

// Notify implements the Notifier interface.
func (n *Slack) Notify(ctx context.Context, as ...*types.Alert) error {
	data := generateTemplateData(ctx, as...)
	alerts := types.Alerts(as...)

	var err error
	tmplText := func(name string) (s string) {
		if err != nil {
			return
		}
		s, err = n.tmpl.ExecuteTextString(name, data)
		return s
	}
	tmplHTML := func(name string) (s string) {
		if err != nil {
			return
		}
		s, err = n.tmpl.ExecuteHTMLString(name, data)
		return s
	}

	attachment := &slackAttachment{
		Title:     tmplText(n.conf.Title),
		TitleLink: tmplText(n.conf.TitleLink),
		Pretext:   tmplText(n.conf.Pretext),
		Text:      tmplHTML(n.conf.Text),
		Fallback:  tmplText(n.conf.Fallback),

		Fields: []slackAttachmentField{{
			Title: "Status",
			Value: string(alerts.Status()),
			Short: true,
		}},
		Color:    tmplText(n.conf.Color),
		MrkdwnIn: []string{"fallback", "pretext"},
	}
	req := &slackReq{
		Channel:     tmplText(n.conf.Channel),
		Attachments: []slackAttachment{*attachment},
	}
	if err != nil {
		return err
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
