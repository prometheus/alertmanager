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

	require.NoError(t, fo.SendEvent([]byte("{\"a\":1}\n")))
	require.NoError(t, fo.SendEvent([]byte("{\"b\":2}\n")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "{\"a\":1}\n{\"b\":2}\n", string(data))
}

func TestFileOutput_ReopenAfterRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)
	defer fo.Close()

	require.NoError(t, fo.SendEvent([]byte("before\n")))

	// Simulate logrotate: rename the file away.
	rotated := filepath.Join(dir, "events.jsonl.1")
	require.NoError(t, os.Rename(path, rotated))

	// Wait for the fsnotify watcher to detect the rename and reopen
	// the file.  The new file should appear on disk.
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	require.NoError(t, fo.SendEvent([]byte("after\n")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "after\n", string(data))

	// The rotated file should have the original content.
	data, err = os.ReadFile(rotated)
	require.NoError(t, err)
	require.Equal(t, "before\n", string(data))
}

func TestFileOutput_ReopenAfterRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)
	defer fo.Close()

	require.NoError(t, fo.SendEvent([]byte("first\n")))
	require.NoError(t, os.Remove(path))

	// Wait for the fsnotify watcher to detect the removal and reopen
	// the file.
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	require.NoError(t, fo.SendEvent([]byte("second\n")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "second\n", string(data))
}

func TestFileOutput_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	fo, err := NewFileOutput(path, slog.Default())
	require.NoError(t, err)

	require.NoError(t, fo.SendEvent([]byte("data\n")))
	require.NoError(t, fo.Close())

	// Writing after close should fail.
	require.Error(t, fo.SendEvent([]byte("more\n")))
}

func TestFileOutput_InvalidPath(t *testing.T) {
	_, err := NewFileOutput("/nonexistent/dir/events.jsonl", slog.Default())
	require.Error(t, err)
}

// --- config tests.

func TestOutput_UnmarshalYAML_File(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, o Output)
	}{
		{
			name: "valid",
			yaml: "type: file\npath: /tmp/events.jsonl\n",
			check: func(t *testing.T, o Output) {
				require.Equal(t, OutputFile, o.Type)
				require.Equal(t, "/tmp/events.jsonl", o.Path)
			},
		},
		{
			name:    "missing path",
			yaml:    "type: file\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o Output
			err := yaml.Unmarshal([]byte(tc.yaml), &o)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, o)
			}
		})
	}
}

func TestEventRecorderConfigEqual_File(t *testing.T) {
	a := Config{Outputs: []Output{{Type: OutputFile, Path: "/tmp/events.jsonl"}}}
	b := Config{Outputs: []Output{{Type: OutputFile, Path: "/tmp/events.jsonl"}}}
	require.True(t, configEqual(a, b))

	b.Outputs[0].Path = "/tmp/other.jsonl"
	require.False(t, configEqual(a, b))
}
