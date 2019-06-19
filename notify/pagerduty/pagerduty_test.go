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
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestPagerDutyRetryV1(t *testing.T) {
	notifier := new(Notifier)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusForbidden)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		resp := &http.Response{
			StatusCode: statusCode,
		}
		actual, _ := notifier.retryV1(resp)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv1 - error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV2(t *testing.T) {
	notifier := new(Notifier)

	retryCodes := append(test.DefaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range test.RetryTests(retryCodes) {
		resp := &http.Response{
			StatusCode: statusCode,
		}
		actual, _ := notifier.retryV2(resp)
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

func TestPagerDutyErr(t *testing.T) {
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

			exp: "unexpected status code: 400",
		},
		{
			status: http.StatusBadRequest,
			body:   nil,

			exp: "unexpected status code: 400",
		},
		{
			status: http.StatusTooManyRequests,
			body:   bytes.NewBuffer([]byte("")),

			exp: "unexpected status code: 429",
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			err := pagerDutyErr(tc.status, tc.body)
			require.Contains(t, err.Error(), tc.exp)
		})
	}
}
