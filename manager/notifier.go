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
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"text/template"

	"github.com/golang/glog"
	"github.com/thorduri/pushover"

	pb "github.com/prometheus/alertmanager/config/generated"
)

const contentTypeJson = "application/json"

var bodyTmpl = template.Must(template.New("message").Parse(`From: Prometheus Alertmanager <{{.From}}>
To: {{.To}}
Subject: [ALERT] {{.Alert.Labels.alertname}}: {{.Alert.Summary}}

{{.Alert.Description}}

Grouping labels:
{{range $label, $value := .Alert.Labels}}
  {{$label}} = "{{$value}}"{{end}}

Payload labels:
{{range $label, $value := .Alert.Payload}}
  {{$label}} = "{{$value}}"{{end}}`))

var (
	notificationBufferSize = flag.Int("notificationBufferSize", 1000, "Size of buffer for pending notifications.")
	pagerdutyApiUrl        = flag.String("pagerdutyApiUrl", "https://events.pagerduty.com/generic/2010-04-15/create_event.json", "PagerDuty API URL.")
	smtpSmartHost          = flag.String("smtpSmartHost", "", "Address of the smarthost to send all email notifications to.")
	smtpSender             = flag.String("smtpSender", "alertmanager@example.org", "Sender email address to use in email notifications.")
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

func writeEmailBody(w io.Writer, from string, to string, a *Alert) error {
	err := bodyTmpl.Execute(w, struct {
		From  string
		To    string
		Alert *Alert
	}{
		From:  from,
		To:    to,
		Alert: a,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *notifier) sendEmailNotification(to string, a *Alert) error {
	// Connect to the SMTP smarthost.
	c, err := smtp.Dial(*smtpSmartHost)
	if err != nil {
		return err
	}
	defer c.Quit()

	// Authenticate if we and the server are both configured for it.
	hasAuth, mechs := c.Extension("AUTH")
	username := os.Getenv("SMTP_AUTH_USERNAME")
	if hasAuth && username != "" {
		for _, mech := range strings.Split(mechs, " ") {
			switch mech {
			default:
				continue
			case "CRAM-MD5":
				secret := os.Getenv("SMTP_AUTH_SECRET")
				if secret == "" {
					continue
				}
				if err := c.Auth(smtp.CRAMMD5Auth(username, secret)); err != nil {
					return fmt.Errorf("cram-md5 auth failed: %s", err)
				}
			case "PLAIN":
				password := os.Getenv("SMTP_AUTH_PASSWORD")
				if password == "" {
					continue
				}
				identity := os.Getenv("SMTP_AUTH_IDENTITY")

				// PLAIN auth requires TLS to be started first.
				host, _, _ := net.SplitHostPort(*smtpSmartHost)
				if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
					return fmt.Errorf("starttls failed: %s", err)
				}

				if err := c.Auth(smtp.PlainAuth(identity, username, password, host)); err != nil {
					return fmt.Errorf("plain auth failed: %s", err)
				}
			}
			break
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
}

func (n *notifier) Dispatch() {
	for req := range n.pendingNotifications {
		n.handleNotification(req.alert, req.notificationConfig)
	}
}

func (n *notifier) Close() {
	close(n.pendingNotifications)
}
