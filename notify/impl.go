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
	"io/ioutil"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var (
	numNotifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "notifications_total",
		Help:      "The total number of attempted notifications.",
	}, []string{"integration"})

	numFailedNotifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "notifications_failed_total",
		Help:      "The total number of failed notifications.",
	}, []string{"integration"})
)

func init() {
	prometheus.Register(numNotifications)
	prometheus.Register(numFailedNotifications)
}

type notifierConfig interface {
	SendResolved() bool
}

type NotifierFunc func(context.Context, ...*types.Alert) error

func (f NotifierFunc) Notify(ctx context.Context, alerts ...*types.Alert) error {
	return f(ctx, alerts...)
}

type integration interface {
	Notifier
	name() string
}

// Build creates a fanout notifier for each receiver.
func Build(confs []*config.Receiver, tmpl *template.Template) map[string]Fanout {
	res := map[string]Fanout{}

	filter := func(n integration, c notifierConfig) Notifier {
		return NotifierFunc(func(ctx context.Context, alerts ...*types.Alert) error {
			var res []*types.Alert

			if c.SendResolved() {
				res = alerts
			} else {
				for _, a := range alerts {
					if a.Status() != model.AlertResolved {
						res = append(res, a)
					}
				}
			}
			if len(res) == 0 {
				return nil
			}

			err := n.Notify(ctx, res...)
			if err != nil {
				numFailedNotifications.WithLabelValues(n.name()).Inc()
			}
			numNotifications.WithLabelValues(n.name()).Inc()

			return err
		})
	}

	for _, nc := range confs {
		var (
			fo  = Fanout{}
			add = func(i int, on integration, n Notifier) { fo[fmt.Sprintf("%s/%d", on.name(), i)] = n }
		)

		for i, c := range nc.WebhookConfigs {
			n := NewWebhook(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.EmailConfigs {
			n := NewEmail(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.PagerdutyConfigs {
			n := NewPagerDuty(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.OpsGenieConfigs {
			n := NewOpsGenie(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.SlackConfigs {
			n := NewSlack(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.HipchatConfigs {
			n := NewHipchat(c, tmpl)
			add(i, n, filter(n, c))
		}
		for i, c := range nc.PushoverConfigs {
			n := NewPushover(c, tmpl)
			add(i, n, filter(n, c))
		}

		res[nc.Name] = fo
	}
	return res
}

const contentTypeJSON = "application/json"

// Webhook implements a Notifier for generic webhooks.
type Webhook struct {
	// The URL to which notifications are sent.
	URL  string
	tmpl *template.Template
}

// NewWebhook returns a new Webhook.
func NewWebhook(conf *config.WebhookConfig, t *template.Template) *Webhook {
	return &Webhook{URL: conf.URL, tmpl: t}
}

func (*Webhook) name() string { return "webhook" }

// WebhookMessage defines the JSON object send to webhook endpoints.
type WebhookMessage struct {
	*template.Data

	// The protocol version.
	Version  string `json:"version"`
	GroupKey uint64 `json:"groupKey"`
}

// Notify implements the Notifier interface.
func (w *Webhook) Notify(ctx context.Context, alerts ...*types.Alert) error {
	data := w.tmpl.Data(receiver(ctx), groupLabels(ctx), alerts...)

	groupKey, ok := GroupKey(ctx)
	if !ok {
		log.Errorf("group key missing")
	}

	msg := &WebhookMessage{
		Version:  "3",
		Data:     data,
		GroupKey: uint64(groupKey),
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
		return fmt.Errorf("unexpected status code %v from %s", resp.StatusCode, w.URL)
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
	if _, ok := c.Headers["Subject"]; !ok {
		c.Headers["Subject"] = config.DefaultEmailSubject
	}
	if _, ok := c.Headers["To"]; !ok {
		c.Headers["To"] = c.To
	}
	if _, ok := c.Headers["From"]; !ok {
		c.Headers["From"] = c.From
	}
	return &Email{conf: c, tmpl: t}
}

func (*Email) name() string { return "email" }

// auth resolves a string of authentication mechanisms.
func (n *Email) auth(mechs string) (smtp.Auth, error) {
	username := n.conf.AuthUsername

	for _, mech := range strings.Split(mechs, " ") {
		switch mech {
		case "CRAM-MD5":
			secret := string(n.conf.AuthSecret)
			if secret == "" {
				continue
			}
			return smtp.CRAMMD5Auth(username, secret), nil

		case "PLAIN":
			password := string(n.conf.AuthPassword)
			if password == "" {
				continue
			}
			identity := n.conf.AuthIdentity

			// We need to know the hostname for both auth and TLS.
			host, _, err := net.SplitHostPort(n.conf.Smarthost)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %s", err)
			}
			return smtp.PlainAuth(identity, username, password, host), nil
		}
	}
	return nil, nil
}

// Notify implements the Notifier interface.
func (n *Email) Notify(ctx context.Context, as ...*types.Alert) error {
	// Connect to the SMTP smarthost.
	c, err := smtp.Dial(n.conf.Smarthost)
	if err != nil {
		return err
	}
	defer c.Quit()

	// We need to know the hostname for both auth and TLS.
	host, _, err := net.SplitHostPort(n.conf.Smarthost)
	if err != nil {
		return fmt.Errorf("invalid address: %s", err)
	}

	if n.conf.RequireTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("require_tls: true (default), but %q does not advertise the STARTTLS extension", n.conf.Smarthost)
		}
		tlsConf := &tls.Config{ServerName: host}
		if err := c.StartTLS(tlsConf); err != nil {
			return fmt.Errorf("starttls failed: %s", err)
		}
	}

	if ok, mech := c.Extension("AUTH"); ok {
		auth, err := n.auth(mech)
		if err != nil {
			return err
		}
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("%T failed: %s", auth, err)
			}
		}
	}

	var (
		data = n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)
		tmpl = tmplText(n.tmpl, data, &err)
		from = tmpl(n.conf.From)
		to   = tmpl(n.conf.To)
	)
	if err != nil {
		return err
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

	for header, t := range n.conf.Headers {
		value, err := n.tmpl.ExecuteTextString(t, data)
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

func (*PagerDuty) name() string { return "pagerduty" }

const (
	pagerDutyEventTrigger = "trigger"
	pagerDutyEventResolve = "resolve"
)

type pagerDutyMessage struct {
	ServiceKey  string            `json:"service_key"`
	IncidentKey model.Fingerprint `json:"incident_key"`
	EventType   string            `json:"event_type"`
	Description string            `json:"description"`
	Client      string            `json:"client,omitempty"`
	ClientURL   string            `json:"client_url,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

// Notify implements the Notifier interface.
//
// http://developer.pagerduty.com/documentation/integration/events/trigger
func (n *PagerDuty) Notify(ctx context.Context, as ...*types.Alert) error {
	key, ok := GroupKey(ctx)
	if !ok {
		return fmt.Errorf("group key missing")
	}

	var err error
	var (
		alerts    = types.Alerts(as...)
		data      = n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)
		tmpl      = tmplText(n.tmpl, data, &err)
		eventType = pagerDutyEventTrigger
	)
	if alerts.Status() == model.AlertResolved {
		eventType = pagerDutyEventResolve
	}

	log.With("incident", key).With("eventType", eventType).Debugln("notifying PagerDuty")

	details := make(map[string]string, len(n.conf.Details))
	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	msg := &pagerDutyMessage{
		ServiceKey:  tmpl(string(n.conf.ServiceKey)),
		EventType:   eventType,
		IncidentKey: key,
		Description: tmpl(n.conf.Description),
		Details:     details,
	}
	if eventType == pagerDutyEventTrigger {
		msg.Client = tmpl(n.conf.Client)
		msg.ClientURL = tmpl(n.conf.ClientURL)
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

// NewSlack returns a new Slack notification handler.
func NewSlack(conf *config.SlackConfig, tmpl *template.Template) *Slack {
	return &Slack{
		conf: conf,
		tmpl: tmpl,
	}
}

func (*Slack) name() string { return "slack" }

// slackReq is the request for sending a slack notification.
type slackReq struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Attachments []slackAttachment `json:"attachments"`
}

// slackAttachment is used to display a richly-formatted message block.
type slackAttachment struct {
	Title     string `json:"title,omitempty"`
	TitleLink string `json:"title_link,omitempty"`
	Pretext   string `json:"pretext,omitempty"`
	Text      string `json:"text"`
	Fallback  string `json:"fallback"`

	Color    string   `json:"color,omitempty"`
	MrkdwnIn []string `json:"mrkdwn_in,omitempty"`
}

// slackAttachmentField is displayed in a table inside the message attachment.
type slackAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Slack) Notify(ctx context.Context, as ...*types.Alert) error {
	var err error
	var (
		data     = n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)
		tmplText = tmplText(n.tmpl, data, &err)
	)

	attachment := &slackAttachment{
		Title:     tmplText(n.conf.Title),
		TitleLink: tmplText(n.conf.TitleLink),
		Pretext:   tmplText(n.conf.Pretext),
		Text:      tmplText(n.conf.Text),
		Fallback:  tmplText(n.conf.Fallback),
		Color:     tmplText(n.conf.Color),
		MrkdwnIn:  []string{"fallback", "pretext", "text"},
	}
	req := &slackReq{
		Channel:     tmplText(n.conf.Channel),
		Username:    tmplText(n.conf.Username),
		IconEmoji:   tmplText(n.conf.IconEmoji),
		Attachments: []slackAttachment{*attachment},
	}
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, string(n.conf.APIURL), contentTypeJSON, &buf)
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

// Hipchat implements a Notifier for Hipchat notifications.
type Hipchat struct {
	conf *config.HipchatConfig
	tmpl *template.Template
}

// NewHipchat returns a new Hipchat notification handler.
func NewHipchat(conf *config.HipchatConfig, tmpl *template.Template) *Hipchat {
	return &Hipchat{
		conf: conf,
		tmpl: tmpl,
	}
}

func (*Hipchat) name() string { return "hipchat" }

type hipchatReq struct {
	From          string `json:"from"`
	Notify        bool   `json:"notify"`
	Message       string `json:"message"`
	MessageFormat string `json:"message_format"`
	Color         string `json:"color"`
}

// Notify implements the Notifier interface.
func (n *Hipchat) Notify(ctx context.Context, as ...*types.Alert) error {
	var err error
	var msg string
	var (
		data     = n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)
		tmplText = tmplText(n.tmpl, data, &err)
		tmplHTML = tmplHTML(n.tmpl, data, &err)
		url      = fmt.Sprintf("%sv2/room/%d/notification?auth_token=%s", n.conf.APIURL, n.conf.RoomID, n.conf.AuthToken)
	)

	if n.conf.MessageFormat == "html" {
		msg = tmplHTML(n.conf.Message)
	} else {
		msg = tmplText(n.conf.Message)
	}

	req := &hipchatReq{
		From:          tmplText(n.conf.From),
		Notify:        n.conf.Notify,
		Message:       msg,
		MessageFormat: n.conf.MessageFormat,
		Color:         tmplText(n.conf.Color),
	}
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, url, contentTypeJSON, &buf)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	return nil
}

// OpsGenie implements a Notifier for OpsGenie notifications.
type OpsGenie struct {
	conf *config.OpsGenieConfig
	tmpl *template.Template
}

// NewOpsGenie returns a new OpsGenie notifier.
func NewOpsGenie(c *config.OpsGenieConfig, t *template.Template) *OpsGenie {
	return &OpsGenie{conf: c, tmpl: t}
}

func (*OpsGenie) name() string { return "opsgenie" }

type opsGenieMessage struct {
	APIKey string            `json:"apiKey"`
	Alias  model.Fingerprint `json:"alias"`
}

type opsGenieCreateMessage struct {
	*opsGenieMessage `json:",inline"`

	Message string            `json:"message"`
	Details map[string]string `json:"details"`
	Source  string            `json:"source"`
	Teams   string            `json:"teams,omitempty"`
	Tags    string            `json:"tags,omitempty"`
}

type opsGenieCloseMessage struct {
	*opsGenieMessage `json:",inline"`
}

type opsGenieErrorResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// Notify implements the Notifier interface.
func (n *OpsGenie) Notify(ctx context.Context, as ...*types.Alert) error {
	key, ok := GroupKey(ctx)
	if !ok {
		return fmt.Errorf("group key missing")
	}
	data := n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)

	log.With("incident", key).Debugln("notifying OpsGenie")

	var err error
	tmpl := tmplText(n.tmpl, data, &err)

	details := make(map[string]string, len(n.conf.Details))
	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	var (
		msg    interface{}
		apiURL string

		apiMsg = opsGenieMessage{
			APIKey: string(n.conf.APIKey),
			Alias:  key,
		}
		alerts = types.Alerts(as...)
	)
	switch alerts.Status() {
	case model.AlertResolved:
		apiURL = n.conf.APIHost + "v1/json/alert/close"
		msg = &opsGenieCloseMessage{&apiMsg}
	default:
		apiURL = n.conf.APIHost + "v1/json/alert"
		msg = &opsGenieCreateMessage{
			opsGenieMessage: &apiMsg,
			Message:         tmpl(n.conf.Description),
			Details:         details,
			Source:          tmpl(n.conf.Source),
			Teams:           tmpl(n.conf.Teams),
			Tags:            tmpl(n.conf.Tags),
		}
	}
	if err != nil {
		return fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, apiURL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 && alerts.Status() == model.AlertResolved {
		body, _ := ioutil.ReadAll(resp.Body)

		var responseMessage opsGenieErrorResponse
		if err := json.Unmarshal(body, &responseMessage); err != nil {
			return fmt.Errorf("could not parse error response %q", body)
		}
		const alreadyClosedError = 5
		if responseMessage.Code == alreadyClosedError {
			return nil
		}
		return fmt.Errorf("error when closing alert: code %d, error %q",
			responseMessage.Code, responseMessage.Error)
	} else if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		log.With("incident", key).Debugf("unexpected OpsGenie response from %s (POSTed %s), %s: %s",
			apiURL, msg, resp.Status, body)
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}
	return nil
}

// Pushover implements a Notifier for Pushover notifications.
type Pushover struct {
	conf *config.PushoverConfig
	tmpl *template.Template
}

// NewPushover returns a new Pushover notifier.
func NewPushover(c *config.PushoverConfig, t *template.Template) *Pushover {
	return &Pushover{conf: c, tmpl: t}
}

func (*Pushover) name() string { return "pushover" }

// Notify implements the Notifier interface.
func (n *Pushover) Notify(ctx context.Context, as ...*types.Alert) error {
	key, ok := GroupKey(ctx)
	if !ok {
		return fmt.Errorf("group key missing")
	}
	data := n.tmpl.Data(receiver(ctx), groupLabels(ctx), as...)

	log.With("incident", key).Debugln("notifying Pushover")

	var err error
	tmpl := tmplText(n.tmpl, data, &err)

	parameters := url.Values{}
	parameters.Add("token", tmpl(string(n.conf.Token)))
	parameters.Add("user", tmpl(string(n.conf.UserKey)))
	title := tmpl(n.conf.Title)
	message := tmpl(n.conf.Message)
	parameters.Add("title", title)
	if len(title) > 512 {
		title = title[:512]
		log.With("incident", key).Debugf("Truncated title to %q due to Pushover message limit", title)
	}
	if len(title)+len(message) > 512 {
		message = message[:512-len(title)]
		log.With("incident", key).Debugf("Truncated message to %q due to Pushover message limit", message)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		// Pushover rejects empty messages.
		message = "(no details)"
	}
	parameters.Add("message", message)
	parameters.Add("url", tmpl(n.conf.URL))
	parameters.Add("priority", tmpl(n.conf.Priority))
	parameters.Add("retry", fmt.Sprintf("%d", int64(time.Duration(n.conf.Retry).Seconds())))
	parameters.Add("expire", fmt.Sprintf("%d", int64(time.Duration(n.conf.Expire).Seconds())))

	apiURL := "https://api.pushover.net/1/messages.json"
	u, err := url.Parse(apiURL)
	if err != nil {
		return err
	}
	u.RawQuery = parameters.Encode()
	log.With("incident", key).Debugf("Pushover URL = %q", u.String())

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, u.String(), "text/plain", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected status code %v (body: %s)", resp.StatusCode, string(body))
	}
	return nil
}

func tmplText(tmpl *template.Template, data *template.Data, err *error) func(string) string {
	return func(name string) (s string) {
		if *err != nil {
			return
		}
		s, *err = tmpl.ExecuteTextString(name, data)
		return s
	}
}

func tmplHTML(tmpl *template.Template, data *template.Data, err *error) func(string) string {
	return func(name string) (s string) {
		if *err != nil {
			return
		}
		s, *err = tmpl.ExecuteHTMLString(name, data)
		return s
	}
}
