// Copyright 2019 Prometheus Team
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

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigFileChecksum_ReturnsConsistentHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("content: a"), 0o644))

	sum1, err := configFileChecksum(path)
	require.NoError(t, err)
	sum2, err := configFileChecksum(path)
	require.NoError(t, err)

	require.Equal(t, sum1, sum2, "same content should produce same checksum")
}

func TestConfigFileChecksum_DifferentContentDifferentHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")

	require.NoError(t, os.WriteFile(path, []byte("content: a"), 0o644))
	sumA, err := configFileChecksum(path)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("content: b"), 0o644))
	sumB, err := configFileChecksum(path)
	require.NoError(t, err)

	require.NotEqual(t, sumA, sumB, "different content must produce different checksum")
}

func TestConfigFileChecksum_MissingFileReturnsError(t *testing.T) {
	_, err := configFileChecksum("/nonexistent/path/alertmanager.yml")
	require.Error(t, err)
}

func TestRunConfigWatcher_NoReloadWhenFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("route:\n  receiver: default\n"), 0o644))

	reloadCh := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	go runConfigWatcher(ctx, path, 50*time.Millisecond, reloadCh, slog.Default())

	// Let the watcher run for at least 3 ticks.
	<-ctx.Done()

	// reloadCh must be empty - no reload should have been triggered.
	require.Empty(t, reloadCh, "no reload expected when file is unchanged")
}

func TestRunConfigWatcher_TriggersReloadOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	reloadCh := make(chan struct{}, 1)
	ctx := t.Context()

	go runConfigWatcher(ctx, path, 30*time.Millisecond, reloadCh, slog.Default())

	// Wait one tick to let the initial checksum be set.
	time.Sleep(50 * time.Millisecond)

	// Change the file.
	require.NoError(t, os.WriteFile(path, []byte("changed"), 0o644))

	// Wait for the watcher to detect the change and send to reloadCh.
	select {
	case <-reloadCh:
		// Reload triggered.
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for reload signal after file change")
	}
}

func TestRunConfigWatcher_DoesNotRetriggerAfterSuccessfulReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	reloadCh := make(chan struct{}, 2) // buffer=2 to catch any spurious second reload
	ctx := t.Context()

	go runConfigWatcher(ctx, path, 30*time.Millisecond, reloadCh, slog.Default())

	time.Sleep(50 * time.Millisecond) // allow initial checksum to be set

	// Change the file once.
	require.NoError(t, os.WriteFile(path, []byte("changed"), 0o644))

	// Consume the first (expected) reload.
	select {
	case <-reloadCh:
		// Reload received.
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected first reload not received")
	}

	// Let a few ticks pass - file is unchanged so no second reload should come.
	time.Sleep(150 * time.Millisecond)

	require.Empty(t, reloadCh, "no second reload expected after successful reload of same content")
}

func TestRunConfigWatcher_HandlesUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	reloadCh := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go runConfigWatcher(ctx, path, 30*time.Millisecond, reloadCh, slog.Default())

	time.Sleep(50 * time.Millisecond)

	// Remove the file to simulate a transient read failure.
	require.NoError(t, os.Remove(path))

	// Watcher should log a warning but NOT send to reloadCh.
	<-ctx.Done()
	require.Empty(t, reloadCh, "no reload expected when file is unreadable")
}

func TestRunConfigWatcher_SeedsChecksumAfterStartupFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")

	// The file does not exist at startup, so the initial checksum read will fail.
	reloadCh := make(chan struct{}, 1)
	ctx := t.Context()

	go runConfigWatcher(ctx, path, 30*time.Millisecond, reloadCh, slog.Default())

	// Create the file after the watcher has started.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))

	// The first successful tick should seed the checksum without reloading.
	time.Sleep(100 * time.Millisecond)
	require.Empty(t, reloadCh, "no reload expected when seeding baseline after startup failure")
}

func TestRunConfigWatcher_ExitsWhenContextCancelled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))

	reloadCh := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runConfigWatcher(ctx, path, 30*time.Millisecond, reloadCh, slog.Default())
		close(done)
	}()

	cancel() // Cancel immediately.

	select {
	case <-done:
		// Watcher exited cleanly.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("watcher goroutine did not exit after context cancellation")
	}
}
