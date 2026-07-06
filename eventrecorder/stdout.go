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
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// StdoutOutputConfig configures a stdout event recorder output.
// There are no required fields; the presence of an entry in
// stdout_outputs is sufficient to enable the output.
type StdoutOutputConfig struct{}

// equal reports whether two stdout output configs are semantically equal.
// All StdoutOutputConfig values are identical since the type carries no fields.
func (c StdoutOutputConfig) equal(_ StdoutOutputConfig) bool { return true }

// StdoutOutput writes events as newline-delimited JSON to os.Stdout.
// This is the recommended output for container deployments where stdout
// is captured by the runtime log driver (Docker, Kubernetes, etc.).
//
// Each event is serialized with protojson and followed by a newline so
// log collectors receive one self-contained JSON object per line.
type StdoutOutput struct{}

// Name returns the stable identifier used in Prometheus metric labels.
func (s *StdoutOutput) Name() string { return "stdout" }

// SendEvent serializes the event as a JSON line and writes it to stdout.
// It returns the byte count written (including the trailing newline) and
// any write error encountered.  A serialization failure is wrapped in
// serializeError so the recorder attributes it to the correct metric.
func (s *StdoutOutput) SendEvent(event *eventrecorderpb.Event) (int, error) {
	data, err := protojson.Marshal(event)
	if err != nil {
		return 0, &serializeError{err: err}
	}
	data = append(data, '\n')
	return os.Stdout.Write(data)
}

// Close is a no-op; os.Stdout is owned by the process, not by this output.
func (s *StdoutOutput) Close() error { return nil }
