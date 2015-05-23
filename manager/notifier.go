// Copyright 2013 Prometheus Team
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

package manager

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/golang/glog"
	"github.com/thorduri/pushover"

	pb "github.com/prometheus/alertmanager/config/generated"
)

const (
	contentTypeJSON = "application/json"

	notificationOpTrigger notificationOp = iota
	notificationOpResolve
)

var bodyTmpl = template.Must(template.New("message").Parse(`From: Prometheus Alertmanager <{{.From}}>
To: {{.To}}
Date: {{.Date}}
Subject: [{{ .Status }}] {{.Alert.Labels.alertname}}: {{.Alert.Summary}}

{{.Alert.Description}}

Grouping labels:
{{range $label, $value := .Alert.Labels}}
  {{$label}} = "{{$value}}"{{end}}

Payload labels:
{{range $label, $value := .Alert.Payload}}
  {{$label}} = "{{$value}}"{{end}}`))

var (
	notificationBufferSize = flag.Int("notification.buffer-size", 1000, "Size of buffer for pending notifications.")
	pagerdutyAPIURL        = flag.String("notification.pagerduty.url", "https://events.pagerduty.com/generic/2010-04-15/create_event.json", "PagerDuty API URL.")
	smtpSmartHost          = flag.String("notification.smtp.smarthost", "", "Address of the smarthost to send all email notifications to.")
	smtpSender             = flag.String("notification.smtp.sender", "alertmanager@example.org", "Sender email address to use in email notifications.")
	hipchatURL             = flag.String("notification.hipchat.url", "https://api.hipchat.com/v2", "HipChat API V2 URL.")
	flowdockURL            = flag.String("notification.flowdock.url", "https://api.flowdock.com/v1/messages/team_inbox", "Flowdock API V1 URL.")
)

type notificationOp int

// A Notifier is responsible for sending notifications for alerts according to
// a provided notification configuration.
type Notifier interface {
	// Queue a notification for asynchronous dispatching.
	QueueNotification(a *Alert, op notificationOp, configName string) error
	// Replace current notification configs. Already enqueued messages will remain
	// unaffected.
	SetNotificationConfigs([]*pb.NotificationConfig)
	// Start alert notification dispatch loop.
	Dispatch()
	// Stop the alert notification dispatch loop.
	Close()
}

// Request for sending a notification.
type notificationReq struct {
	alert              *Alert
	notificationConfig *pb.NotificationConfig
	op                 notificationOp
}

// Alert notification multiplexer and dispatcher.
type notifier struct {
	// Notifications that are queued to be sent.
	pendingNotifications chan *notificationReq

	// Mutex to protect the fields below.
	mu sync.Mutex
	// Map of notification configs by name.
	notificationConfigs map[string]*pb.NotificationConfig
}

// NewNotifier construct a new notifier.
func NewNotifier(configs []*pb.NotificationConfig) *notifier {
	notifier := &notifier{
		pendingNotifications: make(chan *notificationReq, *notificationBufferSize),
	}
	notifier.SetNotificationConfigs(configs)
	return notifier
}

func (n *notifier) SetNotificationConfigs(configs []*pb.NotificationConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.notificationConfigs = map[string]*pb.NotificationConfig{}
	for _, c := range configs {
		n.notificationConfigs[c.GetName()] = c
	}
}

func (n *notifier) QueueNotification(a *Alert, op notificationOp, configName string) error {
	n.mu.Lock()
	nc, ok := n.notificationConfigs[configName]
	n.mu.Unlock()

	if !ok {
		return fmt.Errorf("No such notification configuration %s", configName)
	}

	// We need to save a reference to the notification config in the
	// notificationReq since the config might be replaced or gone at the time the
	// message gets dispatched.
	n.pendingNotifications <- &notificationReq{
		alert:              a,
		notificationConfig: nc,
		op:                 op,
	}
	return nil
}

