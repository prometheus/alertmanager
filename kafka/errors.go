// Copyright The Prometheus Authors
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

package kafka

import (
	"context"
	"errors"
	"net"
	"strings"
)

// Error categories returned by ClassifyError.  These string values
// are intended for use as bounded-cardinality Prometheus label values.
const (
	ErrorCategoryNone    = "none"
	ErrorCategoryTimeout = "timeout"
	ErrorCategoryNetwork = "network"
	ErrorCategoryBroker  = "broker"
	ErrorCategoryUnknown = "unknown"
)

// ClassifyError buckets a franz-go error into one of the
// ErrorCategory* constants.  It keeps Prometheus metric label
// cardinality bounded regardless of the specific error string franz-go
// surfaces.
func ClassifyError(err error) string {
	if err == nil {
		return ErrorCategoryNone
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorCategoryTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorCategoryTimeout
		}
		return ErrorCategoryNetwork
	}
	// franz-go surfaces broker-side errors typed as kerr.Error; we
	// detect them via the error string so this package doesn't have
	// to depend on kerr just for the type assertion.
	msg := err.Error()
	if strings.Contains(msg, "broker") || strings.Contains(msg, "kafka:") {
		return ErrorCategoryBroker
	}
	return ErrorCategoryUnknown
}
