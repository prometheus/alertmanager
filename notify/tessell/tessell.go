// Copyright 2023 Tessell Team
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

package tessell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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
	conf    *config.TessellWebhookConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new Webhook.
func New(conf *config.TessellWebhookConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "tessell", httpOpts...)
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

func truncateAlerts(maxAlerts uint64, alerts []*types.Alert) ([]*types.Alert, uint64) {
	if maxAlerts != 0 && uint64(len(alerts)) > maxAlerts {
		return alerts[:maxAlerts], uint64(len(alerts)) - maxAlerts
	}

	return alerts, 0
}

type NodeStatus struct {
	Status string `json:"status"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	level.Info(n.logger).Log("tessellversion", "9")
	var url string
	if n.conf.URL != nil {
		url = n.conf.URL.String()
	} else {
		content, err := os.ReadFile(n.conf.URLFile)
		if err != nil {
			return false, fmt.Errorf("read url_file: %w", err)
		}
		url = strings.TrimSpace(string(content))
	}

	alerts, numTruncated := truncateAlerts(n.conf.MaxAlerts, alerts)
	level.Info(n.logger).Log("truncatedAlerts", numTruncated)

	shouldRetry := false
	var err error = nil
	for _, alert := range alerts {
		level.Info(n.logger).Log("alertLabels", alert.Labels.String())

		eksNamespace, hasEksNamespace := alert.Labels["eks_namespace"]
		serviceId, hasServiceId := alert.Labels["service_id"]
		serviceInstanceId, hasServiceInstanceId := alert.Labels["service_instance_id"]

		if hasServiceInstanceId && hasEksNamespace && hasServiceId {
			status := "DOWN"
			if strings.Contains(strings.ToLower(alert.Name()), "resolved") {
				status = "UP"
			}
			nodeStatus := &NodeStatus{
				Status: status,
			}

			url = strings.Replace(url, "(EKS_NAMESPACE)", string(eksNamespace), -1)
			url = strings.Replace(url, "(SERVICE_ID)", string(serviceId), -1)
			url = strings.Replace(url, "(SERVICE_INSTANCE_ID)", string(serviceInstanceId), -1)

			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(nodeStatus); err != nil {
				return false, err
			}

			level.Info(n.logger).Log("url", url)

			retries := 5
			resp, err := n.sendPatchRequestWithInstantRetries(ctx, url, &buf, retries)
			if err != nil {
				return true, notify.RedactURL(err)
			}

			level.Info(n.logger).Log("status", status)
			level.Info(n.logger).Log("ApiStatus", resp.StatusCode)

			shouldRetry, err = n.retrier.Check(resp.StatusCode, resp.Body)
			if err != nil {
				err = notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
			}
			notify.Drain(resp)
		}
	}

	return shouldRetry, err
}

func (n *Notifier) sendPatchRequestWithInstantRetries(ctx context.Context, url string, body io.Reader, retries int) (*http.Response, error) {
	if retries < 0 {
		return nil, errors.New("retries should not be less than 0")
	}

	var resp *http.Response
	var err error
	for i := 0; i <= retries; i++ {
		resp, err = notify.PatchJSON(ctx, n.client, url, body)
		// 2xx responses are considered to be always successful.
		if resp.StatusCode/100 == 2 {
			return resp, err
		}

	}
	return resp, err
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
