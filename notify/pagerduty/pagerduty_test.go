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

package pagerduty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestPagerDutyRetryV1(t *testing.T) {
	notifier, err := New(
		&config.PagerdutyConfig{
			ServiceKey: config.Secret("01234567890123456789012345678901"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusForbidden)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv1 - error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV2(t *testing.T) {
	notifier, err := New(
		&config.PagerdutyConfig{
			RoutingKey: config.Secret("01234567890123456789012345678901"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv2 - error on status %d", statusCode))
	}
}

func TestPagerDutyRedactedURLV1(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	key := "01234567890123456789012345678901"
	notifier, err := New(
		&config.PagerdutyConfig{
			ServiceKey: config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	notifier.apiV1 = u.String()

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, key)
}

func TestPagerDutyRedactedURLV2(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	key := "01234567890123456789012345678901"
	notifier, err := New(
		&config.PagerdutyConfig{
			URL:        &config.URL{URL: u},
			RoutingKey: config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, key)
}

func TestPagerDutyTemplating(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		out := make(map[string]interface{})
		err := dec.Decode(&out)
		if err != nil {
			panic(err)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	for _, tc := range []struct {
		title string
		cfg   *config.PagerdutyConfig

		retry  bool
		errMsg string
	}{
		{
			title: "full-blown message",
			cfg: &config.PagerdutyConfig{
				RoutingKey: config.Secret("01234567890123456789012345678901"),
				Images: []config.PagerdutyImage{
					{
						Src:  "{{ .Status }}",
						Alt:  "{{ .Status }}",
						Href: "{{ .Status }}",
					},
				},
				Links: []config.PagerdutyLink{
					{
						Href: "{{ .Status }}",
						Text: "{{ .Status }}",
					},
				},
				Details: map[string]string{
					"firing":       `{{ template "pagerduty.default.instances" .Alerts.Firing }}`,
					"resolved":     `{{ template "pagerduty.default.instances" .Alerts.Resolved }}`,
					"num_firing":   `{{ .Alerts.Firing | len }}`,
					"num_resolved": `{{ .Alerts.Resolved | len }}`,
				},
			},
		},
		{
			title: "details with templating errors",
			cfg: &config.PagerdutyConfig{
				RoutingKey: config.Secret("01234567890123456789012345678901"),
				Details: map[string]string{
					"firing":       `{{ template "pagerduty.default.instances" .Alerts.Firing`,
					"resolved":     `{{ template "pagerduty.default.instances" .Alerts.Resolved }}`,
					"num_firing":   `{{ .Alerts.Firing | len }}`,
					"num_resolved": `{{ .Alerts.Resolved | len }}`,
				},
			},
			errMsg: "failed to template",
		},
		{
			title: "v2 message with templating errors",
			cfg: &config.PagerdutyConfig{
				RoutingKey: config.Secret("01234567890123456789012345678901"),
				Severity:   "{{ ",
			},
			errMsg: "failed to template",
		},
		{
			title: "v1 message with templating errors",
			cfg: &config.PagerdutyConfig{
				ServiceKey: config.Secret("01234567890123456789012345678901"),
				Client:     "{{ ",
			},
			errMsg: "failed to template",
		},
		{
			title: "routing key cannot be empty",
			cfg: &config.PagerdutyConfig{
				RoutingKey: config.Secret(`{{ "" }}`),
			},
			errMsg: "routing key cannot be empty",
		},
		{
			title: "service_key cannot be empty",
			cfg: &config.PagerdutyConfig{
				ServiceKey: config.Secret(`{{ "" }}`),
			},
			errMsg: "service key cannot be empty",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tc.cfg.URL = &config.URL{URL: u}
			tc.cfg.HTTPConfig = &commoncfg.HTTPClientConfig{}
			pd, err := New(tc.cfg, test.CreateTmpl(t), log.NewNopLogger())
			require.NoError(t, err)
			if pd.apiV1 != "" {
				pd.apiV1 = u.String()
			}

			ctx := context.Background()
			ctx = notify.WithGroupKey(ctx, "1")

			ok, err := pd.Notify(ctx, []*types.Alert{
				&types.Alert{
					Alert: model.Alert{
						Labels: model.LabelSet{
							"lbl1": "val1",
						},
						StartsAt: time.Now(),
						EndsAt:   time.Now().Add(time.Hour),
					},
				},
			}...)
			if tc.errMsg == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errMsg)
			}
			require.Equal(t, tc.retry, ok)
		})
	}
}

func TestErrDetails(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   io.Reader

		exp string
	}{
		{
			status: http.StatusBadRequest,
			body: bytes.NewBuffer([]byte(
				`{"status":"invalid event","message":"Event object is invalid","errors":["Length of 'routing_key' is incorrect (should be 32 characters)"]}`,
			)),

			exp: "Length of 'routing_key' is incorrect",
		},
		{
			status: http.StatusBadRequest,
			body:   bytes.NewBuffer([]byte(`{"status"}`)),

			exp: "",
		},
		{
			status: http.StatusBadRequest,

			exp: "",
		},
		{
			status: http.StatusTooManyRequests,

			exp: "",
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			err := errDetails(tc.status, tc.body)
			require.Contains(t, err, tc.exp)
		})
	}
}

func TestEventSizeEnforcement(t *testing.T) {
	bigDetails := map[string]string{
		"firing": strings.Repeat("a", 513000),
	}

	// V1 Messages
	msgV1 := &pagerDutyMessage{
		ServiceKey: "01234567890123456789012345678901",
		EventType:  "trigger",
		Details:    bigDetails,
	}

	notifierV1, err := New(
		&config.PagerdutyConfig{
			ServiceKey: config.Secret("01234567890123456789012345678901"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	encodedV1, err := notifierV1.encodeMessage(msgV1)
	require.NoError(t, err)
	require.Contains(t, encodedV1.String(), `"details":{"error":"Custom details have been removed because the original event exceeds the maximum size of 512KB"}`)

	// V2 Messages
	msgV2 := &pagerDutyMessage{
		RoutingKey:  "01234567890123456789012345678901",
		EventAction: "trigger",
		Payload: &pagerDutyPayload{
			CustomDetails: bigDetails,
		},
	}

	notifierV2, err := New(
		&config.PagerdutyConfig{
			RoutingKey: config.Secret("01234567890123456789012345678901"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	encodedV2, err := notifierV2.encodeMessage(msgV2)
	require.NoError(t, err)
	require.Contains(t, encodedV2.String(), `"custom_details":{"error":"Custom details have been removed because the original event exceeds the maximum size of 512KB"}`)
}
