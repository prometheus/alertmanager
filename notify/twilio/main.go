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

package twilio

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/twilio/twilio-go"
	twapi "github.com/twilio/twilio-go/rest/api/v2010"

	"github.com/prometheus/alertmanager/blobstore"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for generic sigma.
type Notifier struct {
	conf    *config.TwilioConfig
	tmpl    *template.Template
	logger  log.Logger
	mu      sync.Mutex
	history map[model.Fingerprint]*History
}

type History struct {
	Time  time.Time
	Value int
}

// New returns a new Sigma.
func New(conf *config.TwilioConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	return &Notifier{
		conf:    conf,
		tmpl:    t,
		logger:  l,
		history: make(map[model.Fingerprint]*History),
	}, nil
}

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)

	tw := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: n.conf.AccountID,
		Password: string(n.conf.Token),
	})

	n.mu.Lock()
	for _, a := range as {
		w := n.conf.DefaultWeight
		if _, ok := a.Labels["weight"]; ok {
			if i, err := strconv.Atoi(string(a.Labels["weight"])); err == nil {
				w = i
			}
		}

		if a.EndsAt.IsZero() {
			if v, ok := n.history[a.Fingerprint()]; !ok {
				n.history[a.Fingerprint()] = &History{Value: w, Time: time.Now()}
			} else {
				n.history[a.Fingerprint()] = &History{Value: max(v.Value, w), Time: time.Now()}

			}
		} else {
			if _, ok := n.history[a.Fingerprint()]; ok {
				n.history[a.Fingerprint()].Value = 0
			}
		}
	}

	totalWeight := 0
	for k, v := range n.history {
		if v.Time.Add(time.Hour*2).Before(time.Now()) || v.Value <= 0 {
			delete(n.history, k)
		} else {
			totalWeight += v.Value
		}
	}
	n.mu.Unlock()

	n.logger.Log("current alerts weight", totalWeight, "target", n.conf.Threshold)
	if totalWeight < n.conf.Threshold {
		return false, nil
	}

	allErrors := make([]error, 0)
	switch n.conf.NotificationType {
	case "sms":
		for _, recepient := range n.conf.Recipient {
			req := &twapi.CreateMessageParams{From: &n.conf.SenderName, To: &recepient}
			req.SetBody(tmplText(n.conf.Text))
			resp, err := tw.Api.CreateMessage(req)
			if err != nil {
				allErrors = append(allErrors, err)
			} else {
				r, _ := json.Marshal(*resp)
				n.logger.Log("Twilio response", r)
			}
		}
	case "voice":
		voiceReq := VoiceRequest{Say: tmplText(n.conf.Text)}
		if n.conf.PlayFileUrl != nil {
			voiceReq.Play = n.conf.PlayFileUrl.String()
		}
		voiceData, err := xml.Marshal(voiceReq)
		if err != nil {
			return false, err
		}

		key, err := blobstore.PutFileName("twilio", uuid.NewV4().String(), &blobstore.File{Data: voiceData, ContentType: toPtr("text/xml")}, toPtr(time.Hour*24))
		if err != nil {
			return false, fmt.Errorf("failed to write voice data to blob storage, err: %w", err)
		}
		url := *n.conf.AlertManagerUrl
		url.Path = fmt.Sprintf("/blobstore/%s", key)

		for _, recepient := range n.conf.Recipient {
			req := &twapi.CreateCallParams{From: &n.conf.SenderName, To: &recepient}
			req.SetMethod("GET")
			req.SetUrl(url.String())
			resp, err := tw.Api.CreateCall(req)
			if err != nil {
				allErrors = append(allErrors, err)
			} else {
				r, _ := json.Marshal(*resp)
				n.logger.Log("callback_url", url.String(), "Twilio response", r)
			}
		}
	}

	if len(allErrors) > 0 {
		return false, &Error{Errors: allErrors}
	}

	return false, nil
}

type Error struct {
	Errors []error
}

func (e *Error) Error() string {
	return fmt.Sprintf("Errors count: %d", len(e.Errors))
}

type VoiceRequest struct {
	XMLName xml.Name `xml:"Response"`
	Text    string   `xml:",chardata"`
	Play    string   `xml:"Play"`
	Say     string   `xml:"Say"`
}
