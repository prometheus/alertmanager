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
	"net/http/httptest"
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

func TestPostJSON(t *testing.T) {
	tests := []struct {
		name            string
		headers         http.Header
		expectedHeaders http.Header
		data            []byte
	}{{
		name:    "No headers",
		headers: nil,
		expectedHeaders: http.Header{
			"Accept-Encoding": []string{"gzip"},
			"Content-Length":  []string{"13"},
			"Content-Type":    []string{"application/json"},
			"User-Agent":      []string{"Alertmanager/"},
		},
		data: []byte("Hello, world!"),
	}, {
		name: "With headers",
		headers: http.Header{
			"X-Test-PostJSON": []string{"true"},
		},
		expectedHeaders: http.Header{
			"Accept-Encoding": []string{"gzip"},
			"Content-Length":  []string{"13"},
			"Content-Type":    []string{"application/json"},
			"User-Agent":      []string{"Alertmanager/"},
			"X-Test-Postjson": []string{"true"},
		},
		data: []byte("Hello, world!"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				receivedHeaders http.Header
				receivedData    []byte
			)
			// Start an HTTP test server to record the headers and data from the request.
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header
				receivedData, _ = io.ReadAll(r.Body)
			}))
			defer s.Close()

			ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFunc()

			resp, err := PostJSON(ctx, http.DefaultClient, s.URL, test.headers, bytes.NewReader(test.data))
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)

			require.Equal(t, test.expectedHeaders, receivedHeaders)
			require.Equal(t, test.data, receivedData)
		})
	}
}
