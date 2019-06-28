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

package msteams

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

func TestMsTeamsRetry(t *testing.T) {
	notifier := new(Notifier)
	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestMsTeamsErr(t *testing.T) {
	notifier := new(Notifier)
	for _, tc := range []struct {
		status   int
		body     io.Reader
		expected string
	}{
		{
			status:   http.StatusBadRequest,
			body:     nil,
			expected: "unexpected status code 400",
		},
		{
			status:   http.StatusBadRequest,
			body:     bytes.NewBuffer([]byte("invalid_payload")),
			expected: "unexpected status code 400: \"invalid_payload\"",
		},
		{
			status:   http.StatusNotFound,
			body:     bytes.NewBuffer([]byte("channel_not_found")),
			expected: "unexpected status code 404: \"channel_not_found\"",
		},
		{
			status:   http.StatusInternalServerError,
			body:     bytes.NewBuffer([]byte("rollup_error")),
			expected: "unexpected status code 500: \"rollup_error\"",
		},
	} {
		t.Run("", func(t *testing.T) {
			_, err := notifier.retry(tc.status, tc.body)
			require.Contains(t, err.Error(), tc.expected)
		})
	}
}

func TestMsTeamsRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.MsTeamsConfig{
			APIURL:     &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, u.String())
}
