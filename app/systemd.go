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
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

const (
	systemdNotifyTimeout      = 100 * time.Millisecond
	systemdReadyRetryInterval = time.Second
	// The reload error limit includes the truncation marker.
	maxStatusErrLen = 1024
)

// SystemdNotifier sends lifecycle and watchdog notifications to systemd.
// Use NewSystemdNotifier to create one notifier per process.
type SystemdNotifier struct {
	logger           *slog.Logger
	socket           string
	watchdogInterval time.Duration

	mtx        sync.Mutex
	ready      bool
	stopped    bool
	workerStop chan struct{}
	workerDone chan struct{}
}

// NewSystemdNotifier enables notifications when NOTIFY_SOCKET is set.
// WATCHDOG_USEC and WATCHDOG_PID control watchdog notifications.
func NewSystemdNotifier(logger *slog.Logger) *SystemdNotifier {
	n := &SystemdNotifier{
		logger: logger,
		socket: os.Getenv("NOTIFY_SOCKET"),
	}
	if n.socket != "" {
		interval, err := daemon.SdWatchdogEnabled(false)
		if err != nil {
			logger.Warn("Failed to read systemd watchdog configuration", "err", err)
		} else {
			n.watchdogInterval = interval / 2
		}
	}
	return n
}

// Ready reports startup completion and retries failed sends until success or shutdown.
// The watchdog starts after the readiness notification succeeds.
func (n *SystemdNotifier) Ready() {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if n.socket == "" || n.ready || n.workerDone != nil || n.stopped {
		return
	}
	n.ready = n.notify("STATUS=Ready", daemon.SdNotifyReady)
	interval := n.watchdogInterval
	if !n.ready {
		interval = systemdReadyRetryInterval
	}
	if interval > 0 {
		n.workerStop = make(chan struct{})
		n.workerDone = make(chan struct{})
		go n.run(interval)
	}
}

// Reloading reports the start of a reload with its monotonic timestamp.
func (n *SystemdNotifier) Reloading() {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if !n.ready || n.stopped {
		return
	}
	n.notify(daemon.SdNotifyReloading, strings.TrimSuffix(daemon.SdNotifyMonotonicUsec(), "\n"))
}

// Reloaded reports the reload result and restores the ready state.
// A failed reload leaves the previous configuration active.
func (n *SystemdNotifier) Reloaded(err error) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if !n.ready || n.stopped {
		return
	}
	status := "Configuration reloaded"
	if err != nil {
		status = "Configuration reload failed: " + sanitizeStatus(err.Error()) + " (previous configuration remains active)"
	}
	n.notify("STATUS="+status, daemon.SdNotifyReady)
}

// Stopping cancels readiness retries and the watchdog, then reports shutdown.
// It waits for the notification worker to exit. Later calls send nothing.
func (n *SystemdNotifier) Stopping() {
	n.mtx.Lock()
	if !n.stopped {
		n.stopped = true
		if n.workerStop != nil {
			close(n.workerStop)
		}
		if n.ready {
			n.notify(daemon.SdNotifyStopping)
		}
	}
	done := n.workerDone
	n.mtx.Unlock()
	if done != nil {
		<-done
	}
}

// run retries readiness, then sends watchdog notifications until shutdown.
func (n *SystemdNotifier) run(interval time.Duration) {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	defer close(n.workerDone)

	for {
		select {
		case <-n.workerStop:
			return
		case <-timer.C:
			n.mtx.Lock()
			if n.stopped {
				n.mtx.Unlock()
				return
			}
			if n.ready {
				n.notify(daemon.SdNotifyWatchdog)
			} else {
				n.ready = n.notify("STATUS=Ready", daemon.SdNotifyReady)
				if n.ready {
					interval = n.watchdogInterval
				}
			}
			n.mtx.Unlock()
			if interval <= 0 {
				return
			}
			timer.Reset(interval)
		}
	}
}

// notify sends newline-separated assignments and reports success.
// It logs send failures.
func (n *SystemdNotifier) notify(assignments ...string) bool {
	if err := sendSystemdNotification(n.socket, strings.Join(assignments, "\n")); err != nil {
		n.logger.Warn("Failed to notify systemd", "err", err)
		return false
	}
	return true
}

// sendSystemdNotification bounds both connection setup and the datagram write.
// The daemon.SdNotify function does not expose a timeout or a connection deadline.
func sendSystemdNotification(socket, state string) error {
	deadline := time.Now().Add(systemdNotifyTimeout)
	conn, err := (&net.Dialer{Deadline: deadline}).Dial("unixgram", socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	_, err = conn.Write([]byte(state))
	return err
}

// sanitizeStatus returns single-line UTF-8 error text within maxStatusErrLen bytes.
// Truncated text ends with an ellipsis.
func sanitizeStatus(status string) string {
	status = strings.ToValidUTF8(status, "\uFFFD")
	status = strings.ReplaceAll(status, "\x00", " ")
	status = strings.Join(strings.Fields(status), " ")
	if len(status) > maxStatusErrLen {
		const ellipsis = "…"
		status = strings.ToValidUTF8(status[:maxStatusErrLen-len(ellipsis)], "") + ellipsis
	}
	return status
}
