// Copyright The Prometheus Authors
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

// Package matrix sends notifications to a Matrix room through the
// Client-Server API.
package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
)

const (
	// A Matrix event, including the envelope the homeserver adds, is
	// limited to 65536 bytes. The body and the HTML body share that limit.
	// See https://spec.matrix.org/latest/client-server-api/#size-limits.
	maxBodyBytes = 20000
	// The size of an error response that is read is bounded too.
	maxResponseBytes = 64 << 10
)

// Notifier implements a Notifier for Matrix notifications.
type Notifier struct {
	conf    *MatrixConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new Matrix notifier.
func New(c *MatrixConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := notify.NewClientWithTracing(*c.HTTPConfig, "matrix", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
	}, nil
}

// message is the content of an m.room.message event.
// See https://spec.matrix.org/latest/client-server-api/#mtext.
type message struct {
	MsgType       string `json:"msgtype"`
	Body          string `json:"body"`
	Format        string `json:"format,omitempty"`
	FormattedBody string `json:"formatted_body,omitempty"`
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*alert.Alert) notify.NotifyVerdict {
	key, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	logger := n.logger.With("group_key", key)
	logger.Debug("extracted group key")

	data := notify.GetTemplateData(ctx, n.tmpl, as, logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	roomID := tmpl(n.conf.RoomID)
	body := tmpl(n.conf.Message)
	var formattedBody string
	if n.conf.HTMLMessage != "" {
		formattedBody = notify.TmplHTML(n.tmpl, data, &err)(n.conf.HTMLMessage)
	}
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	if !validRoomID(roomID) {
		return notify.Unrecoverable(fmt.Errorf("room_id %q is not a room ID such as !abc123:example.org", roomID), notify.DefaultReason)
	}

	body, truncated := notify.TruncateInBytes(body, maxBodyBytes)
	if truncated {
		logger.Warn("Truncated message", "max_bytes", maxBodyBytes)
	}
	msg := message{MsgType: n.conf.MsgType, Body: body}
	if formattedBody != "" {
		// Cutting HTML can leave it malformed, so a message too long for the
		// limit goes out as plain text only.
		if len(formattedBody) > maxBodyBytes {
			logger.Warn("HTML message too long, sending the plain text message only", "max_bytes", maxBodyBytes)
		} else {
			msg.Format = "org.matrix.custom.html"
			msg.FormattedBody = formattedBody
		}
	}

	token, err := n.accessToken()
	if err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}
	// Each attempt gets its own transaction id: a homeserver drops a request
	// that reuses one, which would also drop a later notification that
	// repeats an earlier one.
	txnID, err := newTxnID()
	if err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}

	var payload bytes.Buffer
	enc := json.NewEncoder(&payload)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(msg); err != nil {
		return notify.Unrecoverable(err, notify.DefaultReason)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, n.sendURL(roomID, txnID), &payload)
	if err != nil {
		return notify.Unrecoverable(notify.RedactURL(err), notify.DefaultReason)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", notify.UserAgentHeader)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := n.client.Do(req)
	received := time.Now()
	if err != nil {
		return notify.Retry(0, notify.RedactURL(err), notify.DefaultReason)
	}
	defer notify.Drain(resp)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return notify.Retry(0, err, notify.DefaultReason)
	}
	shouldRetry, err := n.retrier.Check(resp.StatusCode, bytes.NewReader(respBody))
	if err != nil {
		reason := notify.GetFailureReasonFromStatusCode(resp.StatusCode)
		if shouldRetry {
			retryAfter := retryAfter(resp.Header, respBody, received)
			if retryAfter > 0 {
				logger.Warn("Rate limited by the Matrix homeserver, delaying retry", "retry_after_secs", retryAfter.Seconds())
			}
			return notify.Retry(retryAfter, err, reason)
		}
		return notify.Unrecoverable(err, reason)
	}
	return notify.Success()
}

func (n *Notifier) accessToken() (string, error) {
	if n.conf.AccessTokenFile == "" {
		return string(n.conf.AccessToken), nil
	}
	b, err := os.ReadFile(n.conf.AccessTokenFile)
	if err != nil {
		return "", fmt.Errorf("read access_token_file: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// sendURL is the endpoint that sends an m.room.message event to the room.
// See https://spec.matrix.org/latest/client-server-api/#put_matrixclientv3roomsroomidsendeventtypetxnid.
func (n *Notifier) sendURL(roomID, txnID string) string {
	return strings.TrimSuffix(n.conf.HomeserverURL.String(), "/") +
		"/_matrix/client/v3/rooms/" + url.PathEscape(roomID) +
		"/send/m.room.message/" + url.PathEscape(txnID)
}

func newTxnID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate transaction id: %w", err)
	}
	return "alertmanager-" + hex.EncodeToString(b), nil
}

// retryAfter is the delay a rate limited response asks for: the Retry-After
// header, or the retry_after_ms field of the error, which homeservers that
// predate the header send.
// See https://spec.matrix.org/latest/client-server-api/#rate-limiting.
func retryAfter(h http.Header, body []byte, received time.Time) time.Duration {
	if d := notify.ParseRetryAfter(h, received); d > 0 {
		return d
	}
	var limited struct {
		RetryAfterMs int64 `json:"retry_after_ms"`
	}
	if err := json.Unmarshal(body, &limited); err != nil || limited.RetryAfterMs <= 0 {
		return 0
	}
	return time.Duration(limited.RetryAfterMs) * time.Millisecond
}
