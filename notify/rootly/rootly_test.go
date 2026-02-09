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

package rootly

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestRootlyRetry(t *testing.T) {
	notifier, err := New(
		&config.RootlyConfig{
			URL:         &config.URL{URL: &url.URL{Scheme: "https", Host: "example.com"}},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			RootlyToken: "test-token",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "retry - error on status %d", statusCode)
	}
}

func TestRootlyRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.RootlyConfig{
			URL:         &config.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			RootlyToken: "test-token",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestRootlyURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "rootly_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.RootlyConfig{
			URLFile:     f.Name(),
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			RootlyToken: "test-token",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestRootlyTokenFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp(t.TempDir(), "rootly_token_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString("test-token-from-file\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.RootlyConfig{
			URL:             &config.URL{URL: u},
			HTTPConfig:      &commoncfg.HTTPClientConfig{},
			RootlyTokenFile: f.Name(),
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestRootlyTruncateAlerts(t *testing.T) {
	alerts := make([]*types.Alert, 10)

	truncatedAlerts, numTruncated := truncateAlerts(0, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, 0, numTruncated)

	truncatedAlerts, numTruncated = truncateAlerts(4, alerts)
	require.Len(t, truncatedAlerts, 4)
	require.EqualValues(t, 6, numTruncated)

	truncatedAlerts, numTruncated = truncateAlerts(100, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, 0, numTruncated)
}

func TestRootlyNotify(t *testing.T) {
	// Test regular notifications are correctly sent
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// Verify the content type header
			contentType := r.Header.Get("Content-Type")
			require.Equal(t, "application/json", contentType)

			// Decode the webhook payload
			var msg Message
			require.NoError(t, json.NewDecoder(r.Body).Decode(&msg))

			// Verify required fields
			require.Equal(t, "1", msg.Version)
			require.NotEmpty(t, msg.GroupKey)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	notifier, err := New(
		&config.RootlyConfig{
			URL:         &config.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			RootlyToken: "test-token",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"alertname": "TestAlert",
				"severity":  "critical",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	retry, err := notifier.Notify(ctx, alert)
	require.NoError(t, err)
	require.False(t, retry)
}

func TestRootlyRetryScenarios(t *testing.T) {
	testCases := []struct {
		name                   string
		statusCode             int
		responseBody           []byte
		expectRetry            bool
		expectErrorMsgContains string
	}{
		{
			name:                   "success response",
			statusCode:             http.StatusOK,
			responseBody:           []byte(`{"status":"success"}`),
			expectRetry:            false,
			expectErrorMsgContains: "",
		},
		{
			name:                   "rate limit response",
			statusCode:             http.StatusTooManyRequests,
			responseBody:           []byte(`{"error":"rate limit exceeded","message":"Too many requests"}`),
			expectRetry:            true,
			expectErrorMsgContains: "rate limit exceeded",
		},
		{
			name:                   "server error response",
			statusCode:             http.StatusInternalServerError,
			responseBody:           []byte(`{"error":"internal error"}`),
			expectRetry:            true,
			expectErrorMsgContains: "internal error",
		},
		{
			name:                   "client error response",
			statusCode:             http.StatusBadRequest,
			responseBody:           []byte(`{"error":"invalid request","message":"Invalid payload format"}`),
			expectRetry:            false,
			expectErrorMsgContains: "invalid request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.statusCode)
					w.Write(tc.responseBody)
				},
			))
			defer server.Close()

			u, err := url.Parse(server.URL)
			require.NoError(t, err)

			notifier, err := New(
				&config.RootlyConfig{
					URL:         &config.URL{URL: u},
					HTTPConfig:  &commoncfg.HTTPClientConfig{},
					RootlyToken: "test-token",
				},
				test.CreateTmpl(t),
				promslog.NewNopLogger(),
			)
			require.NoError(t, err)

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			alert := &types.Alert{
				Alert: model.Alert{
					Labels: model.LabelSet{
						"alertname": "TestAlert",
						"severity":  "critical",
					},
					StartsAt: time.Now(),
					EndsAt:   time.Now().Add(time.Hour),
				},
			}

			retry, err := notifier.Notify(ctx, alert)
			if tc.expectErrorMsgContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectErrorMsgContains)
			}
			require.Equal(t, tc.expectRetry, retry)
		})
	}
}

func TestRootlyErrDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   io.Reader
		expect string
	}{
		{
			name:   "empty body",
			status: http.StatusBadRequest,
			body:   nil,
			expect: "",
		},
		{
			name:   "single error field",
			status: http.StatusBadRequest,
			body:   bytes.NewBufferString(`{"error":"Invalid request"}`),
			expect: "Invalid request",
		},
		{
			name:   "message and errors",
			status: http.StatusBadRequest,
			body:   bytes.NewBufferString(`{"message":"Validation failed","errors":["Field is required","Value too long"]}`),
			expect: "Validation failed: Field is required, Value too long",
		},
		{
			name:   "message and error",
			status: http.StatusTooManyRequests,
			body:   bytes.NewBufferString(`{"message":"Too many requests","error":"Rate limit exceeded"}`),
			expect: "Too many requests: Rate limit exceeded",
		},
		{
			name:   "invalid JSON",
			status: http.StatusBadRequest,
			body:   bytes.NewBufferString(`{invalid}`),
			expect: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := errDetails(tc.status, tc.body)
			if tc.expect == "" {
				require.Empty(t, result)
			} else {
				require.Contains(t, result, tc.expect)
			}
		})
	}
}

func TestRootlyAllAlertsSent(t *testing.T) {
	var receivedAlerts int

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var msg Message
			require.NoError(t, json.NewDecoder(r.Body).Decode(&msg))

			receivedAlerts = len(msg.Alerts)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	notifier, err := New(
		&config.RootlyConfig{
			URL:         &config.URL{URL: u},
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			RootlyToken: "test-token",
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	// Create 5 alerts to verify all are sent in the payload.
	var alerts []*types.Alert
	for i := range 5 {
		alerts = append(alerts, &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": model.LabelValue("TestAlert" + string(rune('0'+i))),
					"severity":  "critical",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		})
	}

	retry, err := notifier.Notify(ctx, alerts...)
	require.NoError(t, err)
	require.False(t, retry)
	require.Equal(t, 5, receivedAlerts, "All alerts should be sent in the payload")
}
