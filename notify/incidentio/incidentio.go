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

const (
	// maxPayloadSize is the maximum size of the JSON payload incident.io accepts (512KB).
	maxPayloadSize = 512 * 1024
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
	// conf.HTTPConfig is likely to be the global shared HTTPConfig, so we take a
	// copy to avoid modifying it.
	httpConfig := *conf.HTTPConfig

	// If an alert source token is provided, we use that one instead of whatever configuration is included in `http_config`.
	var token string
	if conf.AlertSourceToken != "" {
		token = string(conf.AlertSourceToken)
	}

	if conf.AlertSourceTokenFile != "" {
		content, err := os.ReadFile(conf.AlertSourceTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read alert_source_token_file: %w", err)
		}
		token = strings.TrimSpace(string(content))
	}

	if token != "" {
		httpConfig.Authorization = &commoncfg.Authorization{
			Type:        "Bearer",
			Credentials: commoncfg.Secret(token),
		}
	}

	client, err := commoncfg.NewClientFromConfig(httpConfig, "incidentio", httpOpts...)
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
			RetryCodes:        []int{http.StatusTooManyRequests},
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

// encodeMessage encodes the message and drops all alerts except the first one if it exceeds maxPayloadSize.
func (n *Notifier) encodeMessage(msg *Message) (bytes.Buffer, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return buf, fmt.Errorf("failed to encode incident.io message: %w", err)
	}

	if buf.Len() <= maxPayloadSize {
		return buf, nil
	}

	originalSize := buf.Len()

	// Drop all but the first alert in the message. For most use cases, a single
	// alert will be created in incident.io for the group, so including more than
	// one alert in that group is useful but non-essential.
	msg.Alerts = msg.Alerts[:1]

	// Re-encode after annotation truncation
	buf.Reset()
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return buf, fmt.Errorf("failed to encode incident.io message after annotation truncation: %w", err)
	}

	if buf.Len() <= maxPayloadSize {
		n.logger.Warn("Truncated alert content due to incident.io payload size limit",
			"original_size", originalSize,
			"final_size", buf.Len(),
			"max_size", maxPayloadSize)

		return buf, nil
	}

	// Still attempt to send the message even if it exceeds the limit, but log an
	// error to explain why this is likely to fail.
	n.logger.Error("Truncated alert content due to incident.io payload size limit, but still exceeds limit",
		"original_size", originalSize,
		"final_size", buf.Len(),
		"max_size", maxPayloadSize)

	return buf, nil
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
		Version:         "1",
		Data:            data,
		GroupKey:        groupKey.String(),
		TruncatedAlerts: numTruncated,
	}

	buf, err := n.encodeMessage(msg)
	if err != nil {
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
		ctxWithTimeout, cancel := context.WithTimeoutCause(ctx, n.conf.Timeout, fmt.Errorf("configured incident.io timeout reached (%s)", n.conf.Timeout))
		defer cancel()
		ctx = ctxWithTimeout
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
func errDetails(_ int, body io.Reader) string {
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

	return strings.Join(parts, ": ")
}
