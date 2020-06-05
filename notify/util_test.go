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
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncate(t *testing.T) {
	testCases := []struct {
		in string
		n  int

		out   string
		trunc bool
	}{
		{
			in:    "",
			n:     5,
			out:   "",
			trunc: false,
		},
		{
			in:    "abcde",
			n:     2,
			out:   "ab",
			trunc: true,
		},
		{
			in:    "abcde",
			n:     4,
			out:   "a...",
			trunc: true,
		},
		{
			in:    "abcde",
			n:     5,
			out:   "abcde",
			trunc: false,
		},
		{
			in:    "abcdefgh",
			n:     5,
			out:   "ab...",
			trunc: true,
		},
		{
			in:    "a⌘cde",
			n:     5,
			out:   "a⌘cde",
			trunc: false,
		},
		{
			in:    "a⌘cdef",
			n:     5,
			out:   "a⌘...",
			trunc: true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("truncate(%s,%d)", tc.in, tc.n), func(t *testing.T) {
			s, trunc := Truncate(tc.in, tc.n)
			require.Equal(t, tc.trunc, trunc)
			require.Equal(t, tc.out, s)
		})
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
				bs, _ := ioutil.ReadAll(b)
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
