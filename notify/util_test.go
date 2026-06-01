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
	"fmt"
	"io"
	"net/http"
	"path"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		retrier  Retrier
		response *http.Response

		retry       bool
		expectedErr string
	}{
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("ok"))),
			},

			retry: false,
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(bytes.NewBuffer([]byte{})),
			},

			retry: false,
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusBadRequest,
			},

			retry:       false,
			expectedErr: "unexpected status code 400",
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("invalid request"))),
			},

			retry:       false,
			expectedErr: "unexpected status code 400: invalid request",
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("too many requests"))),
			},

			retry:       true,
			expectedErr: "unexpected status code 429: too many requests",
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("retry later"))),
			},

			retry:       true,
			expectedErr: "unexpected status code 503: retry later",
		},
		{
			retrier: Retrier{},
			response: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(&brokenReader{}),
			},

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
			response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("retry later"))),
			},

			retry:       true,
			expectedErr: "unexpected status code 503: server response is \"retry later\"",
		},
	} {
		t.Run("", func(t *testing.T) {
			retry, err := tc.retrier.Check(tc.response)
			require.Equal(t, tc.retry, retry)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestRetrierCheckTooManyRequestsRetryAfterPropagation(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		retrier               Retrier
		retryAfter            string
		expected              time.Duration
		useHTTPDate           bool
		expectExactRetryAfter bool
	}{
		{
			name:                  "retry-after seconds",
			retrier:               Retrier{},
			retryAfter:            "7",
			expected:              7 * time.Second,
			expectExactRetryAfter: true,
		},
		{
			name:                  "retry-after seconds with retry-codes including 429",
			retrier:               Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
			retryAfter:            "10",
			expected:              10 * time.Second,
			expectExactRetryAfter: true,
		},
		{
			name:    "retry-after http-date with retry-codes including 429",
			retrier: Retrier{RetryCodes: []int{http.StatusTooManyRequests}},
			// retryAfter is intentionally left empty; the HTTP-date value is
			// generated inside the subtest to avoid it being stale.
			retryAfter:  "",
			expected:    2 * time.Second,
			useHTTPDate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retryAfter := tc.retryAfter
			if tc.useHTTPDate {
				retryAfter = time.Now().Add(tc.expected).UTC().Format(http.TimeFormat)
			}

			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("too many requests")),
			}
			resp.Header.Set("Retry-After", retryAfter)

			retry, err := tc.retrier.Check(resp)
			require.True(t, retry)
			require.Error(t, err)

			var errWithReason *ErrorWithReason
			require.ErrorAs(t, err, &errWithReason)
			require.Equal(t, TooManyRequestsReason, errWithReason.Reason)

			if tc.expectExactRetryAfter {
				require.Equal(t, tc.expected, errWithReason.RetryAfter)
				return
			}

			// HTTP-date parsing depends on wall clock timing; assert we keep a positive value
			// and that it is close to what was requested.
			require.Greater(t, errWithReason.RetryAfter, time.Duration(0))
			require.InDelta(t, tc.expected.Seconds(), errWithReason.RetryAfter.Seconds(), 1.0)
		})
	}
}
