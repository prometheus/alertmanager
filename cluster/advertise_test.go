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

package cluster

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateAdvertiseAddress(t *testing.T) {
	old := getPrivateAddress
	defer func() {
		getPrivateAddress = old
	}()

	cases := []struct {
		name                   string
		privateIPFn            getIPFunc
		publicIPFn             getIPFunc
		bind, advertise        string
		allowInsecureAdvertise bool

		expectedIP net.IP
		err        bool
	}{
		{
			name:      "use provided bind address",
			bind:      "192.0.2.1",
			advertise: "",

			expectedIP: net.ParseIP("192.0.2.1"),
			err:        false,
		},
		{
			name:      "use provided advertise address",
			bind:      "192.0.2.1",
			advertise: "192.0.2.2",

			expectedIP: net.ParseIP("192.0.2.2"),
			err:        false,
		},
		{
			name:        "discover private ip address",
			privateIPFn: func() (string, error) { return "192.0.2.1", nil },
			bind:        "0.0.0.0",
			advertise:   "",

			expectedIP: net.ParseIP("192.0.2.1"),
			err:        false,
		},
		{
			name:        "error if getPrivateAddress errors",
			privateIPFn: func() (string, error) { return "", errors.New("some error") },
			bind:        "0.0.0.0",
			advertise:   "",

			err: true,
		},
		{
			name:        "error if getPrivateAddress returns an invalid address",
			privateIPFn: func() (string, error) { return "invalid", nil },
			bind:        "0.0.0.0",
			advertise:   "",

			err: true,
		},
		{
			name:        "error if getPrivateAddress returns an empty address",
			privateIPFn: func() (string, error) { return "", nil },
			bind:        "0.0.0.0",
			advertise:   "",

			err: true,
		},

		{
			name:                   "discover public advertise address",
			privateIPFn:            func() (string, error) { return "", nil },
			publicIPFn:             func() (string, error) { return "192.0.2.1", nil },
			bind:                   "0.0.0.0",
			advertise:              "",
			allowInsecureAdvertise: true,

			expectedIP: net.ParseIP("192.0.2.1"),
			err:        false,
		},
		{
			name:                   "error if getPublicAddress errors",
			privateIPFn:            func() (string, error) { return "", nil },
			publicIPFn:             func() (string, error) { return "", errors.New("some error") },
			bind:                   "0.0.0.0",
			advertise:              "",
			allowInsecureAdvertise: true,

			err: true,
		},
		{
			name:                   "error if getPublicAddress returns an invalid address",
			privateIPFn:            func() (string, error) { return "", nil },
			publicIPFn:             func() (string, error) { return "invalid", nil },
			bind:                   "0.0.0.0",
			advertise:              "",
			allowInsecureAdvertise: true,

			err: true,
		},
		{
			name:                   "error if getPublicAddress returns an empty address",
			privateIPFn:            func() (string, error) { return "", nil },
			publicIPFn:             func() (string, error) { return "", nil },
			bind:                   "0.0.0.0",
			advertise:              "",
			allowInsecureAdvertise: true,

			err: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getPrivateAddress = c.privateIPFn
			getPublicAddress = c.publicIPFn
			got, err := calculateAdvertiseAddress(c.bind, c.advertise, c.allowInsecureAdvertise)
			if c.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expectedIP.String(), got.String())
		})
	}
}
