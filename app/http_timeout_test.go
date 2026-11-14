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

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApp_HTTPIdleTimeoutOutlastsClient(t *testing.T) {
	// Like the other app lifecycle tests, run sequentially because constructing
	// the API router mutates global go-openapi state.
	opts := testOptions(t)
	opts.WebConfig.WebListenAddresses = &[]string{"127.0.0.1:0", "127.0.0.1:0"}
	a, err := New(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, a.Stop(context.Background())) })

	// prometheus/common keeps client connections idle for five minutes.
	// The server must let the client close first, avoiding a race between
	// closing an idle connection and a non-retryable alert POST reusing it.
	const clientIdleTimeout = 5 * time.Minute
	require.Len(t, a.servers, 2)
	for _, server := range a.servers {
		require.Greater(t, server.IdleTimeout, clientIdleTimeout,
			"server idle timeout must outlast the Prometheus client idle timeout")
	}
}
