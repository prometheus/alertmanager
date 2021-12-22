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
	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/slack-go/slack"
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
}

// New returns a new Slack notification handler.
func New(c *config.SlackConfigV2, t *template.Template, l log.Logger) (*Notifier, error) {
	token := c.Token
	client := slack.New(token)
	//client.Debug()

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
	sendHere := false

	//fmt.Printf("%+v\n", data)

	changedMessages := make([]string, 0)
	for _, newAlert := range data.Alerts {
		messages := n.getMessagesByFingerprint(newAlert.Fingerprint)
		changedMessages = append(changedMessages, messages...)
		if len(messages) > 0 {
			n.mu.Lock()
			for _, ts := range messages {
				for i := range n.storage[ts].Alerts {
					if n.storage[ts].Alerts[i].Fingerprint == newAlert.Fingerprint {
						n.storage[ts].Alerts[i].Status = newAlert.Status
						n.storage[ts].Alerts[i].EndsAt = newAlert.EndsAt
						n.storage[ts].Data.CommonAnnotations = data.CommonAnnotations
					}
				}
			}
			n.mu.Unlock()
		} else {
			ts, err := n.send(data, "", sendHere)
			if err != nil {
				return false, err
			}
			n.mu.Lock()
			n.storage[ts] = Data{Data: data}
			n.mu.Unlock()
		}
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, msg := range changedMessages {
		_, err := n.send(n.storage[msg].Data, msg, sendHere)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (n *Notifier) send(data *template.Data, ts string, here bool) (string, error) {

	attachmets := &slack.Attachment{
		Color:  n.conf.Color,
		Blocks: n.formatMessage(data),
	}

	if len(data.Alerts.Firing()) == 0 {
		attachmets.Color = "#1aad21"
	}

	att := slack.MsgOptionAttachments(*attachmets)

	if ts != "" {
		_, _, messageTs, err := n.client.UpdateMessage(n.conf.Channel, ts, att)
		return messageTs, err
	} else {
		_, messageTs, err := n.client.PostMessage(n.conf.Channel, att)
		return messageTs, err
	}
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
