// Copyright 2018 Prometheus Team
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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// getContextWithCancelingURL returns a context that gets canceled when a
// client does a GET request to the returned URL.
// Handlers passed to the function will be invoked in order before the context gets canceled.
func getContextWithCancelingURL(h ...func(w http.ResponseWriter, r *http.Request)) (context.Context, *url.URL, func()) {
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	i := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if i < len(h) {
			h[i](w, r)
		} else {
			cancel()
			<-done
		}
		i++
	}))

	// No need to check the error since httptest.NewServer always return a valid URL.
	u, _ := url.Parse(srv.URL)

	return ctx, u, func() {
		close(done)
		srv.Close()
	}
}

// assertNotifyLeaksNoSecret calls the Notify() method of the notifier, expects
// it to fail because the context is canceled by the server and checks that no
// secret data is leaked in the error message returned by Notify().
func assertNotifyLeaksNoSecret(t *testing.T, ctx context.Context, n Notifier, secret ...string) {
	t.Helper()
	require.NotEmpty(t, secret)

	ctx = WithGroupKey(ctx, "1")
	ok, err := n.Notify(ctx, []*types.Alert{
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

	require.Error(t, err)
	require.Contains(t, err.Error(), context.Canceled.Error())
	for _, s := range secret {
		require.NotContains(t, err.Error(), s)
	}
	require.True(t, ok)
}

func TestBuildReceiverIntegrations(t *testing.T) {
	for _, tc := range []struct {
		receiver *config.Receiver
		err      bool
		exp      []Integration
	}{
		{
			receiver: &config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
					},
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
						NotifierConfig: config.NotifierConfig{
							VSendResolved: true,
						},
					},
				},
			},
			exp: []Integration{
				Integration{
					name: "webhook",
					idx:  0,
					rs:   sendResolved(false),
				},
				Integration{
					name: "webhook",
					idx:  1,
					rs:   sendResolved(true),
				},
			},
		},
		{
			receiver: &config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{
							TLSConfig: commoncfg.TLSConfig{
								CAFile: "not_existing",
							},
						},
					},
				},
			},
			err: true,
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			integrations, err := BuildReceiverIntegrations(tc.receiver, nil, nil)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, integrations, len(tc.exp))
			for i := range tc.exp {
				require.Equal(t, tc.exp[i].SendResolved(), integrations[i].SendResolved())
				require.Equal(t, tc.exp[i].Name(), integrations[i].Name())
				require.Equal(t, tc.exp[i].Index(), integrations[i].Index())
			}
		})
	}
}

