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

const contentTypeJson = "application/json"

var bodyTmpl = template.Must(template.New("message").Parse(`From: Prometheus Alertmanager <{{.From}}>
To: {{.To}}
Date: {{.Date}}
Subject: [ALERT] {{.Alert.Labels.alertname}}: {{.Alert.Summary}}

{{.Alert.Description}}

Grouping labels:
{{range $label, $value := .Alert.Labels}}
  {{$label}} = "{{$value}}"{{end}}

Payload labels:
{{range $label, $value := .Alert.Payload}}
  {{$label}} = "{{$value}}"{{end}}`))

var (
	notificationBufferSize = flag.Int("notification.buffer-size", 1000, "Size of buffer for pending notifications.")
	pagerdutyApiUrl        = flag.String("notification.pagerduty.url", "https://events.pagerduty.com/generic/2010-04-15/create_event.json", "PagerDuty API URL.")
	smtpSmartHost          = flag.String("notification.smtp.smarthost", "", "Address of the smarthost to send all email notifications to.")
	smtpSender             = flag.String("notification.smtp.sender", "alertmanager@example.org", "Sender email address to use in email notifications.")
	hipchatUrl             = flag.String("notification.hipchat.url", "https://api.hipchat.com/v2", "HipChat API V2 URL.")
)

// A Notifier is responsible for sending notifications for alerts according to
// a provided notification configuration.
type Notifier interface {
	// Queue a notification for asynchronous dispatching.
	QueueNotification(a *Alert, configName string) error
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

// Construct a new notifier.
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

func (n *notifier) QueueNotification(a *Alert, configName string) error {
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
	}
	return nil
}

func (n *notifier) sendPagerDutyNotification(serviceKey string, a *Alert) error {
	// http://developer.pagerduty.com/documentation/integration/events/trigger
	incidentKey := a.Fingerprint()
	buf, err := json.Marshal(map[string]interface{}{
		"service_key":  serviceKey,
		"event_type":   "trigger",
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
		*pagerdutyApiUrl,
		contentTypeJson,
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

func (n *notifier) sendHipChatNotification(authToken string, roomId int32, color string, notify bool, a *Alert) error {
	// https://www.hipchat.com/docs/apiv2/method/send_room_notification
	incidentKey := a.Fingerprint()
	buf, err := json.Marshal(map[string]interface{}{
		"color":          color,
		"message":        fmt.Sprintf("%s: %s <a href='%s'>view</a>", html.EscapeString(a.Labels["alertname"]), html.EscapeString(a.Summary), a.Payload["GeneratorURL"]),
		"notify":         notify,
		"message_format": "html",
	})
	if err != nil {
		return err
	}

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Post(
		*hipchatUrl+fmt.Sprintf("/room/%d/notification?auth_token=%s", roomId, authToken),
		contentTypeJson,
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

func writeEmailBody(w io.Writer, from string, to string, a *Alert) error {
	return writeEmailBodyWithTime(w, from, to, a, time.Now())
}

func writeEmailBodyWithTime(w io.Writer, from string, to string, a *Alert, moment time.Time) error {
	err := bodyTmpl.Execute(w, struct {
		From  string
		To    string
		Date  string
		Alert *Alert
	}{
		From:  from,
		To:    to,
		Date:  moment.Format("Mon, 2 Jan 2006 15:04:05 -0700"),
		Alert: a,
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

func (n *notifier) sendEmailNotification(to string, a *Alert) error {
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

	return writeEmailBody(wc, *smtpSender, to, a)
}

func (n *notifier) sendPushoverNotification(token, userKey string, a *Alert) error {
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

func (n *notifier) handleNotification(a *Alert, config *pb.NotificationConfig) {
	for _, pdConfig := range config.PagerdutyConfig {
		if err := n.sendPagerDutyNotification(pdConfig.GetServiceKey(), a); err != nil {
			glog.Error("Error sending PagerDuty notification: ", err)
		}
	}
	for _, emailConfig := range config.EmailConfig {
		if *smtpSmartHost == "" {
			glog.Warning("No SMTP smarthost configured, not sending email notification.")
			continue
		}
		if err := n.sendEmailNotification(emailConfig.GetEmail(), a); err != nil {
			glog.Error("Error sending email notification: ", err)
		}
	}
	for _, poConfig := range config.PushoverConfig {
		if err := n.sendPushoverNotification(poConfig.GetToken(), poConfig.GetUserKey(), a); err != nil {
			glog.Error("Error sending Pushover notification: ", err)
		}
	}
	for _, hcConfig := range config.HipchatConfig {
		if err := n.sendHipChatNotification(hcConfig.GetAuthToken(), hcConfig.GetRoomId(), hcConfig.GetColor(), hcConfig.GetNotify(), a); err != nil {
			glog.Error("Error sending HipChat notification: ", err)
		}
	}
}

func (n *notifier) Dispatch() {
	for req := range n.pendingNotifications {
		n.handleNotification(req.alert, req.notificationConfig)
	}
}

func (n *notifier) Close() {
	close(n.pendingNotifications)
}
