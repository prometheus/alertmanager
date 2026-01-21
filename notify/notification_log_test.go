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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/types"
)

func TestNotificationLogEntry_JSONSerialization(t *testing.T) {
	entry := &NotificationLogEntry{
		Timestamp:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Integration:    "slack",
		IntegrationIdx: 0,
		Receiver:       "team-alerts",
		GroupKey:       "{}:{alertname=\"HighMemory\"}",
		AlertsCount:    3,
		FiringCount:    2,
		ResolvedCount:  1,
		GroupLabels: map[string]string{
			"alertname": "HighMemory",
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded NotificationLogEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, entry.Integration, decoded.Integration)
	require.Equal(t, entry.IntegrationIdx, decoded.IntegrationIdx)
	require.Equal(t, entry.Receiver, decoded.Receiver)
	require.Equal(t, entry.GroupKey, decoded.GroupKey)
	require.Equal(t, entry.AlertsCount, decoded.AlertsCount)
	require.Equal(t, entry.FiringCount, decoded.FiringCount)
	require.Equal(t, entry.ResolvedCount, decoded.ResolvedCount)
	require.Equal(t, entry.GroupLabels, decoded.GroupLabels)
}

func TestFileNotificationLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "notifications.log")

	nl, err := NewFileNotificationLogger(logPath)
	require.NoError(t, err)
	defer nl.Close()

	entry := &NotificationLogEntry{
		Timestamp:      time.Now().UTC(),
		Integration:    "webhook",
		IntegrationIdx: 1,
		Receiver:       "my-receiver",
		GroupKey:       "{}:{alertname=\"TestAlert\"}",
		AlertsCount:    2,
		FiringCount:    1,
		ResolvedCount:  1,
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
	}

	err = nl.Log(entry)
	require.NoError(t, err)

	// Write another entry
	entry2 := &NotificationLogEntry{
		Timestamp:      time.Now().UTC(),
		Integration:    "slack",
		IntegrationIdx: 0,
		Receiver:       "slack-channel",
		GroupKey:       "{}:{alertname=\"AnotherAlert\"}",
		AlertsCount:    5,
		FiringCount:    5,
		ResolvedCount:  0,
		GroupLabels: map[string]string{
			"alertname": "AnotherAlert",
			"severity":  "critical",
		},
	}

	err = nl.Log(entry2)
	require.NoError(t, err)

	// Close the file and read its contents
	err = nl.Close()
	require.NoError(t, err)

	f, err := os.Open(logPath)
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var entries []NotificationLogEntry

	for scanner.Scan() {
		var e NotificationLogEntry
		err := json.Unmarshal(scanner.Bytes(), &e)
		require.NoError(t, err)
		entries = append(entries, e)
	}
	require.NoError(t, scanner.Err())

	require.Len(t, entries, 2)
	require.Equal(t, "webhook", entries[0].Integration)
	require.Equal(t, "my-receiver", entries[0].Receiver)
	require.Equal(t, "slack", entries[1].Integration)
	require.Equal(t, "slack-channel", entries[1].Receiver)
}

func TestFileNotificationLogger_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "notifications_concurrent.log")

	nl, err := NewFileNotificationLogger(logPath)
	require.NoError(t, err)
	defer nl.Close()

	numGoroutines := 10
	numEntriesPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numEntriesPerGoroutine; j++ {
				entry := &NotificationLogEntry{
					Timestamp:      time.Now().UTC(),
					Integration:    "webhook",
					IntegrationIdx: goroutineID,
					Receiver:       "test-receiver",
					GroupKey:       "{}:{alertname=\"Test\"}",
					AlertsCount:    1,
					FiringCount:    1,
					ResolvedCount:  0,
					GroupLabels: map[string]string{
						"alertname": "Test",
					},
				}
				err := nl.Log(entry)
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
	err = nl.Close()
	require.NoError(t, err)

	// Verify all entries were written
	f, err := os.Open(logPath)
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0

	for scanner.Scan() {
		var e NotificationLogEntry
		err := json.Unmarshal(scanner.Bytes(), &e)
		require.NoError(t, err)
		count++
	}
	require.NoError(t, scanner.Err())

	require.Equal(t, numGoroutines*numEntriesPerGoroutine, count)
}

func TestNoopNotificationLogger(t *testing.T) {
	nl := NoopNotificationLogger{}

	entry := &NotificationLogEntry{
		Timestamp:   time.Now().UTC(),
		Integration: "test",
		Receiver:    "test-receiver",
	}

	err := nl.Log(entry)
	require.NoError(t, err)
}

func TestFileNotificationLogger_InvalidPath(t *testing.T) {
	// Try to create a logger with an invalid path
	_, err := NewFileNotificationLogger("/nonexistent/path/to/file.log")
	require.Error(t, err)
}

