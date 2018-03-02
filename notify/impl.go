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
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

type notifierConfig interface {
	SendResolved() bool
}

// A Notifier notifies about alerts under constraints of the given context.
// It returns an error if unsuccessful and a flag whether the error is
// recoverable. This information is useful for a retry logic.
type Notifier interface {
	Notify(context.Context, ...*types.Alert) (bool, error)
}

// An Integration wraps a notifier and its config to be uniquely identified by
// name and index from its origin in the configuration.
type Integration struct {
	notifier Notifier
	conf     notifierConfig
	name     string
	idx      int
}

// Notify implements the Notifier interface.
func (i *Integration) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	var res []*types.Alert

	// Resolved alerts have to be filtered only at this point, because they need
	// to end up unfiltered in the SetNotifiesStage.
	if i.conf.SendResolved() {
		res = alerts
	} else {
		for _, a := range alerts {
			if a.Status() != model.AlertResolved {
				res = append(res, a)
			}
		}
	}
	if len(res) == 0 {
		return false, nil
	}

	return i.notifier.Notify(ctx, res...)
}

// BuildReceiverIntegrations builds a list of integration notifiers off of a
// receivers config.
func BuildReceiverIntegrations(nc *config.Receiver, tmpl *template.Template, logger log.Logger) []Integration {
	var (
		integrations []Integration
		add          = func(name string, i int, n Notifier, nc notifierConfig) {
			integrations = append(integrations, Integration{
				notifier: n,
				conf:     nc,
				name:     name,
				idx:      i,
			})
		}
	)

	for i, c := range nc.WebhookConfigs {
		n := NewWebhook(c, tmpl, logger)
		add("webhook", i, n, c)
	}
	for i, c := range nc.EmailConfigs {
		n := NewEmail(c, tmpl, logger)
		add("email", i, n, c)
	}
	for i, c := range nc.PagerdutyConfigs {
		n := NewPagerDuty(c, tmpl, logger)
		add("pagerduty", i, n, c)
	}
	for i, c := range nc.OpsGenieConfigs {
		n := NewOpsGenie(c, tmpl, logger)
		add("opsgenie", i, n, c)
	}
	for i, c := range nc.WechatConfigs {
		n := NewWechat(c, tmpl, logger)
		add("wechat", i, n, c)
	}
	for i, c := range nc.SlackConfigs {
		n := NewSlack(c, tmpl, logger)
		add("slack", i, n, c)
	}
	for i, c := range nc.HipchatConfigs {
		n := NewHipchat(c, tmpl, logger)
		add("hipchat", i, n, c)
	}
	for i, c := range nc.VictorOpsConfigs {
		n := NewVictorOps(c, tmpl, logger)
		add("victorops", i, n, c)
	}
	for i, c := range nc.PushoverConfigs {
		n := NewPushover(c, tmpl, logger)
		add("pushover", i, n, c)
	}
	return integrations
}

const contentTypeJSON = "application/json"

var userAgentHeader = fmt.Sprintf("Alertmanager/%s", version.Version)

// Webhook implements a Notifier for generic webhooks.
type Webhook struct {
	conf   *config.WebhookConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewWebhook returns a new Webhook.
func NewWebhook(conf *config.WebhookConfig, t *template.Template, l log.Logger) *Webhook {
	return &Webhook{conf: conf, tmpl: t, logger: l}
}

// WebhookMessage defines the JSON object send to webhook endpoints.
type WebhookMessage struct {
	*template.Data

	// The protocol version.
	Version  string `json:"version"`
	GroupKey string `json:"groupKey"`
}

// Notify implements the Notifier interface.
func (w *Webhook) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	data := w.tmpl.Data(receiverName(ctx, w.logger), groupLabels(ctx, w.logger), alerts...)

	groupKey, ok := GroupKey(ctx)
	if !ok {
		level.Error(w.logger).Log("msg", "group key missing")
	}

	msg := &WebhookMessage{
		Version:  "4",
		Data:     data,
		GroupKey: groupKey,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", w.conf.URL, &buf)
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("User-Agent", userAgentHeader)

	c, err := commoncfg.NewHTTPClientFromConfig(w.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Do(ctx, c, req)
	if err != nil {
		return true, err
	}
	resp.Body.Close()

	return w.retry(resp.StatusCode)
}

func (w *Webhook) retry(statusCode int) (bool, error) {
	// Webhooks are assumed to respond with 2xx response codes on a successful
	// request and 5xx response codes are assumed to be recoverable.
	if statusCode/100 != 2 {
		return (statusCode/100 == 5), fmt.Errorf("unexpected status code %v from %s", statusCode, w.conf.URL)
	}

	return false, nil
}

// Email implements a Notifier for email notifications.
type Email struct {
	conf   *config.EmailConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewEmail returns a new Email notifier.
func NewEmail(c *config.EmailConfig, t *template.Template, l log.Logger) *Email {
	if _, ok := c.Headers["Subject"]; !ok {
		c.Headers["Subject"] = config.DefaultEmailSubject
	}
	if _, ok := c.Headers["To"]; !ok {
		c.Headers["To"] = c.To
	}
	if _, ok := c.Headers["From"]; !ok {
		c.Headers["From"] = c.From
	}
	return &Email{conf: c, tmpl: t, logger: l}
}

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
		case "LOGIN":
			password := string(n.conf.AuthPassword)
			if password == "" {
				continue
			}
			return LoginAuth(username, password), nil
		}
	}
	return nil, nil
}

