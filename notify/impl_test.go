package notify

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebhookRetry(t *testing.T) {
	notifier := new(Webhook)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV1(t *testing.T) {
	notifier := new(PagerDuty)

	retryCodes := append(defaultRetryCodes(), http.StatusForbidden)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retryV1(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv1 - error on status %d", statusCode))
	}
}

func TestPagerDutyRetryV2(t *testing.T) {
	notifier := new(PagerDuty)

	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retryV2(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("retryv2 - error on status %d", statusCode))
	}
}

func TestSlackRetry(t *testing.T) {
	notifier := new(Slack)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestHipchatRetry(t *testing.T) {
	notifier := new(Hipchat)
	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestWechatRetry(t *testing.T) {
	notifier := new(Wechat)
	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}
func TestOpsGenieRetry(t *testing.T) {
	notifier := new(OpsGenie)

	retryCodes := append(defaultRetryCodes(), http.StatusTooManyRequests)
	for statusCode, expected := range retryTests(retryCodes) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestVictorOpsRetry(t *testing.T) {
	notifier := new(VictorOps)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestPushoverRetry(t *testing.T) {
	notifier := new(Pushover)
	for statusCode, expected := range retryTests(defaultRetryCodes()) {
		actual, _ := notifier.retry(statusCode)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
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