func TestNotificationLogEntry_NilGroupLabels(t *testing.T) {
	entry := &NotificationLogEntry{
		Timestamp:      time.Now().UTC(),
		Integration:    "slack",
		IntegrationIdx: 0,
		Receiver:       "test",
		GroupKey:       "test-key",
		AlertsCount:    1,
		FiringCount:    1,
		ResolvedCount:  0,
		GroupLabels:    nil,
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded NotificationLogEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Nil(t, decoded.GroupLabels)
}

// mockNotificationLogger is a test implementation that captures logged entries.
type mockNotificationLogger struct {
	mu      sync.Mutex
	entries []*NotificationLogEntry
}

func (m *mockNotificationLogger) Log(entry *NotificationLogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockNotificationLogger) getEntries() []*NotificationLogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries
}

func TestRetryStageWithNotificationLogger(t *testing.T) {
	// Create a mock notification logger
	mockLogger := &mockNotificationLogger{}

	// Create a successful notifier
	i := Integration{
		name: "test-integration",
		idx:  0,
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			return false, nil // Success
		}),
		rs:           sendResolved(true),
		receiverName: "test-receiver",
	}

	r := NewRetryStage(i, "test-receiver", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}), mockLogger)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "TestAlert", "severity": "critical"},
				EndsAt:   time.Now().Add(time.Hour),
				StartsAt: time.Now(),
			},
		},
	}

	ctx := context.Background()
	ctx = WithFiringAlerts(ctx, []uint64{1})
	ctx = WithResolvedAlerts(ctx, []uint64{})
	ctx = WithGroupKey(ctx, "{}:{alertname=\"TestAlert\"}")
	ctx = WithGroupLabels(ctx, model.LabelSet{"alertname": "TestAlert"})

	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)

	// Verify the notification was logged
	entries := mockLogger.getEntries()
	require.Len(t, entries, 1)

	entry := entries[0]
	require.Equal(t, "test-integration", entry.Integration)
	require.Equal(t, 0, entry.IntegrationIdx)
	require.Equal(t, "test-receiver", entry.Receiver)
	require.Equal(t, "{}:{alertname=\"TestAlert\"}", entry.GroupKey)
	require.Equal(t, 1, entry.AlertsCount)
	require.Equal(t, 1, entry.FiringCount)
	require.Equal(t, 0, entry.ResolvedCount)
	require.Equal(t, "TestAlert", entry.GroupLabels["alertname"])
}

func TestRetryStageWithNilNotificationLogger(t *testing.T) {
	// Create a successful notifier with nil notification logger
	i := Integration{
		name: "test-integration",
		idx:  0,
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			return false, nil // Success
		}),
		rs:           sendResolved(true),
		receiverName: "test-receiver",
	}

	// Pass nil for notification logger
	r := NewRetryStage(i, "test-receiver", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}), nil)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "TestAlert"},
				EndsAt:   time.Now().Add(time.Hour),
				StartsAt: time.Now(),
			},
		},
	}

	ctx := context.Background()
	ctx = WithFiringAlerts(ctx, []uint64{1})
	ctx = WithResolvedAlerts(ctx, []uint64{})

	// Should not panic with nil notification logger
	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)
}

// errorNotificationLogger is a test implementation that always returns an error.
type errorNotificationLogger struct {
	err error
}

func (e *errorNotificationLogger) Log(entry *NotificationLogEntry) error {
	return e.err
}

func TestRetryStageWithNotificationLoggerError(t *testing.T) {
	// Create a notification logger that always returns an error
	errLogger := &errorNotificationLogger{err: errors.New("log write failed")}

	// Create a successful notifier
	i := Integration{
		name: "test-integration",
		idx:  0,
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			return false, nil // Success
		}),
		rs:           sendResolved(true),
		receiverName: "test-receiver",
	}

	r := NewRetryStage(i, "test-receiver", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}), errLogger)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:   model.LabelSet{"alertname": "TestAlert"},
				EndsAt:   time.Now().Add(time.Hour),
				StartsAt: time.Now(),
			},
		},
	}

	ctx := context.Background()
	ctx = WithFiringAlerts(ctx, []uint64{1})
	ctx = WithResolvedAlerts(ctx, []uint64{})
	ctx = WithGroupKey(ctx, "{}:{alertname=\"TestAlert\"}")
	ctx = WithGroupLabels(ctx, model.LabelSet{"alertname": "TestAlert"})

	// Notification should still succeed even if logging fails
	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)
}

func TestFileNotificationLogger_CloseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "notifications_idempotent.log")

	nl, err := NewFileNotificationLogger(logPath)
	require.NoError(t, err)

	// First close should succeed
	err = nl.Close()
	require.NoError(t, err)

	// Second close should also succeed (idempotent)
	err = nl.Close()
	require.NoError(t, err)

	// Third close should also succeed
	err = nl.Close()
	require.NoError(t, err)
}

func TestFileNotificationLogger_LogAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "notifications_after_close.log")

	nl, err := NewFileNotificationLogger(logPath)
	require.NoError(t, err)

	// Close the logger
	err = nl.Close()
	require.NoError(t, err)

	// Attempt to log after close should return ErrNotificationLogClosed
	entry := &NotificationLogEntry{
		Timestamp:   time.Now().UTC(),
		Integration: "test",
		Receiver:    "test-receiver",
	}
	err = nl.Log(entry)
	require.ErrorIs(t, err, ErrNotificationLogClosed)
}
