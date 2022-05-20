// Copyright 2019 Prometheus Team
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

package slackV2

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/slack-go/slack"
	"strings"
	"sync"
	"time"
)

// Notifier implements a Notifier for Slack notifications.
type Notifier struct {
	conf    *config.SlackConfigV2
	tmpl    *template.Template
	logger  log.Logger
	client  *slack.Client
	storage map[string]Data
	mu      sync.RWMutex
}

type Data struct {
	*template.Data
	SendAt time.Time
}

// New returns a new Slack notification handler.
func New(c *config.SlackConfigV2, t *template.Template, l log.Logger) (*Notifier, error) {
	token := string(c.Token)
	client := slack.New(token, slack.OptionDebug(c.Debug))

	notifier := &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		storage: make(map[string]Data),
	}
	go notifier.storageCleaner()
	return notifier, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)

	if n.conf.Debug {
		level.Debug(n.logger).Log("Alert Data", data)
	}

	changedMessages := make([]string, 0)
	notifyMessages := make([]string, 0)
	for _, newAlert := range data.Alerts {
		messages := n.getMessagesByFingerprint(newAlert.Fingerprint)
		changedMessages = append(changedMessages, messages...)
		if len(messages) > 0 {
			n.mu.Lock()
			for _, ts := range messages {
				changed := false
				for i := range n.storage[ts].Alerts {
					if n.storage[ts].Alerts[i].Fingerprint == newAlert.Fingerprint {
						if n.storage[ts].Alerts[i].Status != newAlert.Status {
							n.storage[ts].Alerts[i].Status = newAlert.Status
							changed = true
						}
						n.storage[ts].Alerts[i].EndsAt = newAlert.EndsAt
						n.storage[ts].Data.CommonAnnotations = data.CommonAnnotations

					}
				}
				if !changed {
					notifyMessages = append(notifyMessages, ts)
				}
			}
			n.mu.Unlock()
		} else {
			// Делаем проверку, что бы не отправлять резолвы на "осиратевшие алерты", у которых 0 firing
			if len(data.Alerts.Firing()) > 0 {
				ts, err := n.send(data, "")
				if err != nil {
					return false, err
				}
				n.mu.Lock()
				n.storage[ts] = Data{Data: data}
				n.mu.Unlock()
				notifyMessages = append(notifyMessages, ts)
			}
		}

		for _, ts := range UniqStr(notifyMessages) {
			if n.storage[ts].SendAt.IsZero() || n.storage[ts].SendAt.Add(time.Duration(n.conf.MentionDelay)).Before(time.Now()) {
				if err := n.sendNotify(ts); err != nil {
					return false, err
				}
				n.mu.Lock()
				n.storage[ts] = Data{Data: n.storage[ts].Data, SendAt: time.Now()}
				n.mu.Unlock()
			}
		}
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, msg := range changedMessages {
		_, err := n.send(n.storage[msg].Data, msg)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (n *Notifier) send(data *template.Data, ts string) (string, error) {
	channel := n.conf.Channel
	if n.conf.GrafanaToken != "" {
		for _, alert := range data.Alerts {
			for _, v := range alert.Annotations.SortedPairs() {
				switch v.Name {
				case "channel_override":
					channel = v.Value
				}
			}
		}
	}

	attachment := slack.Attachment{}

	if n.conf.GrafanaToken != "" {
		attachment = slack.Attachment{
			Color:  n.conf.Color,
			Blocks: n.formatGrafanaMessage(data),
		}
	} else {
		attachment = slack.Attachment{
			Color:  n.conf.Color,
			Blocks: n.formatMessage(data),
		}
	}

	if len(data.Alerts.Firing()) == 0 {
		attachment.Color = "#1aad21"
	}

	att := slack.MsgOptionAttachments(attachment)

	if ts != "" {
		_, _, messageTs, err := n.client.UpdateMessage(channel, ts, att)
		return messageTs, err
	} else {
		_, messageTs, err := n.client.PostMessage(channel, att)
		return messageTs, err
	}
}

func (n *Notifier) sendNotify(ts string) error {
	if len(n.conf.Mentions) == 0 {
		return nil
	}
	users := make([]string, len(n.conf.Mentions))
	for i, val := range n.conf.Mentions {
		switch strings.ToLower(val.Type) {
		case "group":
			users[i] = fmt.Sprintf("<!subteam^%s|@%s>", val.ID, val.Name)
		case "user":
			users[i] = fmt.Sprintf("<@%s>", val.ID)
		}
	}

	text := fmt.Sprintf("Look here %s", strings.Join(users, " "))
	opts := make([]slack.MsgOption, 0)
	opts = append(opts, slack.MsgOptionTS(ts))
	opts = append(opts, slack.MsgOptionText(text, false))
	//
	_, _, err := n.client.PostMessage(n.conf.Channel, opts...)
	return err

}

func (n *Notifier) getMessagesByFingerprint(fp string) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	ts := make([]string, 0)
	for i, msg := range n.storage {
		for _, alert := range msg.Alerts {
			if fp == alert.Fingerprint {
				ts = append(ts, i)
			}
		}
	}
	return ts
}

func (n *Notifier) storageCleaner() {
	for range time.Tick(time.Minute * 10) {
		n.mu.Lock()
		for k, data := range n.storage {
			if len(data.Alerts.Firing()) == 0 {
				delete(n.storage, k)
			}
		}
		n.mu.Unlock()
	}
}
