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
	"log/slog"
	"time"

	"github.com/pengsrc/go-shared/reopen"
)

const fileReopenInterval = 5 * time.Minute

// FileOutput writes pre-serialized JSON event bytes to a JSONL file.
// The underlying file is reopened periodically so that external log
// rotation tools (e.g., logrotate) work correctly.
type FileOutput struct {
	fw     *reopen.FileWriter
	logger *slog.Logger
	done   chan struct{}
}

// NewFileOutput creates a new file-based event log output at the given
// path.  The file is reopened every 5 minutes so that external log
// rotation tools (e.g., logrotate) work correctly.
func NewFileOutput(path string, logger *slog.Logger) (*FileOutput, error) {
	fw, err := reopen.NewFileWriter(path)
	if err != nil {
		return nil, err
	}

	fo := &FileOutput{
		fw:     fw,
		logger: logger,
		done:   make(chan struct{}),
	}

	go fo.reopenLoop()
	return fo, nil
}

func (fo *FileOutput) reopenLoop() {
	ticker := time.NewTicker(fileReopenInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := fo.fw.Reopen(); err != nil {
				fo.logger.Error("Failed to reopen event log file", "err", err)
			}
		case <-fo.done:
			return
		}
	}
}

// WriteEvent writes the pre-serialized JSON bytes to the file.
func (fo *FileOutput) WriteEvent(data []byte) error {
	_, err := fo.fw.Write(data)
	return err
}

// Close stops the reopen goroutine and closes the file.
func (fo *FileOutput) Close() error {
	close(fo.done)
	return fo.fw.Close()
}
