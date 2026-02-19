// Copyright 2015 Prometheus Team
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

package notify

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

// ErrNotificationLogClosed is returned when attempting to write to a closed notification log.
var ErrNotificationLogClosed = errors.New("notification log is closed")

// NotificationLogEntry represents a single notification log entry written to the
// notification log file. Each entry captures metadata about a successfully sent
// notification, including the integration type, receiver name, alert counts, and
// group labels. The entry is serialized as a JSON object on a single line.
//
// Example JSON output:
//
//	{
//	  "timestamp": "2024-01-15T10:30:00.123Z",
//	  "integration": "slack",
//	  "integrationIdx": 0,
//	  "receiver": "team-alerts",
//	  "groupKey": "{}:{alertname=\"HighMemory\"}",
//	  "alertsCount": 3,
//	  "firingCount": 2,
//	  "resolvedCount": 1,
//	  "groupLabels": {"alertname": "HighMemory"}
//	}
type NotificationLogEntry struct {
	// Timestamp is the time when the notification was successfully sent.
	Timestamp time.Time `json:"timestamp"`
	// Integration is the type of notifier used (e.g., "slack", "webhook", "email").
	Integration string `json:"integration"`
	// IntegrationIdx is the index of the integration within the receiver configuration.
	IntegrationIdx int `json:"integrationIdx"`
	// Receiver is the name of the receiver that processed the notification.
	Receiver string `json:"receiver"`
	// GroupKey is the unique identifier for the alert group.
	GroupKey string `json:"groupKey"`
	// AlertsCount is the total number of alerts in the notification.
	AlertsCount int `json:"alertsCount"`
	// FiringCount is the number of firing alerts in the notification.
	FiringCount int `json:"firingCount"`
	// ResolvedCount is the number of resolved alerts in the notification.
	ResolvedCount int `json:"resolvedCount"`
	// GroupLabels contains the labels used for grouping alerts.
	GroupLabels map[string]string `json:"groupLabels"`
}

// NotificationLogger is the interface for logging successfully sent notifications.
// Implementations should be safe for concurrent use.
type NotificationLogger interface {
	// Log writes a notification entry to the log. It returns an error if the
	// write fails, but implementations should not block notification delivery.
	Log(entry *NotificationLogEntry) error
}

// FileNotificationLogger logs notifications to a file in JSON lines format.
// Each notification is written as a single JSON object followed by a newline.
// The logger is safe for concurrent use. Data is synced to disk when Close is called.
type FileNotificationLogger struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

// NewFileNotificationLogger creates a new file-based notification logger that
// writes to the specified path. The file is created if it doesn't exist, and
// new entries are appended if it does. Returns an error if the file cannot be
// opened or created.
func NewFileNotificationLogger(path string) (*FileNotificationLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileNotificationLogger{
		file: f,
	}, nil
}

// Log writes a notification entry to the log file as a JSON line.
// The data is buffered by the OS and synced to disk when Close is called.
// Returns ErrNotificationLogClosed if the logger has been closed.
func (l *FileNotificationLogger) Log(entry *NotificationLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrNotificationLogClosed
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = l.file.Write(data)
	return err
}

// Close syncs any buffered data to disk and closes the log file.
// It is safe to call Close multiple times; subsequent calls will return nil.
// After Close is called, any calls to Log will return ErrNotificationLogClosed.
func (l *FileNotificationLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	// Sync before closing to ensure all data is written to disk.
	syncErr := l.file.Sync()
	closeErr := l.file.Close()

	// Return sync error if it occurred, otherwise return close error.
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// NoopNotificationLogger is a no-op implementation of NotificationLogger
// used when notification logging is disabled.
type NoopNotificationLogger struct{}

// Log is a no-op that always returns nil.
func (n NoopNotificationLogger) Log(entry *NotificationLogEntry) error {
	return nil
}
