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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

// SystemdNotifier sends lifecycle and watchdog notifications to systemd.
// Use NewSystemdNotifier to create one notifier per process.
type SystemdNotifier struct {
	logger           *slog.Logger
	enabled          bool
	watchdogInterval time.Duration

	mtx          sync.Mutex
	ready        bool
	stopped      bool
	watchdogStop chan struct{}
	watchdogDone chan struct{}
}

// NewSystemdNotifier enables notifications when NOTIFY_SOCKET is set.
// WATCHDOG_USEC and WATCHDOG_PID control watchdog notifications.
func NewSystemdNotifier(logger *slog.Logger) *SystemdNotifier {
	n := &SystemdNotifier{
		logger:  logger,
		enabled: os.Getenv("NOTIFY_SOCKET") != "",
	}
	if n.enabled {
		interval, err := daemon.SdWatchdogEnabled(false)
		if err != nil {
			logger.Warn("Failed to read systemd watchdog configuration", "err", err)
		} else {
			n.watchdogInterval = interval / 2
		}
	}
	return n
}

// Ready reports startup completion and starts the watchdog, if enabled.
func (n *SystemdNotifier) Ready() {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if !n.enabled || n.ready || n.stopped {
		return
	}
	n.ready = true
	n.notify("STATUS=Ready\n" + daemon.SdNotifyReady)
	if n.watchdogInterval > 0 {
		n.watchdogStop = make(chan struct{})
		n.watchdogDone = make(chan struct{})
		go n.watchdog()
	}
}

// Reloading reports the start of a reload with its monotonic timestamp.
func (n *SystemdNotifier) Reloading() {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if !n.enabled || !n.ready || n.stopped {
		return
	}
	n.notify(daemon.SdNotifyReloading + "\n" + daemon.SdNotifyMonotonicUsec())
}

// Reloaded reports the reload result and restores the ready state.
// A failed reload leaves the previous configuration active.
func (n *SystemdNotifier) Reloaded(err error) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	if !n.enabled || !n.ready || n.stopped {
		return
	}
	status := "Configuration reloaded"
	if err != nil {
		status = "Configuration reload failed: " + err.Error() + " (previous configuration remains active)"
	}
	n.notify("STATUS=" + sanitizeStatus(status) + "\n" + daemon.SdNotifyReady)
}

// Stopping stops the watchdog and reports shutdown. Later calls send nothing.
func (n *SystemdNotifier) Stopping() {
	n.mtx.Lock()
	if !n.stopped {
		n.stopped = true
		if n.watchdogStop != nil {
			close(n.watchdogStop)
		}
		if n.ready {
			n.notify(daemon.SdNotifyStopping)
		}
	}
	done := n.watchdogDone
	n.mtx.Unlock()
	if done != nil {
		<-done
	}
}

func (n *SystemdNotifier) watchdog() {
	ticker := time.NewTicker(n.watchdogInterval)
	defer ticker.Stop()
	defer close(n.watchdogDone)

	for {
		select {
		case <-n.watchdogStop:
			return
		case <-ticker.C:
			n.mtx.Lock()
			if !n.stopped {
				n.notify(daemon.SdNotifyWatchdog)
			}
			n.mtx.Unlock()
		}
	}
}

func (n *SystemdNotifier) notify(state string) {
	if _, err := daemon.SdNotify(false, state); err != nil {
		n.logger.Warn("Failed to notify systemd", "err", err)
	}
}

// sanitizeStatus keeps STATUS on one line and limits it to 256 UTF-8 bytes.
func sanitizeStatus(status string) string {
	status = strings.ToValidUTF8(status, "\uFFFD")
	status = strings.ReplaceAll(status, "\x00", " ")
	status = strings.Join(strings.Fields(status), " ")
	if len(status) > 256 {
		status = strings.ToValidUTF8(status[:256], "")
	}
	return status
}