// Notify implements the Notifier interface.
func (n *Email) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	// We need to know the hostname for both auth and TLS.
	var c *smtp.Client
	host, port, err := net.SplitHostPort(n.conf.Smarthost)
	if err != nil {
		return false, fmt.Errorf("invalid address: %s", err)
	}

	if port == "465" {
		conn, err := tls.Dial("tcp", n.conf.Smarthost, &tls.Config{ServerName: host})
		if err != nil {
			return true, err
		}
		c, err = smtp.NewClient(conn, n.conf.Smarthost)
		if err != nil {
			return true, err
		}

	} else {
		// Connect to the SMTP smarthost.
		c, err = smtp.Dial(n.conf.Smarthost)
		if err != nil {
			return true, err
		}
	}
	defer c.Quit()

	if n.conf.Hello != "" {
		err := c.Hello(n.conf.Hello)
		if err != nil {
			return true, err
		}
	}

	// Global Config guarantees RequireTLS is not nil
	if *n.conf.RequireTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return true, fmt.Errorf("require_tls: true (default), but %q does not advertise the STARTTLS extension", n.conf.Smarthost)
		}
		tlsConf := &tls.Config{ServerName: host}
		if err := c.StartTLS(tlsConf); err != nil {
			return true, fmt.Errorf("starttls failed: %s", err)
		}
	}

	if ok, mech := c.Extension("AUTH"); ok {
		auth, err := n.auth(mech)
		if err != nil {
			return true, err
		}
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return true, fmt.Errorf("%T failed: %s", auth, err)
			}
		}
	}

	var (
		data = n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)
		tmpl = tmplText(n.tmpl, data, &err)
		from = tmpl(n.conf.From)
		to   = tmpl(n.conf.To)
	)
	if err != nil {
		return false, err
	}

	addrs, err := mail.ParseAddressList(from)
	if err != nil {
		return false, fmt.Errorf("parsing from addresses: %s", err)
	}
	if len(addrs) != 1 {
		return false, fmt.Errorf("must be exactly one from address")
	}
	if err := c.Mail(addrs[0].Address); err != nil {
		return true, fmt.Errorf("sending mail from: %s", err)
	}
	addrs, err = mail.ParseAddressList(to)
	if err != nil {
		return false, fmt.Errorf("parsing to addresses: %s", err)
	}
	for _, addr := range addrs {
		if err := c.Rcpt(addr.Address); err != nil {
			return true, fmt.Errorf("sending rcpt to: %s", err)
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return true, err
	}
	defer wc.Close()

	for header, t := range n.conf.Headers {
		value, err := n.tmpl.ExecuteTextString(t, data)
		if err != nil {
			return false, fmt.Errorf("executing %q header template: %s", header, err)
		}
		fmt.Fprintf(wc, "%s: %s\r\n", header, mime.QEncoding.Encode("utf-8", value))
	}

	buffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(buffer)

	fmt.Fprintf(wc, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(wc, "Content-Type: multipart/alternative;  boundary=%s\r\n", multipartWriter.Boundary())
	fmt.Fprintf(wc, "MIME-Version: 1.0\r\n")

	// TODO: Add some useful headers here, such as URL of the alertmanager
	// and active/resolved.
	fmt.Fprintf(wc, "\r\n")

	if len(n.conf.Text) > 0 {
		// Text template
		w, err := multipartWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}})
		if err != nil {
			return false, fmt.Errorf("creating part for text template: %s", err)
		}
		body, err := n.tmpl.ExecuteTextString(n.conf.Text, data)
		if err != nil {
			return false, fmt.Errorf("executing email text template: %s", err)
		}
		_, err = w.Write([]byte(body))
		if err != nil {
			return true, err
		}
	}

	if len(n.conf.HTML) > 0 {
		// Html template
		// Preferred alternative placed last per section 5.1.4 of RFC 2046
		// https://www.ietf.org/rfc/rfc2046.txt
		w, err := multipartWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}})
		if err != nil {
			return false, fmt.Errorf("creating part for html template: %s", err)
		}
		body, err := n.tmpl.ExecuteHTMLString(n.conf.HTML, data)
		if err != nil {
			return false, fmt.Errorf("executing email html template: %s", err)
		}
		_, err = w.Write([]byte(body))
		if err != nil {
			return true, err
		}
	}

	multipartWriter.Close()
	wc.Write(buffer.Bytes())

	return false, nil
}

