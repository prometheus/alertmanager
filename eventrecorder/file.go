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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	closed bool
	logger *slog.Logger
	done   chan struct{}
	wg     sync.WaitGroup
}

// Name returns a stable identifier for this output.
func (fo *FileOutput) Name() string {
	return fmt.Sprintf("file:%s", fo.path)
}

// NewFileOutput creates a new file-based event recorder output at the given
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

	ready := make(chan error, 1)
	fo.wg.Add(1)
	go fo.watchLoop(ready)
	if err := <-ready; err != nil {
		f.Close()
		return nil, fmt.Errorf("starting file watcher: %w", err)
	}
	return fo, nil
}

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

func (fo *FileOutput) reopen() {
	fo.mu.Lock()
	defer fo.mu.Unlock()

	if fo.closed {
		return
	}
	if fo.f != nil {
		fo.f.Close()
	}
	f, err := openAppend(fo.path)
	if err != nil {
		fo.logger.Error("Failed to reopen event recorder file", "path", fo.path, "err", err)
		return
	}
	fo.f = f
}

func (fo *FileOutput) watchLoop(ready chan<- error) {
	defer fo.wg.Done()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		ready <- fmt.Errorf("creating fsnotify watcher: %w", err)
		return
	}
	defer watcher.Close()

	// Watch the parent directory rather than the file itself.
	// When logrotate renames the file, an inode-level watch is
	// lost with the old inode.  A directory watch reliably
	// delivers Rename/Remove/Create events for the target path.
	dir := filepath.Dir(fo.path)
	if err := watcher.Add(dir); err != nil {
		ready <- fmt.Errorf("watching directory %s: %w", dir, err)
		return
	}

	absPath, err := filepath.Abs(fo.path)
	if err != nil {
		ready <- fmt.Errorf("resolving absolute path for %s: %w", fo.path, err)
		return
	}

	ready <- nil

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only react to events on our target file.
			evPath, _ := filepath.Abs(event.Name)
			if evPath != absPath {
				continue
			}
			if event.Op&(fsnotify.Rename|fsnotify.Remove|fsnotify.Create) != 0 {
				fo.reopen()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fo.logger.Error("fsnotify error on event recorder directory", "err", err)
		case <-fo.done:
			return
		}
	}
}

// SendEvent writes the pre-serialized JSON bytes to the file.
func (fo *FileOutput) SendEvent(data []byte) error {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	_, err := fo.f.Write(data)
	return err
}

// Close stops the watcher goroutine, waits for it to exit, and closes
// the file.
func (fo *FileOutput) Close() error {
	close(fo.done)
	fo.wg.Wait()
	fo.mu.Lock()
	defer fo.mu.Unlock()
	fo.closed = true
	return fo.f.Close()
}
