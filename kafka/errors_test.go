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
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeNetErr implements net.Error to exercise the network/timeout paths
// of ClassifyError.
type fakeNetErr struct {
	timeout bool
}

func (f fakeNetErr) Error() string   { return "fake net error" }
func (f fakeNetErr) Timeout() bool   { return f.timeout }
func (f fakeNetErr) Temporary() bool { return false }

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ErrorCategoryNone},
		{"context.Canceled", context.Canceled, ErrorCategoryTimeout},
		{"context.DeadlineExceeded", context.DeadlineExceeded, ErrorCategoryTimeout},
		{"net.Error timeout", fakeNetErr{timeout: true}, ErrorCategoryTimeout},
		{"net.Error non-timeout", fakeNetErr{timeout: false}, ErrorCategoryNetwork},
		{"broker keyword", errors.New("broker unavailable"), ErrorCategoryBroker},
		{"kafka: keyword", errors.New("kafka: UNKNOWN_TOPIC_OR_PARTITION"), ErrorCategoryBroker},
		{"unknown", errors.New("something else entirely"), ErrorCategoryUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ClassifyError(tc.err))
		})
	}
}

// Make sure wrapped net.Errors are still classified correctly via errors.As.
func TestClassifyError_WrappedNetError(t *testing.T) {
	wrapped := &net.OpError{Op: "dial", Err: fakeNetErr{timeout: true}}
	require.Equal(t, ErrorCategoryTimeout, ClassifyError(wrapped))
}
