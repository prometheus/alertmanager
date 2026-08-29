// Copyright 2021 Prometheus Team
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

package sns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/sigv4"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var logger = promslog.NewNopLogger()

func TestValidateAndTruncateMessage(t *testing.T) {
	sBuff := make([]byte, 257*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err := validateAndTruncateMessage(string(sBuff), 256*1024)
	require.True(t, isTruncated)
	require.NoError(t, err)
	require.NotEqual(t, sBuff, truncatedMessage)
	require.Len(t, truncatedMessage, 256*1024)

	sBuff = make([]byte, 100)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err = validateAndTruncateMessage(string(sBuff), 100)
	require.False(t, isTruncated)
	require.NoError(t, err)
	require.Equal(t, string(sBuff), truncatedMessage)

	invalidUtf8String := "\xc3\x28"
	_, _, err = validateAndTruncateMessage(invalidUtf8String, 100)
	require.Error(t, err)
}

func TestNotifyWithInvalidTemplate(t *testing.T) {
	for _, tc := range []struct {
		title     string
		errMsg    string
		updateCfg func(*SNSConfig)
	}{
		{
			title:  "with invalid Attribute template",
			errMsg: "execute 'attributes' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.Attributes = map[string]string{
					"attribName1": "{{ template \"unknown_template\" . }}",
				}
			},
		},
		{
			title:  "with invalid TopicArn template",
			errMsg: "execute 'topic_arn' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.TopicARN = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid PhoneNumber template",
			errMsg: "execute 'phone_number' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.PhoneNumber = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid Message template",
			errMsg: "execute 'message' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.Message = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid Subject template",
			errMsg: "execute 'subject' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.Subject = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid APIUrl template",
			errMsg: "execute 'api_url' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.APIUrl = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid TargetARN template",
			errMsg: "execute 'target_arn' template",
			updateCfg: func(cfg *SNSConfig) {
				cfg.TargetARN = "{{ template \"unknown_template\" . }}"
			},
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			snsCfg := &SNSConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				TopicARN:   "TestTopic",
				Sigv4: sigv4.SigV4Config{
					Region:     "us-west-2",
					RoleARN:    "my:role/arn",
					ExternalID: "external_id",
				},
			}
			if tc.updateCfg != nil {
				tc.updateCfg(snsCfg)
			}
			notifier, err := New(
				snsCfg,
				createTmpl(t),
				logger,
			)
			require.NoError(t, err)
			var alerts []*types.Alert
			_, err = notifier.Notify(context.Background(), alerts...)
			require.Error(t, err)
			require.Contains(t, err.Error(), "template \"unknown_template\" not defined")
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

// CreateTmpl returns a ready-to-use template.
func createTmpl(t *testing.T) *template.Template {
	tmpl, err := template.FromGlobs([]string{})
	require.NoError(t, err)
	tmpl.ExternalURL, _ = url.Parse("http://am")
	return tmpl
}

// sdkErr creates errors faithful to what the SDK would create.
func sdkErr(statusCode int, requestID string, deserErr error) error {
	var err error = &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: statusCode}},
			Err:      deserErr,
		},
		RequestID: requestID,
	}
	if statusCode >= 500 {
		err = &retry.MaxAttemptsError{Attempt: 1, Err: err}
	}
	return &smithy.OperationError{
		ServiceID:     "SNS",
		OperationName: "Publish",
		Err:           err,
	}
}

func TestClassifyClientError(t *testing.T) {
	for _, tc := range []struct {
		title string
		err   error

		retry  bool
		reason notify.Reason
		errMsg string
	}{
		{
			title:  "client build, client error",
			err:    sdkErr(http.StatusBadRequest, "req-1", &snstypes.InvalidParameterException{Message: aws.String("bogus")}),
			retry:  false,
			reason: notify.DefaultReason,
			errMsg: "unexpected status code 400",
		},
		{
			title:  "client build, server error",
			err:    sdkErr(http.StatusInternalServerError, "req-1", &snstypes.InternalErrorException{Message: aws.String("bogus")}),
			retry:  true,
			reason: notify.DefaultReason,
			errMsg: "unexpected status code 500",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			classifyNotifier := &Notifier{retrier: &notify.Retrier{}}
			retry, err := classifyNotifier.classifyClientError(tc.err)
			require.Error(t, err)
			require.Equal(t, tc.retry, retry)
			reason := notify.DefaultReason
			if e, ok := errors.AsType[*notify.ErrorWithReason](err); ok {
				reason = e.Reason
			}
			require.Equal(t, tc.reason, reason)
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestClassifyPublishError(t *testing.T) {
	for _, tc := range []struct {
		title string
		err   error

		retry  bool
		reason notify.Reason
		errMsg string
	}{
		{
			title:  "publish, client error",
			err:    sdkErr(http.StatusBadRequest, "req-1", &snstypes.InvalidParameterException{Message: aws.String("bogus")}),
			retry:  false,
			reason: notify.ClientErrorReason,
			errMsg: "unexpected status code 400",
		},
		{
			title:  "publish, auth error",
			err:    sdkErr(http.StatusForbidden, "req-1", &snstypes.AuthorizationErrorException{Message: aws.String("bogus")}),
			retry:  false,
			reason: notify.AuthErrorReason,
			errMsg: "unexpected status code 403",
		},
		{
			// Surprisingly, the Publish deserializer has no case for the
			// "Throttled" error code (unlike e.g. PublishBatch), so it never
			// builds snstypes.ThrottledException.
			title:  "publish, rate limited",
			err:    sdkErr(http.StatusTooManyRequests, "req-1", &smithy.GenericAPIError{Code: "Throttled", Message: "bogus"}),
			retry:  false,
			reason: notify.RateLimitedReason,
			errMsg: "unexpected status code 429",
		},
		{
			title:  "publish, server error",
			err:    sdkErr(http.StatusInternalServerError, "req-1", &snstypes.InternalErrorException{Message: aws.String("bogus")}),
			retry:  true,
			reason: notify.ServerErrorReason,
			errMsg: "unexpected status code 500",
		},
		{
			// The AWS HTTP client refuses to follow 301/302, and the deserializer
			// builds a GenericAPIError from the empty redirect body.
			title:  "publish, redirect status",
			err:    sdkErr(http.StatusMovedPermanently, "", &smithy.GenericAPIError{Code: "UnknownError", Message: "UnknownError"}),
			retry:  false,
			reason: notify.DefaultReason,
			errMsg: "unexpected status code 301",
		},
		{
			// The SDK only builds an APIError for a non-2xx response, so a 2xx
			// carries a DeserializationError instead and lands here.
			title: "publish, no API error in the chain",
			err: sdkErr(http.StatusOK, "", &smithy.DeserializationError{
				Err: fmt.Errorf("failed to decode response body, %w", errors.New("PublishResult node not found")),
			}),
			retry:  true,
			reason: notify.DefaultReason,
			errMsg: "deserialization failed",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			classifyNotifier := &Notifier{retrier: &notify.Retrier{}}
			retry, err := classifyNotifier.classifyPublishError(tc.err)
			require.Error(t, err)
			require.Equal(t, tc.retry, retry)
			reason := notify.DefaultReason
			if e, ok := errors.AsType[*notify.ErrorWithReason](err); ok {
				reason = e.Reason
			}
			require.Equal(t, tc.reason, reason)
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}
