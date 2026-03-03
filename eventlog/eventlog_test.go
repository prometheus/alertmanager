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
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventlog/eventlogpb"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// mockOutput records all events written to it.
type mockOutput struct {
	mu     sync.Mutex
	name   string
	events [][]byte
}

func newMockOutput(name string) *mockOutput {
	return &mockOutput{name: name}
}

func (m *mockOutput) Name() string { return m.name }
func (m *mockOutput) WriteEvent(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, append([]byte(nil), data...))
	return nil
}
func (m *mockOutput) Close() error { return nil }

func (m *mockOutput) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func startupEvent() *eventlogpb.EventData {
	return &eventlogpb.EventData{
		EventType: &eventlogpb.EventData_AlertmanagerStartupEvent{
			AlertmanagerStartupEvent: &eventlogpb.AlertmanagerStartupEvent{
				Version: "test",
			},
		},
	}
}

func newTestRecorder(outputs ...Output) Recorder {
	core := &recorderCore{
		instance: "test",
		logger:   slog.Default(),
		metrics:  newMetrics(nil),
		outputs:  outputs,
		events:   make(chan writeRequest, eventQueueSize),
		done:     make(chan struct{}),
	}
	core.wg.Add(1)
	go core.writeLoop()
	return Recorder{core: core}
}

func TestRecordEvent(t *testing.T) {
	out := newMockOutput("test:mock")
	rec := newTestRecorder(out)
	defer rec.Close()

	rec.RecordEvent(context.Background(), startupEvent())

	// Wait for the event to be delivered.
	require.Eventually(t, func() bool {
		return out.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestRecordEventMultipleOutputs(t *testing.T) {
	out1 := newMockOutput("test:out1")
	out2 := newMockOutput("test:out2")
	rec := newTestRecorder(out1, out2)
	defer rec.Close()

	rec.RecordEvent(context.Background(), startupEvent())

	require.Eventually(t, func() bool {
		return out1.eventCount() == 1 && out2.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestNopRecorderDoesNotPanic(t *testing.T) {
	rec := NopRecorder()
	rec.RecordEvent(context.Background(), startupEvent())
	require.NoError(t, rec.Close())
}

func TestZeroRecorderDoesNotPanic(t *testing.T) {
	var rec Recorder
	rec.RecordEvent(context.Background(), startupEvent())
	require.NoError(t, rec.Close())
}

func TestRecordingDisabled(t *testing.T) {
	out := newMockOutput("test:mock")
	rec := newTestRecorder(out)
	defer rec.Close()

	ctx := WithRecordingDisabled(context.Background())
	rec.RecordEvent(ctx, startupEvent())

	// Record a second event normally to flush the queue.
	rec.RecordEvent(context.Background(), startupEvent())
	require.Eventually(t, func() bool {
		return out.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestApplyConfig(t *testing.T) {
	out1 := newMockOutput("test:out1")
	rec := newTestRecorder(out1)
	defer rec.Close()

	// Record one event to the initial output.
	rec.RecordEvent(context.Background(), startupEvent())
	require.Eventually(t, func() bool {
		return out1.eventCount() == 1
	}, time.Second, 10*time.Millisecond)

	// Apply a new config.  Since we can't easily create real outputs
	// in a test, we manually swap outputs.  But we can verify that an
	// unchanged config is a no-op.
	rec.core.mu.RLock()
	cfg := rec.core.currentCfg
	rec.core.mu.RUnlock()

	// ApplyConfig with same config should be a no-op.
	rec.ApplyConfig(cfg)
	rec.core.mu.RLock()
	require.Len(t, rec.core.outputs, 1)
	rec.core.mu.RUnlock()
}

func TestExtractEventType(t *testing.T) {
	tests := []struct {
		name     string
		event    *eventlogpb.EventData
		expected string
	}{
		{
			name:     "startup",
			event:    startupEvent(),
			expected: "startup",
		},
		{
			name: "shutdown",
			event: &eventlogpb.EventData{
				EventType: &eventlogpb.EventData_AlertmanagerShutdownEvent{},
			},
			expected: "shutdown",
		},
		{
			name: "alert_created",
			event: &eventlogpb.EventData{
				EventType: &eventlogpb.EventData_AlertCreated{},
			},
			expected: "alert_created",
		},
		{
			name:     "unknown",
			event:    &eventlogpb.EventData{},
			expected: "unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, extractEventType(tc.event))
		})
	}
}

func TestLabelSetAsProto(t *testing.T) {
	ls := model.LabelSet{"foo": "bar", "baz": "qux"}
	proto := LabelSetAsProto(ls)

	require.Len(t, proto.Labels, 2)
	found := map[string]string{}
	for _, lp := range proto.Labels {
		found[lp.Key] = lp.Value
	}
	require.Equal(t, "bar", found["foo"])
	require.Equal(t, "qux", found["baz"])
}

func TestMatcherAsProto(t *testing.T) {
	m, err := labels.NewMatcher(labels.MatchRegexp, "job", "api.*")
	require.NoError(t, err)

	proto := MatcherAsProto(m)
	require.Equal(t, eventlogpb.Matcher_TYPE_REGEXP, proto.Type)
	require.Equal(t, "job", proto.Name)
	require.Equal(t, "api.*", proto.Pattern)
	require.NotEmpty(t, proto.Rendered)
}

func TestMatchersAsProto(t *testing.T) {
	m1, err := labels.NewMatcher(labels.MatchEqual, "env", "prod")
	require.NoError(t, err)
	m2, err := labels.NewMatcher(labels.MatchNotEqual, "team", "")
	require.NoError(t, err)

	protos := MatchersAsProto(labels.Matchers{m1, m2})
	require.Len(t, protos, 2)
	require.Equal(t, eventlogpb.Matcher_TYPE_EQUAL, protos[0].Type)
	require.Equal(t, eventlogpb.Matcher_TYPE_NOT_EQUAL, protos[1].Type)
}

func TestEventLogConfigEqual(t *testing.T) {
	a := config.EventLogConfig{
		Outputs: []config.EventLogOutput{
			{Type: "file", Path: "/tmp/events.jsonl"},
		},
	}
	b := config.EventLogConfig{
		Outputs: []config.EventLogOutput{
			{Type: "file", Path: "/tmp/events.jsonl"},
		},
	}
	require.True(t, eventLogConfigEqual(a, b))

	b.Outputs[0].Path = "/tmp/other.jsonl"
	require.False(t, eventLogConfigEqual(a, b))

	// Different number of outputs.
	c := config.EventLogConfig{}
	require.False(t, eventLogConfigEqual(a, c))
}
