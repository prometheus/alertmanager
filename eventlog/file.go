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

package eventlog

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileOutput writes pre-serialized JSON event bytes to a JSONL file.
// The file is reopened when fsnotify detects a rename or remove (e.g.
// logrotate).
type FileOutput struct {
	path   string
	mu     sync.Mutex
	f      *os.File
	logger *slog.Logger
	done   chan struct{}
}

// Name returns a stable identifier for this output.
func (fo *FileOutput) Name() string {
	return fmt.Sprintf("file:%s", fo.path)
}

// NewFileOutput creates a new file-based event log output at the given
// path.  The file is watched with fsnotify so that external log
// rotation tools (e.g., logrotate) trigger an immediate reopen.
func NewFileOutput(path string, logger *slog.Logger) (*FileOutput, error) {
	f, err := openAppend(path)
	if err != nil {
		return nil, err
	}

	fo := &FileOutput{
		path:   path,
		f:      f,
		logger: logger,
		done:   make(chan struct{}),
	}

	go fo.watchLoop()
	return fo, nil
}

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

func (fo *FileOutput) reopen() {
	fo.mu.Lock()
	defer fo.mu.Unlock()

	if fo.f != nil {
		fo.f.Close()
	}
	f, err := openAppend(fo.path)
	if err != nil {
		fo.logger.Error("Failed to reopen event log file", "path", fo.path, "err", err)
		return
	}
	fo.f = f
}

func (fo *FileOutput) watchLoop() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fo.logger.Error("Failed to create fsnotify watcher for event log file", "err", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(fo.path); err != nil {
		fo.logger.Error("Failed to watch event log file", "path", fo.path, "err", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
				fo.reopen()
				// Re-add the watch since the original inode is gone.
				_ = watcher.Add(fo.path)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fo.logger.Error("fsnotify error on event log file", "err", err)
		case <-fo.done:
			return
		}
	}
}

// WriteEvent writes the pre-serialized JSON bytes to the file.
func (fo *FileOutput) WriteEvent(data []byte) error {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	_, err := fo.f.Write(data)
	return err
}

// Close stops the watcher goroutine and closes the file.
func (fo *FileOutput) Close() error {
	close(fo.done)
	fo.mu.Lock()
	defer fo.mu.Unlock()
	return fo.f.Close()
}
