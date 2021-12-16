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
	"github.com/prometheus/common/model"
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
	return &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		storage: make(map[string]Data),
	}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	go n.clean()

	changedMessages := make([]string, 0)
	for _, alert := range data.Alerts {
		if ok, ts := n.getTsByFP(alert.Fingerprint); ok {
			changedMessages = append(changedMessages, ts)
			n.mu.Lock()
			for i, al := range n.storage[ts].Alerts {
				if al.Fingerprint == alert.Fingerprint {
					n.storage[ts].Alerts[i].Status = alert.Status
					n.storage[ts].Alerts[i].EndsAt = alert.EndsAt
				}
			}
			n.mu.Unlock()
		} else {
			ts, err := n.send(data, "")
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

	for _, alert := range data.Alerts {
		for ts, msg := range n.storage {
			for i, al := range msg.Alerts {
				if alert.Fingerprint == al.Fingerprint && alert.Status == string(model.AlertResolved) {
					n.storage[ts].Alerts[i].Status = string(model.AlertResolved)
				}
			}
		}
	}

	for _, msg := range changedMessages {
		_, err := n.send(n.storage[msg].Data, msg)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (n *Notifier) send(data *template.Data, ts string) (string, error) {
	var err error
	var (
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	attachmets := &slack.Attachment{
		TitleLink: tmplText(n.conf.TitleLink),
		Text:      tmplText(n.conf.Text),
		ImageURL:  tmplText(n.conf.ImageURL),
		Footer:    tmplText(n.conf.Footer),
		Color:     n.conf.Color,
	}

	var numActions = len(n.conf.Actions)
	if numActions > 0 {
		var actions = make([]slack.AttachmentAction, numActions)
		for index, action := range n.conf.Actions {
			slackAction := slack.AttachmentAction{
				Type:  slack.ActionType(action.Type),
				Text:  tmplText(action.Text),
				URL:   tmplText(action.URL),
				Style: tmplText(action.Style),
				Name:  action.Name,
				Value: tmplText(action.Value),
			}
			actions[index] = slackAction
		}
		attachmets.Actions = actions
	}

	var numFiring = len(data.Alerts.Firing())
	if numFiring == 0 {
		attachmets.Color = "good"
	}

	att := slack.MsgOptionAttachments(*attachmets)

	if ts != "" {
		_, _, messageTs, err := n.client.UpdateMessage(n.conf.Channel, ts, att)
		if err != nil {
			return "", err
		}
		return messageTs, nil
	} else {
		_, messageTs, err := n.client.PostMessage(n.conf.Channel, att)

		if err != nil {
			return "", err
		}
		return messageTs, nil
	}
}

func (n *Notifier) getTsByFP(fp string) (bool, string) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for ts, msg := range n.storage {
		for _, alert := range msg.Alerts {
			if fp == alert.Fingerprint {
				return true, ts
			}
		}
	}
	return false, ""
}

func (n *Notifier) storageChecker() {
	for i, data := range n.storage {
		var numFiring = len(data.Alerts.Firing())
		if numFiring == 0 {
			n.storage[i].Data.Status = string(model.AlertResolved)
		}
	}
}

func (n *Notifier) storageCleaner() {
	for i, _ := range n.storage {
		if n.storage[i].Data.Status == string(model.AlertResolved) {
			delete(n.storage, i)
		}
	}
}

func (n *Notifier) clean() {

	for true {
		time.Sleep(600 * time.Second)
		n.storageChecker()
		n.storageCleaner()
	}
}
