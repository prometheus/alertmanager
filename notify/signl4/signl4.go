// Copyright 2026 Prometheus Team
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

package signl4

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var userAgentHeader = fmt.Sprintf("Alertmanager/%s", version.Version)

// Notifier implements a Notifier for SIGNL4.
type Notifier struct {
	conf    *config.SIGNL4Config
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new SIGNL4.
func New(conf *config.SIGNL4Config, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "signl4", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: l,
		client: client,
		// SIGNL4 HTTP requests are assumed to respond with 2xx response codes on a successful
		// request and 5xx response codes are assumed to be recoverable.
		retrier: &notify.Retrier{},
	}, nil
}

// Message defines the JSON object send to SIGNL4 endpoints.
type Message struct {
	*template.Data

	// The protocol version.
	Version  string `json:"version"`
	GroupKey string `json:"groupKey"`

	Title               string `json:"Title"`
	Message             string `json:"Message"`
	XS4Service          string `json:"X-S4-Service"`
	XS4Location         string `json:"X-S4-Location"`
	XS4AlertingScenario string `json:"X-S4-AlertingScenario"`
	XS4Filtering        bool   `json:"X-S4-Filtering"`
	XS4ExternalID       string `json:"X-S4-ExternalID"`
	XS4Status           string `json:"X-S4-Status"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {

	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}
	logger := n.logger.With("group_key", key)

	var (
		alerts = types.Alerts(as...)
		data   = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		status = "new"
	)

	// Check for resolved
	if alerts.Status() == model.AlertResolved {
		status = "resolved"
	}

	logger.Debug("extracted group key", "eventType", status)

	msg := &Message{
		Data:                data,
		Title:               n.conf.Title,
		Message:             n.conf.Message,
		XS4Service:          n.conf.XS4Service,
		XS4Location:         n.conf.XS4Location,
		XS4AlertingScenario: n.conf.XS4AlertingScenario,
		XS4Filtering:        n.conf.XS4Filtering,
		XS4ExternalID:       key.String(),
		XS4Status:           status,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return false, err
	}

	var url string = "https://connect.signl4.com/webhook/" + string(n.conf.TeamSecret)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgentHeader)

	resp, err := n.client.Do(req.WithContext(ctx))
	if err != nil {
		return true, err
	}
	notify.Drain(resp)

	return n.retrier.Check(resp.StatusCode, nil)
}
