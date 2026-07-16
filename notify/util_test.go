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

package notify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/template"
)

func TestTruncate(t *testing.T) {
	type expect struct {
		out   string
		trunc bool
	}

	testCases := []struct {
		in string
		n  int

		runes expect
		bytes expect
	}{
		{
			in:    "",
			n:     5,
			runes: expect{out: "", trunc: false},
			bytes: expect{out: "", trunc: false},
		},
		{
			in:    "abcde",
			n:     2,
			runes: expect{out: "ab", trunc: true},
			bytes: expect{out: "..", trunc: true},
		},
		{
			in:    "abcde",
			n:     4,
			runes: expect{out: "abc…", trunc: true},
			bytes: expect{out: "a…", trunc: true},
		},
		{
			in:    "abcde",
			n:     5,
			runes: expect{out: "abcde", trunc: false},
			bytes: expect{out: "abcde", trunc: false},
		},
		{
			in:    "abcdefgh",
			n:     5,
			runes: expect{out: "abcd…", trunc: true},
			bytes: expect{out: "ab…", trunc: true},
		},
		{
			in:    "a⌘cde",
			n:     5,
			runes: expect{out: "a⌘cde", trunc: false},
			bytes: expect{out: "a…", trunc: true},
		},
		{
			in:    "a⌘cdef",
			n:     5,
			runes: expect{out: "a⌘cd…", trunc: true},
			bytes: expect{out: "a…", trunc: true},
		},
		{
			in:    "世界cdef",
			n:     3,
			runes: expect{out: "世界c", trunc: true},
			bytes: expect{out: "…", trunc: true},
		},
		{
			in:    "❤️✅🚀🔥❌❤️✅🚀🔥❌❤️✅🚀🔥❌❤️✅🚀🔥❌",
			n:     19,
			runes: expect{out: "❤️✅🚀🔥❌❤️✅🚀🔥❌❤️✅🚀🔥❌…", trunc: true},
			bytes: expect{out: "❤️✅🚀…", trunc: true},
		},
	}

	type truncateFunc func(string, int) (string, bool)

	for _, tc := range testCases {
		for _, fn := range []truncateFunc{TruncateInBytes, TruncateInRunes} {
			var truncated bool
			var out string

			fnPath := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
			fnName := path.Base(fnPath)
			switch fnName {
			case "notify.TruncateInRunes":
				truncated = tc.runes.trunc
				out = tc.runes.out
			case "notify.TruncateInBytes":
				truncated = tc.bytes.trunc
				out = tc.bytes.out
			default:
				t.Fatalf("unknown function")
			}

			t.Run(fmt.Sprintf("%s(%s,%d)", fnName, tc.in, tc.n), func(t *testing.T) {
				s, trunc := fn(tc.in, tc.n)
				require.Equal(t, out, s)
				require.Equal(t, truncated, trunc)
			})
		}
	}
}

type brokenReader struct{}

func (b brokenReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("some error")
}

