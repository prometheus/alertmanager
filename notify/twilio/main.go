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
	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/twilio/twilio-go"
	twapi "github.com/twilio/twilio-go/rest/api/v2010"
)

// Notifier implements a Notifier for generic sigma.
type Notifier struct {
	conf   *config.TwilioConfig
	tmpl   *template.Template
	logger log.Logger
}

// New returns a new Sigma.
func New(conf *config.TwilioConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: l,
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

		id := Storage.Put(voiceData)
		url := *n.conf.AlertManagerUrl
		url.Path = "/callback/twilio"
		args := url.Query()
		args.Set("id", id)
		url.RawQuery = args.Encode()

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
