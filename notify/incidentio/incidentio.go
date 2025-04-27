// Copyright 2025 Prometheus Team
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

package incidentio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for incident.io.
type Notifier struct {
	conf    *config.IncidentioConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new incident.io notifier.
func New(conf *config.IncidentioConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	// If alert source token is specified, set authorization in HTTP config
	if conf.HTTPConfig == nil {
		conf.HTTPConfig = &commoncfg.HTTPClientConfig{}
	}

	if conf.AlertSourceToken != "" {
		if conf.HTTPConfig.Authorization == nil {
			conf.HTTPConfig.Authorization = &commoncfg.Authorization{
				Type:        "Bearer",
				Credentials: commoncfg.Secret(conf.AlertSourceToken),
			}
		}
	} else if conf.AlertSourceTokenFile != "" {
		content, err := os.ReadFile(conf.AlertSourceTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read alert_source_token_file: %w", err)
		}

		if conf.HTTPConfig.Authorization == nil {
			conf.HTTPConfig.Authorization = &commoncfg.Authorization{
				Type:        "Bearer",
				Credentials: commoncfg.Secret(strings.TrimSpace(string(content))),
			}
		}
	}

	client, err := commoncfg.NewClientFromConfig(*conf.HTTPConfig, "incidentio", httpOpts...)
	if err != nil {
		return nil, err
	}

	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: l,
		client: client,
		// Always retry on 429 (rate limiting) and 5xx response codes.
		retrier: &notify.Retrier{
			RetryCodes: []int{
				http.StatusTooManyRequests, // 429
				http.StatusInternalServerError,
				http.StatusBadGateway,
				http.StatusServiceUnavailable,
				http.StatusGatewayTimeout,
			},
			CustomDetailsFunc: errDetails,
		},
	}, nil
}

// Message defines the JSON object sent to incident.io endpoints.
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
		return false, err
	}

	n.logger.Debug("incident.io notification", "groupKey", groupKey)

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
		url = strings.TrimSpace(string(content))
	}

	if n.conf.Timeout > 0 {
		postCtx, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured incident.io timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = postCtx
	}

	resp, err := notify.PostJSON(ctx, n.client, url, &buf)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w: %w", err, context.Cause(ctx))
		}
		return true, notify.RedactURL(err)
	}
	defer notify.Drain(resp)

	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	return shouldRetry, err
}

// errDetails extracts error details from the response for better error messages.
func errDetails(status int, body io.Reader) string {
	if body == nil {
		return ""
	}

	// Try to decode the error message from JSON response
	var errorResponse struct {
		Message string   `json:"message"`
		Errors  []string `json:"errors"`
		Error   string   `json:"error"`
	}

	if err := json.NewDecoder(body).Decode(&errorResponse); err != nil {
		return ""
	}

	// Format the error message
	var parts []string
	if errorResponse.Message != "" {
		parts = append(parts, errorResponse.Message)
	}
	if errorResponse.Error != "" {
		parts = append(parts, errorResponse.Error)
	}
	if len(errorResponse.Errors) > 0 {
		parts = append(parts, strings.Join(errorResponse.Errors, ", "))
	}

	if len(parts) > 0 {
		return strings.Join(parts, ": ")
	}
	return ""
}