func (n *notifier) sendPagerDutyNotification(serviceKey string, op notificationOp, a *Alert) error {
	// http://developer.pagerduty.com/documentation/integration/events/trigger
	eventType := ""
	switch op {
	case notificationOpTrigger:
		eventType = "trigger"
	case notificationOpResolve:
		eventType = "resolve"
	}
	incidentKey := a.Fingerprint()
	buf, err := json.Marshal(map[string]interface{}{
		"service_key":  serviceKey,
		"event_type":   eventType,
		"description":  a.Description,
		"incident_key": incidentKey,
		"details": map[string]interface{}{
			"grouping_labels": a.Labels,
			"extra_labels":    a.Payload,
		},
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(
		*pagerdutyAPIURL,
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	glog.Infof("Sent PagerDuty notification: %v: HTTP %d: %s", incidentKey, resp.StatusCode, respBuf)
	// BUG: Check response for result of operation.
	return nil
}

func (n *notifier) sendHipChatNotification(op notificationOp, config *pb.HipChatConfig, a *Alert) error {
	// https://www.hipchat.com/docs/apiv2/method/send_room_notification
	incidentKey := a.Fingerprint()
	color := ""
	status := ""
	message := ""
	messageFormat := ""
	switch op {
	case notificationOpTrigger:
		color = config.GetColor()
		status = "firing"
	case notificationOpResolve:
		color = config.GetColorResolved()
		status = "resolved"
	}
	if config.GetMessageFormat() == pb.HipChatConfig_TEXT {
		message = fmt.Sprintf("%s%s %s: %s", config.GetPrefix(), a.Labels["alertname"], status, a.Summary)
		messageFormat = "text"
	} else {
		message = fmt.Sprintf("%s<b>%s %s</b>: %s (<a href='%s'>view</a>)", config.GetPrefix(), html.EscapeString(a.Labels["alertname"]), status, html.EscapeString(a.Summary), a.Payload["GeneratorURL"])
		messageFormat = "html"
	}
	buf, err := json.Marshal(map[string]interface{}{
		"color":          color,
		"message":        message,
		"notify":         config.GetNotify(),
		"message_format": messageFormat,
	})
	if err != nil {
		return err
	}

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Post(
		fmt.Sprintf("%s/room/%d/notification?auth_token=%s", *hipchatURL, config.GetRoomId(), config.GetAuthToken()),
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	glog.Infof("Sent HipChat notification: %v: HTTP %d: %s", incidentKey, resp.StatusCode, respBuf)
	// BUG: Check response for result of operation.
	return nil
}

// slackReq is the request for sending a slack notification.
type slackReq struct {
	Channel     string            `json:"channel,omitempty"`
	Attachments []slackAttachment `json:"attachments"`
}

// slackAttachment is used to display a richly-formatted message block.
type slackAttachment struct {
	Fallback  string                 `json:"fallback"`
	Pretext   string                 `json:"pretext,omitempty"`
	Title     string                 `json:"title,omitempty"`
	TitleLink string                 `json:"title_link,omitempty"`
	Text      string                 `json:"text"`
	Color     string                 `json:"color,omitempty"`
	MrkdwnIn  []string               `json:"mrkdwn_in,omitempty"`
	Fields    []slackAttachmentField `json:"fields,omitempty"`
}

// slackAttachmentField is displayed in a table inside the message attachment.
type slackAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short,omitempty"`
}

func (n *notifier) sendSlackNotification(op notificationOp, config *pb.SlackConfig, a *Alert) error {
	// https://api.slack.com/incoming-webhooks
	incidentKey := a.Fingerprint()
	color := ""
	status := ""
	switch op {
	case notificationOpTrigger:
		color = config.GetColor()
		status = "firing"
	case notificationOpResolve:
		color = config.GetColorResolved()
		status = "resolved"
	}

	statusField := &slackAttachmentField{
		Title: "Status",
		Value: status,
		Short: true,
	}

	attachment := &slackAttachment{
		Fallback:  fmt.Sprintf("*%s %s*: %s (<%s|view>)", html.EscapeString(a.Labels["alertname"]), status, html.EscapeString(a.Summary), a.Payload["GeneratorURL"]),
		Pretext:   fmt.Sprintf("*%s*", html.EscapeString(a.Labels["alertname"])),
		Title:     html.EscapeString(a.Summary),
		TitleLink: a.Payload["GeneratorURL"],
		Text:      html.EscapeString(a.Description),
		Color:     color,
		MrkdwnIn:  []string{"fallback", "pretext"},
		Fields: []slackAttachmentField{
			*statusField,
		},
	}

	req := &slackReq{
		Channel: config.GetChannel(),
		Attachments: []slackAttachment{
			*attachment,
		},
	}

	buf, err := json.Marshal(req)
	if err != nil {
		return err
	}

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Post(
		config.GetWebhookUrl(),
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBuf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	glog.Infof("Sent Slack notification: %v: HTTP %d: %s", incidentKey, resp.StatusCode, respBuf)
	// BUG: Check response for result of operation.
	return nil
}

type flowdockMessage struct {
	Source      string   `json:"source"`
	FromAddress string   `json:"from_address"`
	Subject     string   `json:"subject"`
	Content     string   `json:"content"`
	Format      string   `json:"format"`
	Link        string   `json:"link"`
	Tags        []string `json:"tags,omitempty"`
}

func (n *notifier) sendFlowdockNotification(op notificationOp, config *pb.FlowdockConfig, a *Alert) error {
	flowdockMessage := newFlowdockMessage(op, config, a)
	url := strings.TrimRight(*flowdockURL, "/") + "/" + config.GetApiToken()
	jsonMessage, err := json.Marshal(flowdockMessage)
	if err != nil {
		return err
	}
	httpResponse, err := postJSON(jsonMessage, url)
	if err != nil {
		return err
	}
	if err := processResponse(httpResponse, "Flowdock", a); err != nil {
		return err
	}
	return nil
}

func newFlowdockMessage(op notificationOp, config *pb.FlowdockConfig, a *Alert) *flowdockMessage {
	status := ""
	switch op {
	case notificationOpTrigger:
		status = "firing"
	case notificationOpResolve:
		status = "resolved"
	}

	msg := &flowdockMessage{
		Source:      "Prometheus",
		FromAddress: config.GetFromAddress(),
		Subject:     html.EscapeString(a.Summary),
		Format:      "html",
		Content:     fmt.Sprintf("*%s %s*: %s (<%s|view>)", html.EscapeString(a.Labels["alertname"]), status, html.EscapeString(a.Summary), a.Payload["GeneratorURL"]),
		Link:        a.Payload["GeneratorURL"],
		Tags:        append(config.GetTag(), status),
	}
	return msg
}

func postJSON(jsonMessage []byte, url string) (*http.Response, error) {
	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	return client.Post(url, contentTypeJSON, bytes.NewBuffer(jsonMessage))
}

func writeEmailBody(w io.Writer, from, to, status string, a *Alert) error {
	return writeEmailBodyWithTime(w, from, to, status, a, time.Now())
}

func writeEmailBodyWithTime(w io.Writer, from, to, status string, a *Alert, moment time.Time) error {
	err := bodyTmpl.Execute(w, struct {
		From   string
		To     string
		Date   string
		Alert  *Alert
		Status string
	}{
		From:   from,
		To:     to,
		Date:   moment.Format("Mon, 2 Jan 2006 15:04:05 -0700"),
		Alert:  a,
		Status: status,
	})
	if err != nil {
		return err
	}
	return nil
}

func getSMTPAuth(hasAuth bool, mechs string) (smtp.Auth, *tls.Config, error) {
	if !hasAuth {
		return nil, nil, nil
	}

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
			host, _, err := net.SplitHostPort(*smtpSmartHost)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid address: %s", err)
			}

			auth := smtp.PlainAuth(identity, username, password, host)
			cfg := &tls.Config{ServerName: host}
			return auth, cfg, nil
		}
	}
	return nil, nil, nil
}

func (n *notifier) sendEmailNotification(to string, op notificationOp, a *Alert) error {
	status := ""
	switch op {
	case notificationOpTrigger:
		status = "ALERT"
	case notificationOpResolve:
		status = "RESOLVED"
	}
	// Connect to the SMTP smarthost.
	c, err := smtp.Dial(*smtpSmartHost)
	if err != nil {
		return err
	}
	defer c.Quit()

	// Authenticate if we and the server are both configured for it.
	auth, tlsConfig, err := getSMTPAuth(c.Extension("AUTH"))
	if err != nil {
		return err
	}

	if tlsConfig != nil {
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls failed: %s", err)
		}
	}

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("%T failed: %s", auth, err)
		}
	}

	// Set the sender and recipient.
	c.Mail(*smtpSender)
	c.Rcpt(to)

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	return writeEmailBody(wc, *smtpSender, status, to, a)
}