func TestWebhookRetry(t *testing.T) {
	u, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	notifier := &Webhook{conf: &config.WebhookConfig{URL: &config.URL{URL: u}}}
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV1(t *testing.T) {
	notifier := new(PagerDuty)

	retryCodes := append(defaultRetryCodes(), http.StatusForbidden)
	for statusCode, expected := range retryTests(retryCodes) {
		resp := &http.Response{
			StatusCode: statusCode,
		}
		actual, _ := notifier.retryV1(resp)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv1 - error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV2(t *testing.T) {
	notifier := new(PagerDuty)

	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		resp := &http.Response{
			StatusCode: statusCode,
		}
		actual, _ := notifier.retryV2(resp)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv2 - error on status %d", statusCode))
	}
}

func TestPagerDutyRedactedURLV1(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	key := "01234567890123456789012345678901"
	notifier, err := NewPagerDuty(
		&config.PagerdutyConfig{
			ServiceKey: config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	notifier.apiV1 = u.String()

	assertNotifyLeaksNoSecret(t, ctx, notifier, key)
}

func TestPagerDutyRedactedURLV2(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	key := "01234567890123456789012345678901"
	notifier, err := NewPagerDuty(
		&config.PagerdutyConfig{
			URL:        &config.URL{URL: u},
			RoutingKey: config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, key)
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

func TestSlackRetry(t *testing.T) {
	notifier := new(Slack)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestSlackErr(t *testing.T) {
	notifier := new(Slack)
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

func TestSlackRedactedURL(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	notifier, err := NewSlack(
		&config.SlackConfig{
			APIURL:     &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, u.String())
}

func TestHipchatRetry(t *testing.T) {
	notifier := new(Hipchat)
	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestHipchatRedactedURL(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	token := "secret_token"
	notifier, err := NewHipchat(
		&config.HipchatConfig{
			APIURL:     &config.URL{URL: u},
			AuthToken:  config.Secret(token),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, token)
}

func TestOpsGenieRetry(t *testing.T) {
	notifier := new(OpsGenie)

	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestOpsGenieRedactedURL(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	key := "key"
	notifier, err := NewOpsGenie(
		&config.OpsGenieConfig{
			APIURL:     &config.URL{URL: u},
			APIKey:     config.Secret(key),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, key)
}

func TestVictorOpsRetry(t *testing.T) {
	notifier := new(VictorOps)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestVictorOpsRedactedURL(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := NewVictorOps(
		&config.VictorOpsConfig{
			APIURL:     &config.URL{URL: u},
			APIKey:     config.Secret(secret),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, secret)
}

func TestPushoverRetry(t *testing.T) {
	notifier := new(Pushover)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestPushoverRedactedURL(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	key, token := "user_key", "token"
	notifier, err := NewPushover(
		&config.PushoverConfig{
			UserKey:    config.Secret(key),
			Token:      config.Secret(token),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	notifier.apiURL = u.String()

	assertNotifyLeaksNoSecret(t, ctx, notifier, key, token)
}

func retryTests(retryCodes []int) map[int]bool {
	tests := map[int]bool{
		// 1xx
		http.StatusContinue:           false,
		http.StatusSwitchingProtocols: false,
		http.StatusProcessing:         false,

		// 2xx
		http.StatusOK:                   false,
		http.StatusCreated:              false,
		http.StatusAccepted:             false,
		http.StatusNonAuthoritativeInfo: false,
		http.StatusNoContent:            false,
		http.StatusResetContent:         false,
		http.StatusPartialContent:       false,
		http.StatusMultiStatus:          false,
		http.StatusAlreadyReported:      false,
		http.StatusIMUsed:               false,

		// 3xx
		http.StatusMultipleChoices:   false,
		http.StatusMovedPermanently:  false,
		http.StatusFound:             false,
		http.StatusSeeOther:          false,
		http.StatusNotModified:       false,
		http.StatusUseProxy:          false,
		http.StatusTemporaryRedirect: false,
		http.StatusPermanentRedirect: false,

		// 4xx
		http.StatusBadRequest:                   false,
		http.StatusUnauthorized:                 false,
		http.StatusPaymentRequired:              false,
		http.StatusForbidden:                    false,
		http.StatusNotFound:                     false,
		http.StatusMethodNotAllowed:             false,
		http.StatusNotAcceptable:                false,
		http.StatusProxyAuthRequired:            false,
		http.StatusRequestTimeout:               false,
		http.StatusConflict:                     false,
		http.StatusGone:                         false,
		http.StatusLengthRequired:               false,
		http.StatusPreconditionFailed:           false,
		http.StatusRequestEntityTooLarge:        false,
		http.StatusRequestURITooLong:            false,
		http.StatusUnsupportedMediaType:         false,
		http.StatusRequestedRangeNotSatisfiable: false,
		http.StatusExpectationFailed:            false,
		http.StatusTeapot:                       false,
		http.StatusUnprocessableEntity:          false,
		http.StatusLocked:                       false,
		http.StatusFailedDependency:             false,
		http.StatusUpgradeRequired:              false,
		http.StatusPreconditionRequired:         false,
		http.StatusTooManyRequests:              false,
		http.StatusRequestHeaderFieldsTooLarge:  false,
		http.StatusUnavailableForLegalReasons:   false,

		// 5xx
		http.StatusInternalServerError:           false,
		http.StatusNotImplemented:                false,
		http.StatusBadGateway:                    false,
		http.StatusServiceUnavailable:            false,
		http.StatusGatewayTimeout:                false,
		http.StatusHTTPVersionNotSupported:       false,
		http.StatusVariantAlsoNegotiates:         false,
		http.StatusInsufficientStorage:           false,
		http.StatusLoopDetected:                  false,
		http.StatusNotExtended:                   false,
		http.StatusNetworkAuthenticationRequired: false,
	}

	for _, statusCode := range retryCodes {
		tests[statusCode] = true
	}

	return tests
}

func defaultRetryCodes() []int {
	return []int{
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusHTTPVersionNotSupported,
		http.StatusVariantAlsoNegotiates,
		http.StatusInsufficientStorage,
		http.StatusLoopDetected,
		http.StatusNotExtended,
		http.StatusNetworkAuthenticationRequired,
	}
}

func createTmpl(t *testing.T) *template.Template {
	tmpl, err := template.FromGlobs()
	require.NoError(t, err)
	tmpl.ExternalURL, _ = url.Parse("http://am")
	return tmpl
}

func readBody(t *testing.T, r *http.Request) string {
	body, err := ioutil.ReadAll(r.Body)
	require.NoError(t, err)
	return string(body)
}

func TestOpsGenie(t *testing.T) {
	u, err := url.Parse("https://opsgenie/api")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	logger := log.NewNopLogger()
	tmpl := createTmpl(t)
	conf := &config.OpsGenieConfig{
		NotifierConfig: config.NotifierConfig{
			VSendResolved: true,
		},
		Message:     `{{ .CommonLabels.Message }}`,
		Description: `{{ .CommonLabels.Description }}`,
		Source:      `{{ .CommonLabels.Source }}`,
		Responders: []config.OpsGenieConfigResponder{
			{
				Name: `{{ .CommonLabels.ResponderName1 }}`,
				Type: `{{ .CommonLabels.ResponderType1 }}`,
			},
			{
				Name: `{{ .CommonLabels.ResponderName2 }}`,
				Type: `{{ .CommonLabels.ResponderType2 }}`,
			},
		},
		Tags:       `{{ .CommonLabels.Tags }}`,
		Note:       `{{ .CommonLabels.Note }}`,
		Priority:   `{{ .CommonLabels.Priority }}`,
		APIKey:     `{{ .ExternalURL }}`,
		APIURL:     &config.URL{URL: u},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
	}
	notifier, err := NewOpsGenie(conf, tmpl, logger)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "1")

	expectedURL, _ := url.Parse("https://opsgenie/apiv2/alerts")

	// Empty alert.
	alert1 := &types.Alert{
		Alert: model.Alert{
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}
	expectedBody := `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"","details":{},"source":""}
`
	req, retry, err := notifier.createRequest(ctx, alert1)
	require.NoError(t, err)
	require.Equal(t, true, retry)
	require.Equal(t, expectedURL, req.URL)
	require.Equal(t, "GenieKey http://am", req.Header.Get("Authorization"))
	require.Equal(t, expectedBody, readBody(t, req))

	// Fully defined alert.
	alert2 := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"Message":        "message",
				"Description":    "description",
				"Source":         "http://prometheus",
				"ResponderName1": "TeamA",
				"ResponderType1": "team",
				"ResponderName2": "EscalationA",
				"ResponderType2": "escalation",
				"Tags":           "tag1,tag2",
				"Note":           "this is a note",
				"Priority":       "P1",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}
	expectedBody = `{"alias":"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b","message":"message","description":"description","details":{},"source":"http://prometheus","responders":[{"name":"TeamA","type":"team"},{"name":"EscalationA","type":"escalation"}],"tags":["tag1","tag2"],"note":"this is a note","priority":"P1"}
`
	req, retry, err = notifier.createRequest(ctx, alert2)
	require.NoError(t, err)
	require.Equal(t, true, retry)
	require.Equal(t, expectedBody, readBody(t, req))

	// Broken API Key Template.
	conf.APIKey = "{{ kaput "
	_, _, err = notifier.createRequest(ctx, alert2)
	require.Error(t, err)
	require.Equal(t, err.Error(), "templating error: template: :1: function \"kaput\" not defined")
}

func TestVictorOpsCustomFields(t *testing.T) {
	logger := log.NewNopLogger()
	tmpl := createTmpl(t)

	url, err := url.Parse("http://nowhere.com")

	require.NoError(t, err, "unexpected error parsing mock url")

	conf := &config.VictorOpsConfig{
		APIKey:            `12345`,
		APIURL:            &config.URL{URL: url},
		EntityDisplayName: `{{ .CommonLabels.Message }}`,
		StateMessage:      `{{ .CommonLabels.Message }}`,
		RoutingKey:        `test`,
		MessageType:       ``,
		MonitoringTool:    `AM`,
		CustomFields: map[string]string{
			"Field_A": "{{ .CommonLabels.Message }}",
		},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
	}

	notifier, err := NewVictorOps(conf, tmpl, logger)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "1")

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"Message": "message",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	msg, err := notifier.createVictorOpsPayload(ctx, alert)
	require.NoError(t, err)

	var m map[string]string
	err = json.Unmarshal(msg.Bytes(), &m)

	require.NoError(t, err)

	// Verify that a custom field was added to the payload and templatized.
	require.Equal(t, "message", m["Field_A"])
}

func TestWechatRedactedURLOnInitialAuthentication(t *testing.T) {
	ctx, u, fn := getContextWithCancelingURL()
	defer fn()

	secret := "secret_key"
	notifier, err := NewWechat(
		&config.WechatConfig{
			APIURL:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  config.Secret(secret),
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, secret)
}

func TestWechatRedactedURLOnNotify(t *testing.T) {
	secret, token := "secret", "token"
	ctx, u, fn := getContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":"%s"}`, token)
	})
	defer fn()

	notifier, err := NewWechat(
		&config.WechatConfig{
			APIURL:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			CorpID:     "corpid",
			APISecret:  config.Secret(secret),
		},
		createTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assertNotifyLeaksNoSecret(t, ctx, notifier, secret, token)
}

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
			s, trunc := truncate(tc.in, tc.n)
			require.Equal(t, tc.trunc, trunc)
			require.Equal(t, tc.out, s)
		})
	}
}
