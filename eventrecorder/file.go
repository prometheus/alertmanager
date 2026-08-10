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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// FileOutputConfig configures a JSONL file event recorder output.
type FileOutputConfig struct {
	// Path is the JSONL file to append events to.  Created if absent.
	Path string `yaml:"path" json:"path"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface, validating
// the file output configuration.
func (c *FileOutputConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type plain FileOutputConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Path == "" {
		return errors.New("event_recorder file output requires a path")
	}
	return nil
}

// equal reports whether two file output configs are semantically equal.
func (c FileOutputConfig) equal(o FileOutputConfig) bool {
	return c.Path == o.Path
}

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

// SendEvent serializes the event as a JSON line and appends it to the
// file.  It returns the number of bytes written (including the trailing
// newline) for the bytes-written metric.
func (fo *FileOutput) SendEvent(event *eventrecorderpb.Event) (int, error) {
	data, err := protojson.Marshal(event)
	if err != nil {
		return 0, &serializeError{err: err}
	}
	data = append(data, '\n')

	fo.mu.Lock()
	defer fo.mu.Unlock()
	n, err := fo.f.Write(data)
	return n, err
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
