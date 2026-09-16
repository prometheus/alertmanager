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

func TestTmplTextWithLogger_LogsOnError(t *testing.T) {
	tmpl, err := template.FromGlobs([]string{})
	require.NoError(t, err)

	data := &template.Data{}
	var tmplErr error

	var buf bytes.Buffer
	logger := promslog.New(&promslog.Config{Writer: &buf})

	tmplFn := TmplTextWithLogger(tmpl, data, &tmplErr, logger)

	// An unclosed action causes a parse/execute error.
	result := tmplFn("{{ .Missing")

	require.Error(t, tmplErr)
	require.Empty(t, result)
	logOutput := buf.String()
	require.Contains(t, logOutput, "template execution failed")
	require.Contains(t, logOutput, "{{ .Missing")   // template name is logged
	require.Contains(t, logOutput, tmplErr.Error()) // rendering error is logged

	// Subsequent calls must short-circuit without logging again.
	buf.Reset()
	_ = tmplFn("{{ .AnotherField }}")
	require.Empty(t, buf.String(), "no second log expected after first error")
}

func TestTmplHTMLWithLogger_LogsOnError(t *testing.T) {
	tmpl, err := template.FromGlobs([]string{})
	require.NoError(t, err)

	data := &template.Data{}
	var tmplErr error

	var buf bytes.Buffer
	logger := promslog.New(&promslog.Config{Writer: &buf})

	tmplFn := TmplHTMLWithLogger(tmpl, data, &tmplErr, logger)

	result := tmplFn("{{ .Missing")

	require.Error(t, tmplErr)
	require.Empty(t, result)
	logOutput := buf.String()
	require.Contains(t, logOutput, "template execution failed")
	require.Contains(t, logOutput, "{{ .Missing")   // template name is logged
	require.Contains(t, logOutput, tmplErr.Error()) // rendering error is logged
}
