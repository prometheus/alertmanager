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
	"net"
	"testing"

	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/require"
)

// freePort binds an ephemeral port, records its address, then releases it
// so callers can reuse the (now free) address. There is an inherent race
// between releasing and rebinding, but it is acceptable for tests.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func tcpFlags(addrs ...string) *web.FlagConfig {
	a := addrs
	systemd := false
	cfg := ""
	return &web.FlagConfig{
		WebListenAddresses: &a,
		WebSystemdSocket:   &systemd,
		WebConfigFile:      &cfg,
	}
}

func TestListenAll_NoListeners(t *testing.T) {
	// Neither systemd activation nor any listen address: should surface
	// the toolkit's sentinel error rather than binding anything.
	empty := []string{}
	_, err := listenAll(&web.FlagConfig{WebListenAddresses: &empty})
	require.ErrorIs(t, err, web.ErrNoListeners)

	_, err = listenAll(&web.FlagConfig{})
	require.ErrorIs(t, err, web.ErrNoListeners)
}

func TestListenAll_MultipleTCP(t *testing.T) {
	listeners, err := listenAll(tcpFlags("127.0.0.1:0", "127.0.0.1:0"))
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	})

	require.Len(t, listeners, 2)
	require.NotEqual(t, listeners[0].Addr().String(), listeners[1].Addr().String(),
		"each listener should bind a distinct ephemeral port")
}

func TestListenAll_PartialBindIsCleanedUp(t *testing.T) {
	// Occupy a port so the *second* address fails to bind, forcing
	// listenAll to roll back the first (successfully bound) listener.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	busyAddr := occupied.Addr().String()

	firstAddr := freePort(t)

	listeners, err := listenAll(tcpFlags(firstAddr, busyAddr))
	require.Error(t, err)
	require.Nil(t, listeners)

	// The first listener must have been closed on the failure path, so
	// its address is bindable again.
	l, err := net.Listen("tcp", firstAddr)
	require.NoError(t, err, "first listener should have been closed during rollback")
	require.NoError(t, l.Close())
}

func TestParseVsockPort(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address string
		want    uint32
		wantErr bool
	}{
		{name: "valid", address: "vsock://:1234", want: 1234},
		{name: "valid high port", address: "vsock://:65535", want: 65535},
		{name: "missing port", address: "vsock://", wantErr: true},
		{name: "non-numeric port", address: "vsock://:abc", wantErr: true},
		{name: "port overflows uint32", address: "vsock://:4294967296", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVsockPort(tc.address)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