func (n *notifier) sendPushoverNotification(token string, op notificationOp, userKey string, a *Alert) error {
	po, err := pushover.NewPushover(token, userKey)
	if err != nil {
		return err
	}

	// Validate credentials
	err = po.Validate()
	if err != nil {
		return err
	}

	// Send pushover message
	_, _, err = po.Push(&pushover.Message{
		Title:   a.Summary,
		Message: a.Description,
	})
	return err
}

func processResponse(r *http.Response, targetName string, a *Alert) error {
	spec := fmt.Sprintf("%s notification for alert %v", targetName, a.Fingerprint())
	if r == nil {
		return fmt.Errorf("No HTTP response for %s", spec)
	}
	defer r.Body.Close()
	respBuf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	glog.Infof("Sent %s. Response: HTTP %d: %s", spec, r.StatusCode, respBuf)
	return nil
}

func (n *notifier) handleNotification(a *Alert, op notificationOp, config *pb.NotificationConfig) {
	for _, pdConfig := range config.PagerdutyConfig {
		if err := n.sendPagerDutyNotification(pdConfig.GetServiceKey(), op, a); err != nil {
			glog.Errorln("Error sending PagerDuty notification:", err)
		}
	}
	for _, emailConfig := range config.EmailConfig {
		if op == notificationOpResolve && !emailConfig.GetSendResolved() {
			continue
		}
		if *smtpSmartHost == "" {
			glog.Warning("No SMTP smarthost configured, not sending email notification.")
			continue
		}
		if err := n.sendEmailNotification(emailConfig.GetEmail(), op, a); err != nil {
			glog.Errorln("Error sending email notification:", err)
		}
	}
	for _, poConfig := range config.PushoverConfig {
		if op == notificationOpResolve && !poConfig.GetSendResolved() {
			continue
		}
		if err := n.sendPushoverNotification(poConfig.GetToken(), op, poConfig.GetUserKey(), a); err != nil {
			glog.Errorln("Error sending Pushover notification:", err)
		}
	}
	for _, hcConfig := range config.HipchatConfig {
		if op == notificationOpResolve && !hcConfig.GetSendResolved() {
			continue
		}
		if err := n.sendHipChatNotification(op, hcConfig, a); err != nil {
			glog.Errorln("Error sending HipChat notification:", err)
		}
	}
	for _, scConfig := range config.SlackConfig {
		if op == notificationOpResolve && !scConfig.GetSendResolved() {
			continue
		}
		if err := n.sendSlackNotification(op, scConfig, a); err != nil {
			glog.Errorln("Error sending Slack notification:", err)
		}
	}
	for _, fdConfig := range config.FlowdockConfig {
		if op == notificationOpResolve && !fdConfig.GetSendResolved() {
			continue
		}
		if err := n.sendFlowdockNotification(op, fdConfig, a); err != nil {
			glog.Errorln("Error sending Flowdock notification:", err)
		}
	}
}

func (n *notifier) Dispatch() {
	for req := range n.pendingNotifications {
		n.handleNotification(req.alert, req.op, req.notificationConfig)
	}
}

func (n *notifier) Close() {
	close(n.pendingNotifications)
}
