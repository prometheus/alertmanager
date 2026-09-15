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
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/mdlayher/vsock"
	"github.com/prometheus/exporter-toolkit/web"
)

// listenAll eagerly binds every listener described by flags so that the
// bound addresses are known before serving begins (Addr/Addrs can then
// report concrete ports, including those chosen by the kernel for ":0").
//
// It mirrors the listener selection performed by
// exporter-toolkit/web.ListenAndServe so that the binary keeps support
// for systemd socket activation and vsock addresses after the extraction
// into this package. The resulting listeners are later served by
// web.ServeMultiple in Start.
func listenAll(flags *web.FlagConfig) ([]net.Listener, error) {
	if flags.WebSystemdSocket != nil && *flags.WebSystemdSocket {
		listeners, err := activation.Listeners()
		if err != nil {
			return nil, fmt.Errorf("alertmanager/app: systemd socket activation: %w", err)
		}
		if len(listeners) < 1 {
			return nil, errors.New("alertmanager/app: no socket activation file descriptors found")
		}
		return listeners, nil
	}
	if flags.WebListenAddresses == nil || len(*flags.WebListenAddresses) == 0 {
		return nil, web.ErrNoListeners
	}
	addrs := *flags.WebListenAddresses
	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		l, err := listenOne(addr)
		if err != nil {
			for _, prev := range listeners {
				_ = prev.Close()
			}
			return nil, fmt.Errorf("alertmanager/app: listen %q: %w", addr, err)
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// listenOne binds a single listener, honouring the "vsock://" scheme used
// by exporter-toolkit and falling back to a TCP listener otherwise.
func listenOne(address string) (net.Listener, error) {
	if strings.HasPrefix(address, "vsock://") {
		port, err := parseVsockPort(address)
		if err != nil {
			return nil, err
		}
		return vsock.Listen(port, nil)
	}
	return net.Listen("tcp", address)
}

// parseVsockPort extracts the port from a "vsock://:{port}" address. It
// matches the parsing in exporter-toolkit/web.
func parseVsockPort(address string) (uint32, error) {
	uri, err := url.Parse(address)
	if err != nil {
		return 0, err
	}
	_, portStr, err := net.SplitHostPort(uri.Host)
	if err != nil {
		return 0, err
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(port), nil
}