// PagerDuty implements a Notifier for PagerDuty notifications.
type PagerDuty struct {
	conf   *config.PagerdutyConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewPagerDuty returns a new PagerDuty notifier.
func NewPagerDuty(c *config.PagerdutyConfig, t *template.Template, l log.Logger) *PagerDuty {
	return &PagerDuty{conf: c, tmpl: t, logger: l}
}

const (
	pagerDutyEventTrigger = "trigger"
	pagerDutyEventResolve = "resolve"
)

type pagerDutyMessage struct {
	RoutingKey  string            `json:"routing_key,omitempty"`
	ServiceKey  string            `json:"service_key,omitempty"`
	DedupKey    string            `json:"dedup_key,omitempty"`
	IncidentKey string            `json:"incident_key,omitempty"`
	EventType   string            `json:"event_type,omitempty"`
	Description string            `json:"description,omitempty"`
	EventAction string            `json:"event_action"`
	Payload     *pagerDutyPayload `json:"payload"`
	Client      string            `json:"client,omitempty"`
	ClientURL   string            `json:"client_url,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

type pagerDutyPayload struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Timestamp     string            `json:"timestamp,omitempty"`
	Class         string            `json:"class,omitempty"`
	Component     string            `json:"component,omitempty"`
	Group         string            `json:"group,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

func (n *PagerDuty) notifyV1(ctx context.Context, c *http.Client, eventType, key string, tmpl func(string) string, details map[string]string, as ...*types.Alert) (bool, error) {

	msg := &pagerDutyMessage{
		ServiceKey:  tmpl(string(n.conf.ServiceKey)),
		EventType:   eventType,
		IncidentKey: hashKey(key),
		Description: tmpl(n.conf.Description),
		Details:     details,
	}

	n.conf.URL = "https://events.pagerduty.com/generic/2010-04-15/create_event.json"

	if eventType == pagerDutyEventTrigger {
		msg.Client = tmpl(n.conf.Client)
		msg.ClientURL = tmpl(n.conf.ClientURL)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, n.conf.URL, contentTypeJSON, &buf)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	return n.retryV1(resp.StatusCode)
}

func (n *PagerDuty) notifyV2(ctx context.Context, c *http.Client, eventType, key string, tmpl func(string) string, details map[string]string, as ...*types.Alert) (bool, error) {
	if n.conf.Severity == "" {
		n.conf.Severity = "error"
	}

	var payload *pagerDutyPayload
	if eventType == pagerDutyEventTrigger {
		payload = &pagerDutyPayload{
			Summary:       tmpl(n.conf.Description),
			Source:        tmpl(n.conf.Client),
			Severity:      tmpl(n.conf.Severity),
			CustomDetails: details,
			Class:         tmpl(n.conf.Class),
			Component:     tmpl(n.conf.Component),
			Group:         tmpl(n.conf.Group),
		}
	}

	msg := &pagerDutyMessage{
		RoutingKey:  tmpl(string(n.conf.RoutingKey)),
		EventAction: eventType,
		DedupKey:    hashKey(key),
		Payload:     payload,
	}

	if eventType == pagerDutyEventTrigger {
		msg.Client = tmpl(n.conf.Client)
		msg.ClientURL = tmpl(n.conf.ClientURL)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, n.conf.URL, contentTypeJSON, &buf)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	return n.retryV2(resp.StatusCode)
}

// Notify implements the Notifier interface.
//
// https://v2.developer.pagerduty.com/docs/events-api-v2
func (n *PagerDuty) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, ok := GroupKey(ctx)
	if !ok {
		return false, fmt.Errorf("group key missing")
	}

	var err error
	var (
		alerts    = types.Alerts(as...)
		data      = n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)
		tmpl      = tmplText(n.tmpl, data, &err)
		eventType = pagerDutyEventTrigger
	)
	if alerts.Status() == model.AlertResolved {
		eventType = pagerDutyEventResolve
	}

	level.Debug(n.logger).Log("msg", "Notifying PagerDuty", "incident", key, "eventType", eventType)

	details := make(map[string]string, len(n.conf.Details))
	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	if err != nil {
		return false, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	if n.conf.ServiceKey != "" {
		return n.notifyV1(ctx, c, eventType, key, tmpl, details, as...)
	}
	return n.notifyV2(ctx, c, eventType, key, tmpl, details, as...)
}

func (n *PagerDuty) retryV1(statusCode int) (bool, error) {
	// Retrying can solve the issue on 403 (rate limiting) and 5xx response codes.
	// 2xx response codes indicate a successful request.
	// https://v2.developer.pagerduty.com/docs/trigger-events
	if statusCode/100 != 2 {
		return (statusCode == 403 || statusCode/100 == 5), fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

func (n *PagerDuty) retryV2(statusCode int) (bool, error) {
	// Retrying can solve the issue on 429 (rate limiting) and 5xx response codes.
	// 2xx response codes indicate a successful request.
	// https://v2.developer.pagerduty.com/docs/events-api-v2#api-response-codes--retry-logic
	if statusCode/100 != 2 {
		return (statusCode == 429 || statusCode/100 == 5), fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

// Slack implements a Notifier for Slack notifications.
type Slack struct {
	conf   *config.SlackConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewSlack returns a new Slack notification handler.
func NewSlack(c *config.SlackConfig, t *template.Template, l log.Logger) *Slack {
	return &Slack{
		conf:   c,
		tmpl:   t,
		logger: l,
	}
}

// slackReq is the request for sending a slack notification.
type slackReq struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	IconURL     string            `json:"icon_url,omitempty"`
	LinkNames   bool              `json:"link_names,omitempty"`
	Attachments []slackAttachment `json:"attachments"`
}

// slackAttachment is used to display a richly-formatted message block.
type slackAttachment struct {
	Title     string                 `json:"title,omitempty"`
	TitleLink string                 `json:"title_link,omitempty"`
	Pretext   string                 `json:"pretext,omitempty"`
	Text      string                 `json:"text"`
	Fallback  string                 `json:"fallback"`
	Fields    []slackAttachmentField `json:"fields"`
	Footer    string                 `json:"footer"`

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
func (n *Slack) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)
		tmplText = tmplText(n.tmpl, data, &err)
	)

	attachment := &slackAttachment{
		Title:     tmplText(n.conf.Title),
		TitleLink: tmplText(n.conf.TitleLink),
		Pretext:   tmplText(n.conf.Pretext),
		Text:      tmplText(n.conf.Text),
		Fallback:  tmplText(n.conf.Fallback),
		Footer:    tmplText(n.conf.Footer),
		Color:     tmplText(n.conf.Color),
		MrkdwnIn:  []string{"fallback", "pretext", "text"},
	}

	var numFields = len(n.conf.Fields)
	if numFields > 0 {
		var fields = make([]slackAttachmentField, numFields)
		for k, v := range n.conf.Fields {
			fields[k] = slackAttachmentField{
				tmplText(v["title"]),
				tmplText(v["value"]),
				n.conf.ShortFields,
			}
		}
		attachment.Fields = fields
	}

	req := &slackReq{
		Channel:     tmplText(n.conf.Channel),
		Username:    tmplText(n.conf.Username),
		IconEmoji:   tmplText(n.conf.IconEmoji),
		IconURL:     tmplText(n.conf.IconURL),
		LinkNames:   n.conf.LinkNames,
		Attachments: []slackAttachment{*attachment},
	}
	if err != nil {
		return false, err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, string(n.conf.APIURL), contentTypeJSON, &buf)
	if err != nil {
		return true, err
	}
	resp.Body.Close()

	return n.retry(resp.StatusCode)
}

func (n *Slack) retry(statusCode int) (bool, error) {
	// Only 5xx response codes are recoverable and 2xx codes are successful.
	// https://api.slack.com/incoming-webhooks#handling_errors
	// https://api.slack.com/changelog/2016-05-17-changes-to-errors-for-incoming-webhooks
	if statusCode/100 != 2 {
		return (statusCode/100 == 5), fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

// Hipchat implements a Notifier for Hipchat notifications.
type Hipchat struct {
	conf   *config.HipchatConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewHipchat returns a new Hipchat notification handler.
func NewHipchat(c *config.HipchatConfig, t *template.Template, l log.Logger) *Hipchat {
	return &Hipchat{
		conf:   c,
		tmpl:   t,
		logger: l,
	}
}

type hipchatReq struct {
	From          string `json:"from"`
	Notify        bool   `json:"notify"`
	Message       string `json:"message"`
	MessageFormat string `json:"message_format"`
	Color         string `json:"color"`
}

// Notify implements the Notifier interface.
func (n *Hipchat) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var msg string
	var (
		data     = n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)
		tmplText = tmplText(n.tmpl, data, &err)
		tmplHTML = tmplHTML(n.tmpl, data, &err)
		url      = fmt.Sprintf("%sv2/room/%s/notification?auth_token=%s", n.conf.APIURL, n.conf.RoomID, n.conf.AuthToken)
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
		return false, err
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return false, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, url, contentTypeJSON, &buf)
	if err != nil {
		return true, err
	}

	defer resp.Body.Close()

	return n.retry(resp.StatusCode)
}

func (n *Hipchat) retry(statusCode int) (bool, error) {
	// Response codes 429 (rate limiting) and 5xx can potentially recover. 2xx
	// responce codes indicate successful requests.
	// https://developer.atlassian.com/hipchat/guide/hipchat-rest-api/api-response-codes
	if statusCode/100 != 2 {
		return (statusCode == 429 || statusCode/100 == 5), fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

// Wechat implements a Notfier for wechat notifications
type Wechat struct {
	conf   *config.WechatConfig
	tmpl   *template.Template
	logger log.Logger

	accessToken   string
	accessTokenAt time.Time
}

// Wechat AccessToken with corpid and corpsecret.
type WechatToken struct {
	AccessToken string `json:"access_token"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `json:"-"`
}

type weChatMessage struct {
	Text    weChatMessageContent `yaml:"text,omitempty" json:"text,omitempty"`
	ToUser  string               `yaml:"touser,omitempty" json:"touser,omitempty"`
	ToParty string               `yaml:"toparty,omitempty" json:"toparty,omitempty"`
	Totag   string               `yaml:"totag,omitempty" json:"totag,omitempty"`
	AgentID string               `yaml:"agentid,omitempty" json:"agentid,omitempty"`
	Safe    string               `yaml:"safe,omitempty" json:"safe,omitempty"`
	Type    string               `yaml:"msgtype,omitempty" json:"msgtype,omitempty"`
}

type weChatMessageContent struct {
	Content string `json:"content"`
}

type weChatResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// NewWechat returns a new Wechat notifier.
func NewWechat(c *config.WechatConfig, t *template.Template, l log.Logger) *Wechat {
	return &Wechat{conf: c, tmpl: t, logger: l}
}

// Notify implements the Notifier interface.
func (n *Wechat) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, ok := GroupKey(ctx)
	if !ok {
		return false, fmt.Errorf("group key missing")
	}

	level.Debug(n.logger).Log("msg", "Notifying Wechat", "incident", key)
	data := n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)

	var err error
	tmpl := tmplText(n.tmpl, data, &err)
	if err != nil {
		return false, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	// Refresh AccessToken over 2 hours
	if n.accessToken == "" || time.Now().Sub(n.accessTokenAt) > 2*time.Hour {
		parameters := url.Values{}
		parameters.Add("corpsecret", tmpl(string(n.conf.APISecret)))
		parameters.Add("corpid", tmpl(string(n.conf.CorpID)))

		apiURL := n.conf.APIURL + "gettoken"

		u, err := url.Parse(apiURL)
		if err != nil {
			return false, err
		}

		u.RawQuery = parameters.Encode()

		level.Debug(n.logger).Log("msg", "Sending Wechat message", "incident", key, "url", u.String())

		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return true, err
		}

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := c.Do(req.WithContext(ctx))
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()

		var wechatToken WechatToken
		if err := json.NewDecoder(resp.Body).Decode(&wechatToken); err != nil {
			return false, err
		}

		if wechatToken.AccessToken == "" {
			return false, fmt.Errorf("invalid APISecret for CorpID: %s", n.conf.CorpID)
		}

		// Cache accessToken
		n.accessToken = wechatToken.AccessToken
		n.accessTokenAt = time.Now()
	}

	msg := &weChatMessage{
		Text: weChatMessageContent{
			Content: tmpl(n.conf.Message),
		},
		ToUser:  n.conf.ToUser,
		ToParty: n.conf.ToParty,
		Totag:   n.conf.ToTag,
		AgentID: n.conf.AgentID,
		Type:    "text",
		Safe:    "0",
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	postMessageURL := n.conf.APIURL + "message/send?access_token=" + n.accessToken

	req, err := http.NewRequest(http.MethodPost, postMessageURL, &buf)
	if err != nil {
		return true, err
	}

	resp, err := c.Do(req.WithContext(ctx))
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	level.Debug(n.logger).Log("msg", "response: "+string(body), "incident", key)

	if resp.StatusCode != 200 {
		return true, fmt.Errorf("unexpected status code %v", resp.StatusCode)
	} else {
		var weResp weChatResponse
		if err := json.Unmarshal(body, &weResp); err != nil {
			return true, err
		}

		// https://work.weixin.qq.com/api/doc#10649
		if weResp.Code == 0 {
			return false, nil
		}

		// AccessToken is expired
		if weResp.Code == 42001 {
			n.accessToken = ""
			return true, errors.New(weResp.Error)
		}

		return false, errors.New(weResp.Error)
	}
}

// OpsGenie implements a Notifier for OpsGenie notifications.
type OpsGenie struct {
	conf   *config.OpsGenieConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewOpsGenie returns a new OpsGenie notifier.
func NewOpsGenie(c *config.OpsGenieConfig, t *template.Template, l log.Logger) *OpsGenie {
	return &OpsGenie{conf: c, tmpl: t, logger: l}
}

type opsGenieCreateMessage struct {
	Alias       string              `json:"alias"`
	Message     string              `json:"message"`
	Description string              `json:"description,omitempty"`
	Details     map[string]string   `json:"details"`
	Source      string              `json:"source"`
	Teams       []map[string]string `json:"teams,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Note        string              `json:"note,omitempty"`
	Priority    string              `json:"priority,omitempty"`
}

type opsGenieCloseMessage struct {
	Source string `json:"source"`
}

// Notify implements the Notifier interface.
func (n *OpsGenie) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	req, retry, err := n.createRequest(ctx, as...)
	if err != nil {
		return retry, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Do(ctx, c, req)

	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	return n.retry(resp.StatusCode)
}

// Like Split but filter out empty strings.
func safeSplit(s string, sep string) []string {
	a := strings.Split(strings.TrimSpace(s), sep)
	b := a[:0]
	for _, x := range a {
		if x != "" {
			b = append(b, x)
		}
	}
	return b
}

// Create requests for a list of alerts.
func (n *OpsGenie) createRequest(ctx context.Context, as ...*types.Alert) (*http.Request, bool, error) {
	key, ok := GroupKey(ctx)
	if !ok {
		return nil, false, fmt.Errorf("group key missing")
	}
	data := n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)

	level.Debug(n.logger).Log("msg", "Notifying OpsGenie", "incident", key)

	var err error
	tmpl := tmplText(n.tmpl, data, &err)

	details := make(map[string]string, len(n.conf.Details))
	for k, v := range n.conf.Details {
		details[k] = tmpl(v)
	}

	var (
		msg    interface{}
		apiURL string
		alias  = hashKey(key)
		alerts = types.Alerts(as...)
	)
	switch alerts.Status() {
	case model.AlertResolved:
		apiURL = fmt.Sprintf("%sv2/alerts/%s/close?identifierType=alias", n.conf.APIURL, alias)
		msg = &opsGenieCloseMessage{Source: tmpl(n.conf.Source)}
	default:
		message := tmpl(n.conf.Message)
		if len(message) > 130 {
			message = message[:127] + "..."
			level.Debug(n.logger).Log("msg", "Truncated message to %q due to OpsGenie message limit", "truncated_message", message, "incident", key)
		}

		apiURL = n.conf.APIURL + "v2/alerts"
		var teams []map[string]string
		for _, t := range safeSplit(string(tmpl(n.conf.Teams)), ",") {
			teams = append(teams, map[string]string{"name": t})
		}
		tags := safeSplit(string(tmpl(n.conf.Tags)), ",")

		msg = &opsGenieCreateMessage{
			Alias:       alias,
			Message:     message,
			Description: tmpl(n.conf.Description),
			Details:     details,
			Source:      tmpl(n.conf.Source),
			Teams:       teams,
			Tags:        tags,
			Note:        tmpl(n.conf.Note),
			Priority:    tmpl(n.conf.Priority),
		}
	}
	if err != nil {
		return nil, false, fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return nil, true, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Authorization", fmt.Sprintf("GenieKey %s", n.conf.APIKey))
	return req, true, nil
}

func (n *OpsGenie) retry(statusCode int) (bool, error) {
	// https://docs.opsgenie.com/docs/response#section-response-codes
	// Response codes 429 (rate limiting) and 5xx are potentially recoverable
	if statusCode/100 == 5 || statusCode == 429 {
		return true, fmt.Errorf("unexpected status code %v", statusCode)
	} else if statusCode/100 != 2 {
		return false, fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

// VictorOps implements a Notifier for VictorOps notifications.
type VictorOps struct {
	conf   *config.VictorOpsConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewVictorOps returns a new VictorOps notifier.
func NewVictorOps(c *config.VictorOpsConfig, t *template.Template, l log.Logger) *VictorOps {
	return &VictorOps{
		conf:   c,
		tmpl:   t,
		logger: l,
	}
}

const (
	victorOpsEventTrigger = "CRITICAL"
	victorOpsEventResolve = "RECOVERY"
)

type victorOpsMessage struct {
	MessageType       string `json:"message_type"`
	EntityID          string `json:"entity_id"`
	EntityDisplayName string `json:"entity_display_name"`
	StateMessage      string `json:"state_message"`
	MonitoringTool    string `json:"monitoring_tool"`
}

// Notify implements the Notifier interface.
func (n *VictorOps) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	victorOpsAllowedEvents := map[string]bool{
		"INFO":     true,
		"WARNING":  true,
		"CRITICAL": true,
	}

	key, ok := GroupKey(ctx)
	if !ok {
		return false, fmt.Errorf("group key missing")
	}

	var err error
	var (
		alerts       = types.Alerts(as...)
		data         = n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)
		tmpl         = tmplText(n.tmpl, data, &err)
		apiURL       = fmt.Sprintf("%s%s/%s", n.conf.APIURL, n.conf.APIKey, tmpl(n.conf.RoutingKey))
		messageType  = tmpl(n.conf.MessageType)
		stateMessage = tmpl(n.conf.StateMessage)
	)

	if alerts.Status() == model.AlertFiring && !victorOpsAllowedEvents[messageType] {
		messageType = victorOpsEventTrigger
	}

	if alerts.Status() == model.AlertResolved {
		messageType = victorOpsEventResolve
	}

	if len(stateMessage) > 20480 {
		stateMessage = stateMessage[0:20475] + "\n..."
		level.Debug(n.logger).Log("msg", "Truncated stateMessage due to VictorOps stateMessage limit", "truncated_state_message", stateMessage, "incident", key)
	}

	msg := &victorOpsMessage{
		MessageType:       messageType,
		EntityID:          hashKey(key),
		EntityDisplayName: tmpl(n.conf.EntityDisplayName),
		StateMessage:      stateMessage,
		MonitoringTool:    tmpl(n.conf.MonitoringTool),
	}

	if err != nil {
		return false, fmt.Errorf("templating error: %s", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, apiURL, contentTypeJSON, &buf)
	if err != nil {
		return true, err
	}

	defer resp.Body.Close()

	return n.retry(resp.StatusCode)
}

func (n *VictorOps) retry(statusCode int) (bool, error) {
	// Missing documentation therefore assuming only 5xx response codes are
	// recoverable.
	if statusCode/100 == 5 {
		return true, fmt.Errorf("unexpected status code %v", statusCode)
	} else if statusCode/100 != 2 {
		return false, fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
}

// Pushover implements a Notifier for Pushover notifications.
type Pushover struct {
	conf   *config.PushoverConfig
	tmpl   *template.Template
	logger log.Logger
}

// NewPushover returns a new Pushover notifier.
func NewPushover(c *config.PushoverConfig, t *template.Template, l log.Logger) *Pushover {
	return &Pushover{conf: c, tmpl: t, logger: l}
}

// Notify implements the Notifier interface.
func (n *Pushover) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	key, ok := GroupKey(ctx)
	if !ok {
		return false, fmt.Errorf("group key missing")
	}
	data := n.tmpl.Data(receiverName(ctx, n.logger), groupLabels(ctx, n.logger), as...)

	level.Debug(n.logger).Log("msg", "Notifying Pushover", "incident", key)

	var err error
	tmpl := tmplText(n.tmpl, data, &err)

	parameters := url.Values{}
	parameters.Add("token", tmpl(string(n.conf.Token)))
	parameters.Add("user", tmpl(string(n.conf.UserKey)))

	title := tmpl(n.conf.Title)
	if len(title) > 250 {
		title = title[:247] + "..."
		level.Debug(n.logger).Log("msg", "Truncated title due to Pushover title limit", "truncated_title", title, "incident", key)
	}
	parameters.Add("title", title)

	message := tmpl(n.conf.Message)
	if len(message) > 1024 {
		message = message[:1021] + "..."
		level.Debug(n.logger).Log("msg", "Truncated message due to Pushover message limit", "truncated_message", message, "incident", key)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		// Pushover rejects empty messages.
		message = "(no details)"
	}
	parameters.Add("message", message)

	supplementaryURL := tmpl(n.conf.URL)
	if len(supplementaryURL) > 512 {
		supplementaryURL = supplementaryURL[:509] + "..."
		level.Debug(n.logger).Log("msg", "Truncated URL due to Pushover url limit", "truncated_url", supplementaryURL, "incident", key)
	}
	parameters.Add("url", supplementaryURL)

	parameters.Add("priority", tmpl(n.conf.Priority))
	parameters.Add("retry", fmt.Sprintf("%d", int64(time.Duration(n.conf.Retry).Seconds())))
	parameters.Add("expire", fmt.Sprintf("%d", int64(time.Duration(n.conf.Expire).Seconds())))
	if err != nil {
		return false, err
	}

	apiURL := "https://api.pushover.net/1/messages.json"
	u, err := url.Parse(apiURL)
	if err != nil {
		return false, err
	}
	u.RawQuery = parameters.Encode()
	level.Debug(n.logger).Log("msg", "Sending Pushover message", "incident", key, "url", u.String())

	c, err := commoncfg.NewHTTPClientFromConfig(n.conf.HTTPConfig)
	if err != nil {
		return false, err
	}

	resp, err := ctxhttp.Post(ctx, c, u.String(), "text/plain", nil)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	return n.retry(resp.StatusCode)
}

func (n *Pushover) retry(statusCode int) (bool, error) {
	// Only documented behaviour is that 2xx response codes are successful and
	// 4xx are unsuccessful, therefore assuming only 5xx are recoverable.
	// https://pushover.net/api#response
	if statusCode/100 == 5 {
		return true, fmt.Errorf("unexpected status code %v", statusCode)
	} else if statusCode/100 != 2 {
		return false, fmt.Errorf("unexpected status code %v", statusCode)
	}

	return false, nil
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

type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

// Used for AUTH LOGIN. (Maybe password should be encrypted)
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch strings.ToLower(string(fromServer)) {
		case "username:":
			return []byte(a.username), nil
		case "password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unexpected server challenge")
		}
	}
	return nil, nil
}

// hashKey returns the sha256 for a group key as integrations may have
// maximum length requirements on deduplication keys.
func hashKey(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}
