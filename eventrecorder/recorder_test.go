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
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// mockDestination records all events written to it.
type mockDestination struct {
	mu     sync.Mutex
	name   string
	events [][]byte
}

func newMockDestination(name string) *mockDestination {
	return &mockDestination{name: name}
}

func (m *mockDestination) Name() string { return m.name }
func (m *mockDestination) SendEvent(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, append([]byte(nil), data...))
	return nil
}
func (m *mockDestination) Close() error { return nil }

func (m *mockDestination) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func startupEvent() *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertmanagerStartupEvent{
			AlertmanagerStartupEvent: &eventrecorderpb.AlertmanagerStartupEvent{
				Version: "test",
			},
		},
	}
}

func newTestRecorder(outputs ...Destination) Recorder {
	core := &sharedRecorder{
		instance:  "test",
		logger:    slog.Default(),
		metrics:   newMetrics(nil),
		events:    make(chan writeRequest, eventQueueSize),
		cfgUpdate: make(chan cfgUpdateMsg),
		done:      make(chan struct{}),
	}
	core.wg.Add(1)
	go core.writeLoop(outputs, Config{})
	return Recorder{core: core}
}

func recordCtx() context.Context {
	return WithEventRecording(context.Background())
}

func TestRecordEvent(t *testing.T) {
	out := newMockDestination("test:mock")
	rec := newTestRecorder(out)
	defer rec.Close()

	rec.RecordEvent(recordCtx(), startupEvent())

	// Wait for the event to be delivered.
	require.Eventually(t, func() bool {
		return out.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestRecordEventMultipleDestinations(t *testing.T) {
	out1 := newMockDestination("test:out1")
	out2 := newMockDestination("test:out2")
	rec := newTestRecorder(out1, out2)
	defer rec.Close()

	rec.RecordEvent(recordCtx(), startupEvent())

	require.Eventually(t, func() bool {
		return out1.eventCount() == 1 && out2.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestNopRecorderDoesNotPanic(t *testing.T) {
	rec := NopRecorder()
	rec.RecordEvent(recordCtx(), startupEvent())
	rec.ApplyConfig(Config{})
	rec.SetClusterPeer(nil)
	require.NoError(t, rec.Close())
}

func TestZeroRecorderDoesNotPanic(t *testing.T) {
	var rec Recorder
	rec.RecordEvent(recordCtx(), startupEvent())
	rec.ApplyConfig(Config{})
	rec.SetClusterPeer(nil)
	require.NoError(t, rec.Close())
}

// NewRecorderFromConfig must tolerate a nil *slog.Logger by
// substituting a discard logger, so downstream code (buildOutputs,
// writeLoop, per-output constructors) can call the logger
// unconditionally.
func TestNewRecorderFromConfig_NilLogger(t *testing.T) {
	require.NotPanics(t, func() {
		rec := NewRecorderFromConfig(Config{}, "test-host", nil, nil)
		rec.RecordEvent(recordCtx(), startupEvent())
		rec.ApplyConfig(Config{})
		require.NoError(t, rec.Close())
	})
}

func TestRecordingNotEnabledByDefault(t *testing.T) {
	out := newMockDestination("test:mock")
	rec := newTestRecorder(out)
	defer rec.Close()

	// Without WithEventRecording, events should be silently discarded.
	rec.RecordEvent(context.Background(), startupEvent())

	// Record an event with recording enabled to flush the queue.
	rec.RecordEvent(recordCtx(), startupEvent())
	require.Eventually(t, func() bool {
		return out.eventCount() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestApplyConfig(t *testing.T) {
	out1 := newMockDestination("test:out1")
	rec := newTestRecorder(out1)
	defer rec.Close()

	// Record one event to the initial destination.
	rec.RecordEvent(recordCtx(), startupEvent())
	require.Eventually(t, func() bool {
		return out1.eventCount() == 1
	}, time.Second, 10*time.Millisecond)

	// ApplyConfig with the same (zero) config should be a no-op.
	rec.ApplyConfig(Config{})

	// Events still flow to the same output after no-op reload.
	rec.RecordEvent(recordCtx(), startupEvent())
	require.Eventually(t, func() bool {
		return out1.eventCount() == 2
	}, time.Second, 10*time.Millisecond)
}

func TestEventRecorderConfigEqual_OutputCount(t *testing.T) {
	a := Config{Outputs: []Output{{Type: OutputFile, Path: "/tmp/a"}}}
	b := Config{}
	require.False(t, configEqual(a, b),
		"configs with different output counts must compare unequal")
}

func TestEventRecorderConfigEqual_TypeMismatch(t *testing.T) {
	a := Config{Outputs: []Output{{Type: OutputFile, Path: "/tmp/a"}}}
	b := Config{Outputs: []Output{{Type: OutputWebhook}}}
	require.False(t, configEqual(a, b),
		"outputs of different types must compare unequal")
}

// Smoke test for the proto fast path in marshalAndSend: build a
// recorder whose only output is a fake ProtoDestination and assert
// it receives SendProto, not SendEvent, and that protojson is never
// invoked (we observe this indirectly by inspecting the captured event).
func TestMarshalAndSend_ProtoFastPath(t *testing.T) {
	pd := &fakeProtoDest{wantProto: true}
	rec := newTestRecorder(pd)
	defer rec.Close()

	rec.RecordEvent(recordCtx(), startupEvent())

	require.Eventually(t, func() bool {
		pd.mu.Lock()
		defer pd.mu.Unlock()
		return pd.protoCount == 1 && pd.jsonCount == 0
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		pd.mu.Lock()
		defer pd.mu.Unlock()
		return pd.last != nil && pd.last.GetData().GetAlertmanagerStartupEvent() != nil
	}, time.Second, 10*time.Millisecond)
}

// fakeProtoDest implements ProtoDestination and counts both code paths
// so the test can assert the fast path was taken.
type fakeProtoDest struct {
	mu         sync.Mutex
	wantProto  bool
	jsonCount  int
	protoCount int
	last       *eventrecorderpb.Event
}

func (f *fakeProtoDest) Name() string { return "fake:proto" }

func (f *fakeProtoDest) SendEvent(_ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jsonCount++
	return nil
}

func (f *fakeProtoDest) Close() error { return nil }

func (f *fakeProtoDest) WantsProto() bool { return f.wantProto }

func (f *fakeProtoDest) SendProto(ev *eventrecorderpb.Event) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.protoCount++
	f.last = ev
	return 42, nil
}
