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

package eventrecorder

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestFileOutput_SendEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)
	defer fo.Close()

	require.Equal(t, "file:"+path, fo.Name())

	n1, err := fo.SendEvent(sampleEvent())
	require.NoError(t, err)
	require.Positive(t, n1)
	n2, err := fo.SendEvent(sampleEvent())
	require.NoError(t, err)
	require.Positive(t, n2)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Two JSONL records, each a newline-terminated JSON object.
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 2)
	for _, line := range lines {
		require.True(t, strings.HasPrefix(line, "{"))
		require.Contains(t, line, "alertmanagerStartupEvent")
	}
}

func TestFileOutput_ReopenAfterRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)
	defer fo.Close()

	_, err = fo.SendEvent(sampleEvent())
	require.NoError(t, err)

	// Simulate logrotate: rename the file away.
	rotated := filepath.Join(dir, "events.jsonl.1")
	require.NoError(t, os.Rename(path, rotated))

	// Wait for the fsnotify watcher to detect the rename and reopen
	// the file.  The new file should appear on disk.
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	_, err = fo.SendEvent(sampleEvent())
	require.NoError(t, err)

	// The freshly reopened file holds exactly one record.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(data), "\n"), "\n"), 1)

	// The rotated file holds the record written before rotation.
	data, err = os.ReadFile(rotated)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(data), "\n"), "\n"), 1)
}

func TestFileOutput_ReopenAfterRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)
	defer fo.Close()

	_, err = fo.SendEvent(sampleEvent())
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))

	// Wait for the fsnotify watcher to detect the removal and reopen
	// the file.
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	_, err = fo.SendEvent(sampleEvent())
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(data), "\n"), "\n"), 1)
}

func TestFileOutput_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)

	_, err = fo.SendEvent(sampleEvent())
	require.NoError(t, err)
	require.NoError(t, fo.Close())

	// Writing after close should fail.
	_, err = fo.SendEvent(sampleEvent())
	require.Error(t, err)
}

func TestFileOutput_InvalidPath(t *testing.T) {
	_, err := NewFileOutput("/nonexistent/dir/events.jsonl", slog.Default())
	require.Error(t, err)
}

// --- config tests.

func TestFileOutputConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, c FileOutputConfig)
	}{
		{
			name: "valid",
			yaml: "path: /tmp/events.jsonl\n",
			check: func(t *testing.T, c FileOutputConfig) {
				require.Equal(t, "/tmp/events.jsonl", c.Path)
			},
		},
		{
			name:    "missing path",
			yaml:    "{}\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c FileOutputConfig
			err := yaml.Unmarshal([]byte(tc.yaml), &c)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, c)
			}
		})
	}
}

func TestEventRecorderConfigEqual_File(t *testing.T) {
	a := Config{FileOutputs: []FileOutputConfig{{Path: "/tmp/events.jsonl"}}}
	b := Config{FileOutputs: []FileOutputConfig{{Path: "/tmp/events.jsonl"}}}
	require.True(t, configEqual(a, b))

	b.FileOutputs[0].Path = "/tmp/other.jsonl"
	require.False(t, configEqual(a, b))
}
