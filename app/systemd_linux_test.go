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
	"bytes"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

// systemdSocket creates a notification socket and isolates the systemd environment.
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

// pauseSystemdSocket makes sends fail until the returned function restores the socket.
func pauseSystemdSocket(t *testing.T, conn *net.UnixConn) func() {
	t.Helper()
	path := conn.LocalAddr().String()
	require.NoError(t, os.Rename(path, path+"-paused"))
	resume := sync.OnceFunc(func() { require.NoError(t, os.Rename(path+"-paused", path)) })
	t.Cleanup(resume)
	return resume
}

// readSystemdMessage reads the next non-ignored datagram with a bounded wait.
func readSystemdMessage(t *testing.T, conn *net.UnixConn, ignored ...string) string {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 2048)
	for {
		n, err := conn.Read(buf)
		require.NoError(t, err)
		if message := string(buf[:n]); !slices.Contains(ignored, message) {
			return message
		}
	}
}

// expectNoSystemdMessage checks that an inactive notifier sends no datagrams.
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

// TestSystemdNotifierLifecycle checks status fields and ignores late lifecycle calls.
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
		{
			err:    errors.New(strings.Repeat("x", maxStatusErrLen+1)),
			status: "Configuration reload failed: " + strings.Repeat("x", maxStatusErrLen-len("…")) + "… (previous configuration remains active)",
		},
		{status: "Configuration reloaded"},
	} {
		n.Reloading()
		require.Regexp(t, `^RELOADING=1\nMONOTONIC_USEC=[0-9]+$`, readSystemdMessage(t, conn))
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

// TestSystemdNotifierWatchdog checks that watchdog sends stop before shutdown completes.
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

// TestSystemdNotifierDisabledWatchdog checks invalid intervals and other process IDs.
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

// TestSystemdNotifierWithoutSocket checks that notifications require an initial socket.
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

// TestSystemdNotifierStopBeforeReady checks that late readiness cannot start a worker.
func TestSystemdNotifierStopBeforeReady(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("WATCHDOG_USEC", "10000")
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)
	n.Stopping()
	n.Ready()
	expectNoSystemdMessage(t, conn)
}

// TestSystemdNotifierSendFailure checks that send failures are logged.
func TestSystemdNotifierSendFailure(t *testing.T) {
	conn := systemdSocket(t)
	t.Setenv("NOTIFY_SOCKET", conn.LocalAddr().String()+"-missing")
	var logs bytes.Buffer
	n := NewSystemdNotifier(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(n.Stopping)
	n.Ready()
	n.Stopping()
	require.Contains(t, logs.String(), "Failed to notify systemd")
}

// TestSystemdNotifierRetriesReadiness checks recovery and cancellation after a failed send.
func TestSystemdNotifierRetriesReadiness(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run("stop="+strconv.FormatBool(stop), func(t *testing.T) {
			path := systemdSocket(t).LocalAddr().String() + "-retry"
			t.Setenv("NOTIFY_SOCKET", path)
			n := NewSystemdNotifier(promslog.NewNopLogger())
			t.Cleanup(n.Stopping)
			n.Ready()
			if stop {
				n.Stopping()
			}
			conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			if !stop {
				require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
				n.Ready()
				expectNoSystemdMessage(t, conn)
				n.Stopping()
				require.Equal(t, "STOPPING=1", readSystemdMessage(t, conn))
			}
			select {
			case <-n.workerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("readiness retry worker did not exit")
			}
			n.Ready()
			expectNoSystemdMessage(t, conn)
		})
	}
}

