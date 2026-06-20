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
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// captureStdout replaces os.Stdout with a pipe for the duration of fn,
// then returns everything written to it.  The original os.Stdout is
// restored via t.Cleanup regardless of how fn exits.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	fn()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	return buf.String()
}

func TestStdoutOutput_Name(t *testing.T) {
	out := &StdoutOutput{}
	require.Equal(t, "stdout", out.Name())
}

func TestStdoutOutput_SendEvent(t *testing.T) {
	out := &StdoutOutput{}

	got := captureStdout(t, func() {
		n, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
		require.Positive(t, n)
	})

	// Expect a single newline-terminated JSON object.
	require.True(t, strings.HasPrefix(got, "{"), "output should start with a JSON object")
	require.True(t, strings.HasSuffix(got, "}\n"), "output should end with a closing brace and newline")
	require.Contains(t, got, "alertmanagerStartupEvent")
}

func TestStdoutOutput_SendEventTwice(t *testing.T) {
	out := &StdoutOutput{}

	got := captureStdout(t, func() {
		_, err := out.SendEvent(sampleEvent())
		require.NoError(t, err)
		_, err = out.SendEvent(sampleEvent())
		require.NoError(t, err)
	})

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Len(t, lines, 2, "two events should produce two JSONL lines")
	for _, line := range lines {
		require.True(t, strings.HasPrefix(line, "{"))
		require.Contains(t, line, "alertmanagerStartupEvent")
	}
}

func TestStdoutOutput_Close(t *testing.T) {
	out := &StdoutOutput{}
	require.NoError(t, out.Close(), "Close must be a no-op and return nil")
}

// TestStdoutOutput_ImplementsDestination is a compile-time check that
// StdoutOutput satisfies the Destination interface.
func TestStdoutOutput_ImplementsDestination(t *testing.T) {
	var _ Destination = (*StdoutOutput)(nil)
}

// TestStdoutOutput_IntegrationWithRecorder verifies events flow from a
// Recorder through the StdoutOutput and appear on stdout as JSON lines.
func TestStdoutOutput_IntegrationWithRecorder(t *testing.T) {
	// mirror is a mockDestination used solely to detect when the write
	// loop has delivered the event, so we know stdout was written.
	mirror := newMockDestination("test:mirror")

	r, w, err := os.Pipe()
	require.NoError(t, err)

	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	rec := newTestRecorder(&StdoutOutput{}, mirror)
	defer rec.Close()

	rec.RecordEvent(recordCtx(), startupEvent)

	// Wait until the write loop has delivered the event to both outputs.
	require.Eventually(t, func() bool {
		return mirror.eventCount() == 1
	}, time.Second, 10*time.Millisecond)

	// Close the write end so io.Copy below can reach EOF.
	require.NoError(t, w.Close())

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	require.Contains(t, buf.String(), "alertmanagerStartupEvent")
}

// --- config tests.

func TestStdoutOutputConfig_Equal(t *testing.T) {
	// All StdoutOutputConfig values compare equal since the type has no fields.
	a := StdoutOutputConfig{}
	b := StdoutOutputConfig{}
	require.True(t, a.equal(b))
}

func TestEventRecorderConfig_StdoutInTotalOutputs(t *testing.T) {
	cfg := Config{
		StdoutOutputs: []StdoutOutputConfig{{}},
	}
	require.Equal(t, 1, cfg.totalOutputs())
}

func TestEventRecorderConfigEqual_Stdout(t *testing.T) {
	a := Config{StdoutOutputs: []StdoutOutputConfig{{}}}
	b := Config{StdoutOutputs: []StdoutOutputConfig{{}}}
	require.True(t, configEqual(a, b))

	// Removing the stdout output makes them unequal.
	b.StdoutOutputs = nil
	require.False(t, configEqual(a, b))
}

func TestEventRecorderConfigEqual_StdoutVsFile(t *testing.T) {
	// Same total count but in different per-type lists must compare unequal.
	a := Config{StdoutOutputs: []StdoutOutputConfig{{}}}
	b := Config{FileOutputs: []FileOutputConfig{{Path: "/tmp/events.jsonl"}}}
	require.False(t, configEqual(a, b))
}

func TestStdoutOutputConfig_UnmarshalYAML(t *testing.T) {
	// An empty map is the natural YAML representation of a
	// StdoutOutputConfig since it carries no fields.
	raw := "stdout_outputs:\n  - {}\n"
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.Len(t, cfg.StdoutOutputs, 1)
}
