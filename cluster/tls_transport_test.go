// Copyright 2020 The Prometheus Authors
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
	"bufio"
	"bytes"
	context2 "context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

var logger = promslog.NewNopLogger()

func freeport() int {
	lis, _ := net.Listen(network, "127.0.0.1:0")
	defer lis.Close()

	return lis.Addr().(*net.TCPAddr).Port
}

func newTLSTransport(file, address string, port int) (*TLSTransport, error) {
	cfg, err := GetTLSTransportConfig(file)
	if err != nil {
		return nil, err
	}

	return NewTLSTransport(context2.Background(), promslog.NewNopLogger(), prometheus.NewRegistry(), address, port, cfg)
}

func TestNewTLSTransport(t *testing.T) {
	port := freeport()
	for _, tc := range []struct {
		bindAddr    string
		bindPort    int
		tlsConfFile string
		err         string
	}{
		{
			err: "must specify TLSTransportConfig",
		},
		{
			tlsConfFile: "testdata/empty_tls_config.yml",
			err:         "missing 'tls_server_config' entry in the TLS configuration",
		},
		{
			tlsConfFile: "testdata/tls_config_with_missing_server.yml",
			err:         "missing 'tls_server_config' entry in the TLS configuration",
		},
		{
			err:         "invalid bind address \"\"",
			tlsConfFile: "testdata/tls_config_node1.yml",
		},
		{
			bindAddr:    "abc123",
			err:         "invalid bind address \"abc123\"",
			tlsConfFile: "testdata/tls_config_node1.yml",
		},
		{
			bindAddr:    localhost,
			bindPort:    0,
			tlsConfFile: "testdata/tls_config_node1.yml",
		},
		{
			bindAddr:    localhost,
			bindPort:    port,
			tlsConfFile: "testdata/tls_config_node2.yml",
		},
		{
			tlsConfFile: "testdata/tls_config_with_missing_client.yml",
			bindAddr:    localhost,
		},
	} {
		t.Run("", func(t *testing.T) {
			transport, err := newTLSTransport(tc.tlsConfFile, tc.bindAddr, tc.bindPort)
			if len(tc.err) > 0 {
				require.Error(t, err)
				require.Equal(t, tc.err, err.Error())
				return
			}

			defer transport.Shutdown()

			require.NoError(t, err)
			require.Equal(t, tc.bindAddr, transport.bindAddr)
			require.Equal(t, tc.bindPort, transport.bindPort)
			require.NotNil(t, transport.listener)
		})
	}
}

const localhost = "127.0.0.1"

func TestFinalAdvertiseAddr(t *testing.T) {
	ports := [...]int{freeport(), freeport(), freeport()}
	testCases := []struct {
		bindAddr      string
		bindPort      int
		inputIP       string
		inputPort     int
		expectedIP    string
		expectedPort  int
		expectedError string
	}{
		{bindAddr: localhost, bindPort: ports[0], inputIP: "10.0.0.5", inputPort: 54231, expectedIP: "10.0.0.5", expectedPort: 54231},
		{bindAddr: localhost, bindPort: ports[1], inputIP: "invalid", inputPort: 54231, expectedError: "failed to parse advertise address \"invalid\""},
		{bindAddr: "0.0.0.0", bindPort: 0, inputIP: "", inputPort: 0, expectedIP: "random"},
		{bindAddr: localhost, bindPort: 0, inputIP: "", inputPort: 0, expectedIP: localhost},
		{bindAddr: localhost, bindPort: ports[2], inputIP: "", inputPort: 0, expectedIP: localhost, expectedPort: ports[2]},
	}
	for _, tc := range testCases {
		tlsConf := loadTLSTransportConfig(t, "testdata/tls_config_node1.yml")
		transport, err := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), tc.bindAddr, tc.bindPort, tlsConf)
		require.NoError(t, err)
		ip, port, err := transport.FinalAdvertiseAddr(tc.inputIP, tc.inputPort)
		if len(tc.expectedError) > 0 {
			require.Equal(t, tc.expectedError, err.Error())
		} else {
			require.NoError(t, err)
			if tc.expectedPort == 0 {
				require.Less(t, tc.expectedPort, port)
			} else {
				require.Equal(t, tc.expectedPort, port)
			}
			if tc.expectedIP == "random" {
				require.NotNil(t, ip)
			} else {
				require.Equal(t, tc.expectedIP, ip.String())
			}
		}
		transport.Shutdown()
	}
}