// TestSystemdNotifierRetriesReloadCompletion checks repeated failures and heartbeats during retries.
func TestSystemdNotifierRetriesReloadCompletion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		watchdog string
		err      error
		status   string
	}{
		{name: "without watchdog", status: "Configuration reloaded"},
		{
			name:     "with watchdog",
			watchdog: "100000",
			err:      errors.New("bad\nconfig"),
			status:   "Configuration reload failed: bad config (previous configuration remains active)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := systemdSocket(t)
			t.Setenv("WATCHDOG_USEC", tc.watchdog)
			n := NewSystemdNotifier(promslog.NewNopLogger())
			t.Cleanup(n.Stopping)
			failures := make(chan struct{}, 2)
			allowCompletion := make(chan struct{})
			allowHeartbeat := make(chan struct{})
			gates := map[string]<-chan struct{}{
				"STATUS=" + tc.status + "\nREADY=1": allowCompletion,
				"WATCHDOG=1":                        allowHeartbeat,
			}
			blocked := errors.New("notification blocked by test")
			n.send = func(socket, state string) error {
				var err error
				if gate := gates[state]; tc.watchdog != "" && gate != nil {
					select {
					case <-gate:
					default:
						err = blocked
					}
				}
				if err == nil {
					err = sendSystemdNotification(socket, state)
				}
				if strings.HasPrefix(state, "STATUS=Configuration ") && err != nil {
					select {
					case failures <- struct{}{}:
					default:
					}
				}
				return err
			}
			n.Ready()
			require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
			n.Reloading()
			require.Regexp(t, `^RELOADING=1\nMONOTONIC_USEC=[0-9]+$`, readSystemdMessage(t, conn, "WATCHDOG=1"))

			resume := pauseSystemdSocket(t, conn)
			n.Reloaded(tc.err)
			if tc.watchdog != "" {
				resume()
			}
			// Observe the synchronous failure and a background failure before recovery.
			for range 2 {
				select {
				case <-failures:
				case <-time.After(5 * time.Second):
					t.Fatal("reload completion did not retry while delivery was blocked")
				}
			}
			if tc.watchdog == "" {
				resume()
			} else {
				// No heartbeat was sent earlier, so this one must arrive during the pending completion.
				close(allowHeartbeat)
				require.Equal(t, "WATCHDOG=1", readSystemdMessage(t, conn))
				close(allowCompletion)
			}
			require.Equal(t, "STATUS="+tc.status+"\nREADY=1", readSystemdMessage(t, conn, "WATCHDOG=1"))
			if tc.watchdog != "" {
				require.Equal(t, "WATCHDOG=1", readSystemdMessage(t, conn))
			}
			n.Stopping()
			require.Equal(t, "STOPPING=1", readSystemdMessage(t, conn, "WATCHDOG=1"))
			expectNoSystemdMessage(t, conn)
		})
	}
}

// TestSystemdNotifierReloadRetryOrdering rejects stale completions during reload and shutdown.
func TestSystemdNotifierReloadRetryOrdering(t *testing.T) {
	conn := systemdSocket(t)
	n := NewSystemdNotifier(promslog.NewNopLogger())
	t.Cleanup(n.Stopping)
	n.Ready()
	require.Equal(t, "STATUS=Ready\nREADY=1", readSystemdMessage(t, conn))
	n.Reloading()
	require.Regexp(t, `^RELOADING=1\nMONOTONIC_USEC=[0-9]+$`, readSystemdMessage(t, conn))

	resume := pauseSystemdSocket(t, conn)
	n.Reloaded(errors.New("old reload error"))
	// Start another reload before notification delivery recovers.
	n.Reloading()
	resume()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*systemdReadyRetryInterval)))
	_, err := conn.Read(make([]byte, 2048))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	n.Reloaded(nil)
	require.Equal(t, "STATUS=Configuration reloaded\nREADY=1", readSystemdMessage(t, conn))

	n.Reloading()
	require.Regexp(t, `^RELOADING=1\nMONOTONIC_USEC=[0-9]+$`, readSystemdMessage(t, conn))
	resume = pauseSystemdSocket(t, conn)
	n.Reloaded(nil)
	n.Stopping()
	resume()
	select {
	case <-n.workerDone:
	default:
		t.Fatal("notification worker still running after shutdown")
	}
	n.Reloaded(nil)
	expectNoSystemdMessage(t, conn)
}

// TestSystemdNotifyTimeout checks that a full receive queue cannot block a send indefinitely.
func TestSystemdNotifyTimeout(t *testing.T) {
	expectFullQueue := func(err error) {
		t.Helper()
		// BSD systems can return ENOBUFS without waiting for the deadline.
		if !errors.Is(err, syscall.ENOBUFS) {
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
		}
	}
	conn := systemdSocket(t)
	sender, err := net.DialUnix("unixgram", nil, conn.LocalAddr().(*net.UnixAddr))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sender.Close()) })
	require.NoError(t, sender.SetWriteDeadline(time.Now().Add(systemdNotifyTimeout)))
	for err == nil {
		_, err = sender.Write([]byte("full"))
	}
	expectFullQueue(err)

	done := make(chan error, 1)
	go func() { done <- sendSystemdNotification(conn.LocalAddr().String(), "READY=1") }()
	select {
	case err := <-done:
		expectFullQueue(err)
	case <-time.After(5 * time.Second):
		t.Fatal("notification send exceeded its deadline")
	}
}

// TestSanitizeStatusLength checks the byte limit and complete UTF-8 characters.
func TestSanitizeStatusLength(t *testing.T) {
	t.Parallel()
	atLimit := strings.Repeat("x", maxStatusErrLen)
	require.Equal(t, atLimit, sanitizeStatus(atLimit))
	prefix := strings.Repeat("\u00E9", (maxStatusErrLen-len("…"))/2)
	require.Equal(t, prefix+"…", sanitizeStatus(prefix+"\u754C\u754C"))
}