func TestRetrierCheck(t *testing.T) {
	for _, tc := range []struct {
		retrier Retrier
		status  int
		body    io.Reader

		retry       bool
		expectedErr string
	}{
		{
			retrier: Retrier{},
			status:  http.StatusOK,
			body:    bytes.NewBuffer([]byte("ok")),

			retry: false,
		},
		{
			retrier: Retrier{},
			status:  http.StatusNoContent,

			retry: false,
		},
		{
			retrier: Retrier{},
			status:  http.StatusBadRequest,

			retry:       false,
			expectedErr: "unexpected status code 400",
		},
		{
			retrier: Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
			status:  http.StatusBadRequest,
			body:    bytes.NewBuffer([]byte("invalid request")),

			retry:       false,
			expectedErr: "unexpected status code 400: invalid request",
		},
		{
			retrier: Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
			status:  http.StatusTooManyRequests,

			retry:       true,
			expectedErr: "unexpected status code 429",
		},
		{
			retrier: Retrier{},
			status:  http.StatusServiceUnavailable,
			body:    bytes.NewBuffer([]byte("retry later")),

			retry:       true,
			expectedErr: "unexpected status code 503: retry later",
		},
		{
			retrier: Retrier{},
			status:  http.StatusBadGateway,
			body:    &brokenReader{},

			retry:       true,
			expectedErr: "unexpected status code 502",
		},
		{
			retrier: Retrier{CustomDetailsFunc: func(status int, b io.Reader) string {
				if status != http.StatusServiceUnavailable {
					return "invalid"
				}
				bs, _ := io.ReadAll(b)
				return fmt.Sprintf("server response is %q", string(bs))
			}},
			status: http.StatusServiceUnavailable,
			body:   bytes.NewBuffer([]byte("retry later")),

			retry:       true,
			expectedErr: "unexpected status code 503: server response is \"retry later\"",
		},
	} {
		t.Run("", func(t *testing.T) {
			retry, err := tc.retrier.Check(tc.status, tc.body)
			require.Equal(t, tc.retry, retry)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestGetTemplateDataWithRouteLabels(t *testing.T) {
	tmpl, err := template.New()
	require.NoError(t, err)
	tmpl.ExternalURL = &url.URL{Scheme: "http", Host: "example.com"}

	// A route label value containing template metacharacters: the dispatcher
	// has already rendered route labels, so GetTemplateData must mark them
	// rendered and the routeLabels function must return them verbatim rather
	// than executing them a second time.
	ctx := context.Background()
	ctx = WithReceiverName(ctx, "test-receiver")
	ctx = WithGroupKey(ctx, "test-key")
	ctx = WithGroupLabels(ctx, model.LabelSet{"alertname": "Test"})
	ctx = WithNotificationReason(ctx, ReasonFirstNotification)
	ctx = WithRouteLabels(ctx, model.LabelSet{
		"team": "ops",
		"desc": "value is {{ $value }}",
	})

	data := GetTemplateData(ctx, tmpl, nil, promslog.NewNopLogger())

	require.Equal(t, "ops", data.RouteLabels["team"])
	require.Equal(t, "value is {{ $value }}", data.RouteLabels["desc"])

	// The routeLabels template function returns the values verbatim.
	got, err := tmpl.ExecuteTextString(`{{ routeLabels "team" }}|{{ routeLabels "desc" }}`, data)
	require.NoError(t, err)
	require.Equal(t, "ops|value is {{ $value }}", got)
}

func TestGetFailureReasonFromStatusCode(t *testing.T) {
	for _, tc := range []struct {
		statusCode int
		expected   Reason
	}{
		{http.StatusUnauthorized, AuthErrorReason},
		{http.StatusForbidden, AuthErrorReason},
		{http.StatusTooManyRequests, RateLimitedReason},
		{http.StatusBadRequest, ClientErrorReason},
		{http.StatusNotFound, ClientErrorReason},
		{http.StatusInternalServerError, ServerErrorReason},
		{http.StatusServiceUnavailable, ServerErrorReason},
		{http.StatusOK, DefaultReason},
		{http.StatusMovedPermanently, DefaultReason},
	} {
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			require.Equal(t, tc.expected, GetFailureReasonFromStatusCode(tc.statusCode))
		})
	}
}

func TestCheckResponse(t *testing.T) {
	for _, tc := range []struct {
		name        string
		retrier     Retrier
		response    *http.Response
		retry       bool
		expectedErr string
		reason      Reason
	}{
		{
			name: "2xx success",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
			},
			retry: false,
		},
		{
			name: "204 no content",
			response: &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(bytes.NewBuffer(nil)),
			},
			retry: false,
		},
		{
			name: "400 bad request",
			response: &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewBufferString("invalid request")),
			},
			retry:       false,
			expectedErr: "unexpected status code 400: invalid request",
			reason:      ClientErrorReason,
		},
		{
			name: "401 unauthorized",
			response: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBuffer(nil)),
			},
			retry:       false,
			expectedErr: "unexpected status code 401",
			reason:      AuthErrorReason,
		},
		{
			name: "429 without Retry-After",
			response: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("too many requests")),
			},
			retry:       true,
			expectedErr: "unexpected status code 429: too many requests",
			reason:      RateLimitedReason,
		},
		{
			name:    "429 in RetryCodes uses RateLimitedReason",
			retrier: Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
			response: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("too many requests")),
			},
			retry:       true,
			expectedErr: "unexpected status code 429: too many requests",
			reason:      RateLimitedReason,
		},
		{
			name: "503 service unavailable",
			response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewBufferString("retry later")),
			},
			retry:       true,
			expectedErr: "unexpected status code 503: retry later",
			reason:      ServerErrorReason,
		},
		{
			name: "502 bad gateway with broken body",
			response: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(&brokenReader{}),
			},
			retry:       true,
			expectedErr: "unexpected status code 502",
			reason:      ServerErrorReason,
		},
		{
			name:    "non-retryable code in RetryCodes (e.g. 409)",
			retrier: Retrier{RetryCodes: []int{http.StatusConflict}},
			response: &http.Response{
				StatusCode: http.StatusConflict,
				Body:       io.NopCloser(bytes.NewBufferString("conflict")),
			},
			retry:       true,
			expectedErr: "unexpected status code 409: conflict",
			reason:      ClientErrorReason,
		},
		{
			name:        "nil response",
			response:    nil,
			retry:       false,
			expectedErr: "nil HTTP response",
			reason:      DefaultReason,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retry, err := tc.retrier.CheckResponse(tc.response)
			require.Equal(t, tc.retry, retry)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectedErr)
			var e *ErrorWithReason
			require.ErrorAs(t, err, &e)
			require.Equal(t, tc.reason, e.Reason)
		})
	}
}

