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

package main

import (
	"fmt"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func TestExternalURL(t *testing.T) {
	hostname := "foo"
	for _, tc := range []struct {
		hostnameResolver func() (string, error)
		external         string
		listen           string

		expURL string
		err    bool
	}{
		{
			listen: ":9093",
			expURL: "http://" + hostname + ":9093",
		},
		{
			listen: "localhost:9093",
			expURL: "http://" + hostname + ":9093",
		},
		{
			listen: "localhost:",
			expURL: "http://" + hostname + ":",
		},
		{
			external: "https://host.example.com",
			expURL:   "https://host.example.com",
		},
		{
			external: "https://host.example.com/",
			expURL:   "https://host.example.com",
		},
		{
			external: "http://host.example.com/alertmanager",
			expURL:   "http://host.example.com/alertmanager",
		},
		{
			external: "http://host.example.com/alertmanager/",
			expURL:   "http://host.example.com/alertmanager",
		},
		{
			external: "http://host.example.com/////alertmanager//",
			expURL:   "http://host.example.com/////alertmanager",
		},
		{
			err: true,
		},
		{
			hostnameResolver: func() (string, error) { return "", fmt.Errorf("some error") },
			err:              true,
		},
		{
			external: "://broken url string",
			err:      true,
		},
		{
			external: "host.example.com:8080",
			err:      true,
		},
	} {
		tc := tc
		if tc.hostnameResolver == nil {
			tc.hostnameResolver = func() (string, error) {
				return hostname, nil
			}
		}
		t.Run(fmt.Sprintf("external=%q,listen=%q", tc.external, tc.listen), func(t *testing.T) {
			u, err := extURL(promslog.NewNopLogger(), tc.hostnameResolver, tc.listen, tc.external)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expURL, u.String())
		})
	}
}
