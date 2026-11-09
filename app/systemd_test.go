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

//go:build unix

package app

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func systemdSocket(t *testing.T) *net.UnixConn {
	t.Helper()
	// t.TempDir includes the test name, which can exceed the Unix socket path limit.
	dir, err := os.MkdirTemp("", "am-sd-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	path := filepath.Join(dir, "notify")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	t.Setenv("NOTIFY_SOCKET", path)
	t.Setenv("WATCHDOG_USEC", "")
	t.Setenv("WATCHDOG_PID", "")
	return conn
}

func readSystemdMessage(t *testing.T, conn *net.UnixConn) string {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	return string(buf[:n])
}

func expectNoSystemdMessage(t *testing.T, conn *net.UnixConn) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	require.Error(t, err, "unexpected notification: %s", buf[:n])
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout())
}

func TestSystemdNotifierLifecycle(t *testing.T) {
	conn := systemdSocket(t)
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)

	n.Reloading()
	n.Reloaded(nil)
	n.Ready()
	require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
	n.Ready()

	for _, tc := range []struct {
		err    error
		status string
	}{
		{
			err:    errors.New("line1\nREADY=0\r\nline2\x00\xff"),
			status: "Configuration reload failed: line1 READY=0 line2 \uFFFD (previous configuration remains active)",
		},
		{status: "Configuration reloaded"},
	} {
		n.Reloading()
		require.Regexp(t, `^RELOADING=1\nMONOTONIC_USEC=[0-9]+\n$`, readSystemdMessage(t, conn))
		n.Reloaded(tc.err)
		require.Equal(t, "STATUS="+tc.status+"\nREADY=1", readSystemdMessage(t, conn))
	}

	n.Stopping()
	require.Equal(t, "STOPPING=1", readSystemdMessage(t, conn))
	n.Ready()
	n.Reloading()
	n.Reloaded(nil)
	n.Stopping()
	expectNoSystemdMessage(t, conn)
}

func TestSystemdNotifierWatchdog(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("WATCHDOG_USEC", "100000")
	t.Setenv("WATCHDOG_PID", strconv.Itoa(os.Getpid()))
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)
	n.Ready()
	require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
	require.Equal(t, "WATCHDOG=1", readSystemdMessage(t, conn))

	n.Stopping()
	for {
		message := readSystemdMessage(t, conn)
		if message == "STOPPING=1" {
			break
		}
		require.Equal(t, "WATCHDOG=1", message)
	}
	n.Ready()
	expectNoSystemdMessage(t, conn)
}

func TestSystemdNotifierDisabledWatchdog(t *testing.T) {
	for _, tc := range []struct {
		name string
		usec string
		pid  string
		warn bool
	}{
		{name: "other process", usec: "10000", pid: strconv.Itoa(os.Getpid() + 1)},
		{name: "invalid interval", usec: "invalid", warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := systemdSocket(t)
			t.Setenv("WATCHDOG_USEC", tc.usec)
			t.Setenv("WATCHDOG_PID", tc.pid)
			var logs bytes.Buffer
			n := NewSystemdNotifier(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(n.Stopping)
			n.Ready()
			require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
			expectNoSystemdMessage(t, conn)
			if tc.warn {
				require.Contains(t, logs.String(), "Failed to read systemd watchdog configuration")
			} else {
				require.Empty(t, logs.String())
			}
		})
	}
}

func TestSystemdNotifierWithoutSocket(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("NOTIFY_SOCKET", "")
	t.Setenv("WATCHDOG_USEC", "10000")
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)
	t.Setenv("NOTIFY_SOCKET", conn.LocalAddr().String())
	n.Ready()
	n.Reloading()
	n.Reloaded(nil)
	n.Stopping()
	expectNoSystemdMessage(t, conn)
}

func TestSystemdNotifierStopBeforeReady(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("WATCHDOG_USEC", "10000")
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)
	n.Stopping()
	n.Ready()
	expectNoSystemdMessage(t, conn)
}

func TestSystemdNotifierSendFailure(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("NOTIFY_SOCKET", conn.LocalAddr().String()+"-missing")
	var logs bytes.Buffer
	n := NewSystemdNotifier(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(n.Stopping)
	n.Ready()
	require.Contains(t, logs.String(), "Failed to notify systemd")
}

func TestSanitizeStatusLength(t *testing.T) {
	t.Parallel()
	status := strings.Repeat("\u00E9", 127)
	require.Equal(t, status, sanitizeStatus(status+"\u754C"))
}
