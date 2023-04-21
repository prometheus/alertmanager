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

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for generic webhooks.
type Notifier struct {
	conf    *config.WebhookConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new Webhook.
func New(conf *config.WebhookConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "webhook", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: l,
		client: client,
		// Webhooks are assumed to respond with 2xx response codes on a successful
		// request and 5xx response codes are assumed to be recoverable.
		retrier: &notify.Retrier{
			CustomDetailsFunc: func(_ int, body io.Reader) string {
				return errDetails(body, conf.URL.String())
			},
		},
	}, nil
}

// Message defines the JSON object send to webhook endpoints.
type Message struct {
	*template.Data

	// The protocol version.
	Version         string `json:"version"`
	GroupKey        string `json:"groupKey"`
	TruncatedAlerts uint64 `json:"truncatedAlerts"`
}

func truncateAlerts(maxAlerts uint64, alerts []*types.Alert) ([]*types.Alert, uint64) {
	if maxAlerts != 0 && uint64(len(alerts)) > maxAlerts {
		return alerts[:maxAlerts], uint64(len(alerts)) - maxAlerts
	}

	return alerts, 0
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	alerts, numTruncated := truncateAlerts(n.conf.MaxAlerts, alerts)
	data := notify.GetTemplateData(ctx, n.tmpl, alerts, n.logger)

	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		level.Error(n.logger).Log("err", err)
	}

	msg := &Message{
		Version:         "4",
		Data:            data,
		GroupKey:        groupKey.String(),
		TruncatedAlerts: numTruncated,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	var url string
	if n.conf.URL != nil {
		url = n.conf.URL.String()
	} else {
		content, err := os.ReadFile(n.conf.URLFile)
		if err != nil {
			return false, fmt.Errorf("read url_file: %w", err)
		}
		url = string(content)
	}

	resp, err := notify.PostJSON(ctx, n.client, url, &buf)
	if err != nil {
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	return shouldRetry, err
}

func errDetails(body io.Reader, url string) string {
	if body == nil {
		return url
	}
	bs, err := io.ReadAll(body)
	if err != nil {
		return url
	}
	return fmt.Sprintf("%s: %s", url, string(bs))
}
