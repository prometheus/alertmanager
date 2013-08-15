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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"sync"
	"text/template"

	pb "github.com/prometheus/alertmanager/config/generated"
)

const contentTypeJson = "application/json"

const bodyTmpl = `Subject: [ALERT] {{.Labels.alertname}}: {{.Summary}}

{{.Description}}

Grouping labels:
{{range $label, $value := .Labels}}
  {{$label}} = "{{$value}}"
{{end}}

Payload labels:
{{range $label, $value := .Payload}}
  {{$label}} = "{{$value}}"
{{end}}`

var (
	notificationBufferSize = flag.Int("notificationBufferSize", 1000, "Size of buffer for pending notifications.")
	pagerdutyApiUrl        = flag.String("pagerdutyApiUrl", "https://events.pagerduty.com/generic/2010-04-15/create_event.json", "PagerDuty API URL.")
	smtpSmartHost          = flag.String("smtpSmartHost", "", "Address of the smarthost to send all email notifications to.")
	smtpSender             = flag.String("smtpSender", "alertmanager@example.org", "Sender email address to use in email notifications.")
)

// A Notifier is responsible for sending notifications for events according to
// a provided notification configuration.
type Notifier interface {
	// Queue a notification for asynchronous dispatching.
	QueueNotification(e *Event, configName string) error
	// Replace current notification configs. Already enqueued messages will remain
	// unaffected.
	SetNotificationConfigs([]*pb.NotificationConfig)
	// Start event notification dispatch loop.
	Dispatch(IsInhibitedInterrogator)
	// Stop the event notification dispatch loop.
	Close()
}

// Request for sending a notification.
type notificationReq struct {
	event              *Event
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

func (n *notifier) QueueNotification(event *Event, configName string) error {
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
		event:              event,
		notificationConfig: nc,
	}
	return nil
}

func (n *notifier) sendPagerDutyNotification(serviceKey string, event *Event) error {
	// http://developer.pagerduty.com/documentation/integration/events/trigger
	incidentKey := event.Fingerprint()
	buf, err := json.Marshal(map[string]interface{}{
		"service_key":  serviceKey,
		"event_type":   "trigger",
		"description":  event.Description,
		"incident_key": incidentKey,
		"details": map[string]interface{}{
			"grouping_labels": event.Labels,
			"extra_labels":    event.Payload,
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

	log.Printf("Sent PagerDuty notification: %v: HTTP %d: %s", incidentKey, resp.StatusCode, respBuf)
	// BUG: Check response for result of operation.
	return nil
}

func (n *notifier) sendEmailNotification(email string, event *Event) error {
	// Connect to the SMTP smarthost.
	c, err := smtp.Dial(*smtpSmartHost)
	if err != nil {
		return err
	}
	defer c.Quit()

	// Set the sender and recipient.
	c.Mail(*smtpSender)
	c.Rcpt(email)

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	t := template.New("message")
	if _, err := t.Parse(bodyTmpl); err != nil {
		return err
	}
	if err := t.Execute(wc, event); err != nil {
		return err
	}
	return nil
}

func (n *notifier) handleNotification(event *Event, config *pb.NotificationConfig, i IsInhibitedInterrogator) {
	if inhibited, _ := i.IsInhibited(event); inhibited {
		return
	}

	for _, pdConfig := range config.PagerdutyConfig {
		if err := n.sendPagerDutyNotification(pdConfig.GetServiceKey(), event); err != nil {
			log.Printf("Error sending PagerDuty notification: %s", err)
		}
	}
	for _, emailConfig := range config.EmailConfig {
		if *smtpSmartHost == "" {
			log.Printf("No SMTP smarthost configured, not sending email notification.")
		}
		if err := n.sendEmailNotification(emailConfig.GetEmail(), event); err != nil {
			log.Printf("Error sending email notification: %s", err)
		}
	}
}

func (n *notifier) Dispatch(i IsInhibitedInterrogator) {
	for req := range n.pendingNotifications {
		n.handleNotification(req.event, req.notificationConfig, i)
	}
}

func (n *notifier) Close() {
	close(n.pendingNotifications)
}