func TestWriteTo(t *testing.T) {
	tlsConf1 := loadTLSTransportConfig(t, "testdata/tls_config_node1.yml")
	t1, _ := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf1)
	defer t1.Shutdown()

	tlsConf2 := loadTLSTransportConfig(t, "testdata/tls_config_node2.yml")
	t2, _ := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf2)
	defer t2.Shutdown()

	from := fmt.Sprintf("%s:%d", t1.bindAddr, t1.GetAutoBindPort())
	to := fmt.Sprintf("%s:%d", t2.bindAddr, t2.GetAutoBindPort())
	sent := []byte(("test packet"))
	_, err := t1.WriteTo(sent, to)
	require.NoError(t, err)
	packet := <-t2.PacketCh()
	require.Equal(t, sent, packet.Buf)
	require.Equal(t, from, packet.From.String())
}

func BenchmarkWriteTo(b *testing.B) {
	tlsConf1 := loadTLSTransportConfig(b, "testdata/tls_config_node1.yml")
	t1, _ := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf1)
	defer t1.Shutdown()

	tlsConf2 := loadTLSTransportConfig(b, "testdata/tls_config_node2.yml")
	t2, _ := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf2)
	defer t2.Shutdown()

	b.ResetTimer()
	from := fmt.Sprintf("%s:%d", t1.bindAddr, t1.GetAutoBindPort())
	to := fmt.Sprintf("%s:%d", t2.bindAddr, t2.GetAutoBindPort())
	sent := []byte(("test packet"))

	_, err := t1.WriteTo(sent, to)
	require.NoError(b, err)
	packet := <-t2.PacketCh()

	require.Equal(b, sent, packet.Buf)
	require.Equal(b, from, packet.From.String())
}

func TestDialTimeout(t *testing.T) {
	tlsConf1 := loadTLSTransportConfig(t, "testdata/tls_config_node1.yml")
	t1, err := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf1)
	require.NoError(t, err)
	defer t1.Shutdown()

	tlsConf2 := loadTLSTransportConfig(t, "testdata/tls_config_node2.yml")
	t2, err := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf2)
	require.NoError(t, err)
	defer t2.Shutdown()

	addr := fmt.Sprintf("%s:%d", t2.bindAddr, t2.GetAutoBindPort())
	from, err := t1.DialTimeout(addr, 5*time.Second)
	require.NoError(t, err)
	defer from.Close()

	var to net.Conn
	var wg sync.WaitGroup
	wg.Go(func() {
		to = <-t2.StreamCh()
	})

	sent := []byte(("test stream"))
	m, err := from.Write(sent)
	require.NoError(t, err)
	require.Positive(t, m)

	wg.Wait()

	reader := bufio.NewReader(to)
	buf := make([]byte, len(sent))
	n, err := io.ReadFull(reader, buf)
	require.NoError(t, err)
	require.Len(t, sent, n)
	require.Equal(t, sent, buf)
}

func TestShutdown(t *testing.T) {
	var buf bytes.Buffer
	promslogConfig := &promslog.Config{Writer: &buf}
	logger := promslog.New(promslogConfig)
	// Set logger to debug, otherwise it won't catch some logging from `Shutdown()` method.
	_ = promslogConfig.Level.Set("debug")

	tlsConf1 := loadTLSTransportConfig(t, "testdata/tls_config_node1.yml")
	t1, _ := NewTLSTransport(context2.Background(), logger, prometheus.NewRegistry(), "127.0.0.1", 0, tlsConf1)
	// Sleeping to make sure listeners have started and can subsequently be shut down gracefully.
	time.Sleep(500 * time.Millisecond)
	err := t1.Shutdown()
	require.NoError(t, err)
	require.NotContains(t, buf.String(), "use of closed network connection")
	require.Contains(t, buf.String(), "shutting down tls transport")
}

func loadTLSTransportConfig(tb testing.TB, filename string) *TLSTransportConfig {
	tb.Helper()

	config, err := GetTLSTransportConfig(filename)
	if err != nil {
		tb.Fatal(err)
	}

	return config
}