func TestCheckResponseRetryAfterPropagation(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		retryAfterHeader      string
		useHTTPDate           bool
		expectedRetryAfter    time.Duration
		expectExactRetryAfter bool
	}{
		{
			name:                  "integer seconds",
			retryAfterHeader:      "7",
			expectedRetryAfter:    7 * time.Second,
			expectExactRetryAfter: true,
		},
		{
			name:                  "zero seconds is treated as no Retry-After",
			retryAfterHeader:      "0",
			expectedRetryAfter:    0,
			expectExactRetryAfter: true,
		},
		{
			name:               "HTTP-date in the future",
			useHTTPDate:        true,
			expectedRetryAfter: 2 * time.Second,
		},
		{
			name:                  "absent header means zero RetryAfter",
			retryAfterHeader:      "",
			expectedRetryAfter:    0,
			expectExactRetryAfter: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := make(http.Header)
			if tc.useHTTPDate {
				header.Set("Retry-After", time.Now().Add(tc.expectedRetryAfter).UTC().Format(http.TimeFormat))
			} else if tc.retryAfterHeader != "" {
				header.Set("Retry-After", tc.retryAfterHeader)
			}

			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     header,
				Body:       io.NopCloser(bytes.NewBufferString("too many requests")),
			}

			retry, err := (&Retrier{}).CheckResponse(resp)
			require.True(t, retry)
			require.Error(t, err)

			var e *ErrorWithReason
			require.ErrorAs(t, err, &e)
			require.Equal(t, RateLimitedReason, e.Reason)

			if tc.expectExactRetryAfter {
				require.Equal(t, tc.expectedRetryAfter, e.RetryAfter)
				return
			}
			// HTTP-date parsing depends on wall-clock timing; assert a positive
			// value close to the requested duration.
			require.Greater(t, e.RetryAfter, time.Duration(0))
			require.InDelta(t, tc.expectedRetryAfter.Seconds(), e.RetryAfter.Seconds(), 1.0)
		})
	}
}
