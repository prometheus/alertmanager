package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

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

	// TODO(fabxc): implement retrying as long as context is not canceled.
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

// type SlackMessage struct {
// 	Channel     string            `json:"channel,omitempty"`
// 	Attachments []SlackAttachment `json:"attachments"`
// }

// type SlackAttachment struct {
// 	Title      string                 `json:"title,omitempty"`
// 	TitleLink  string                 `json:"title_link,omitempty"`
// 	Pretext    string                 `json:"pretext,omitempty"`
// 	Text       string                 `json:"text"`
// 	Fallback   string                 `json:"fallback"`
// 	Color      string                 `json:"color,omitempty"`
// 	MarkdownIn []string               `json:"mrkdwn_in,omitempty"`
// 	Fields     []SlackAttachmentField `json:"fields,omitempty"`
// }

// type SlackAttachmentField struct {
// 	Title string `json:"title"`
// 	Value string `json:"value"`
// 	Short bool   `json:"short,omitempty"`
// }

// type Slack struct{}

// func (s *Slack) Notify(ctx context.Context, alerts ...*types.Alert) error {
// 	group, ok := ctx.Value(NotifyGroup).(string)
// 	if !ok {
// 		return fmt.Errorf("group identifier missing")
// 	}
// 	// https://api.slack.com/incoming-webhooks
// 	var (
// 		incidentKey = a.Fingerprint()
// 		color       = ""
// 		status      = ""
// 	)
// 	switch op {
// 	case notificationOpTrigger:
// 		color = config.GetColor()
// 		status = "firing"
// 	case notificationOpResolve:
// 		color = config.GetColorResolved()
// 		status = "resolved"
// 	}

// 	statusField := &slackAttachmentField{
// 		Title: "Status",
// 		Value: status,
// 		Short: true,
// 	}

// 	attachment := &slackAttachment{
// 		Fallback:  fmt.Sprintf("*%s %s*: %s (<%s|view>)", html.EscapeString(a.Labels["alertname"]), status, html.EscapeString(a.Summary), a.Payload["generatorURL"]),
// 		Pretext:   fmt.Sprintf("*%s*", html.EscapeString(a.Labels["alertname"])),
// 		Title:     html.EscapeString(a.Summary),
// 		TitleLink: a.Payload["generatorURL"],
// 		Text:      html.EscapeString(a.Description),
// 		Color:     color,
// 		MrkdwnIn:  []string{"fallback", "pretext"},
// 		Fields: []slackAttachmentField{
// 			*statusField,
// 		},
// 	}

// 	req := &slackReq{
// 		Channel: config.GetChannel(),
// 		Attachments: []slackAttachment{
// 			*attachment,
// 		},
// 	}

// 	buf, err := json.Marshal(req)
// 	if err != nil {
// 		return err
// 	}

// 	timeout := time.Duration(*slackConnectTimeout) * time.Second
// 	client := http.Client{
// 		Timeout: timeout,
// 	}
// 	resp, err := client.Post(
// 		config.GetWebhookUrl(),
// 		contentTypeJSON,
// 		bytes.NewBuffer(buf),
// 	)
// 	if err != nil {
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	respBuf, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		return err
// 	}

// 	log.Infof("Sent Slack notification (channel %s): %v: HTTP %d: %s", config.GetChannel(), incidentKey, resp.StatusCode, respBuf)
// 	// BUG: Check response for result of operation.
// 	return nil
// }
