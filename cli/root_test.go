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

package cli

import (
	"testing"

	"github.com/prometheus/cfaccess"
	promconfig "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
)

// TestNewAmtoolHTTPClient verifies standard and Cloudflare Access client configuration.
func TestNewAmtoolHTTPClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  promconfig.HTTPClientConfig
		wantErr string
	}{
		{
			name: "default configuration",
		},
		{
			name: "Cloudflare Access",
			config: promconfig.HTTPClientConfig{
				Authorization: &promconfig.Authorization{Type: cfaccess.AuthorizationType},
			},
		},
		{
			name: "Cloudflare Access with credentials",
			config: promconfig.HTTPClientConfig{
				Authorization: &promconfig.Authorization{Type: cfaccess.AuthorizationType, Credentials: "token"},
			},
			wantErr: `authorization credentials, credentials_file & credentials_ref must not be configured when authorization type is "cf-access"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := newAmtoolHTTPClient(tc.config)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.Nil(t, client)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
			require.NotNil(t, client.Transport)
		})
	}
}
