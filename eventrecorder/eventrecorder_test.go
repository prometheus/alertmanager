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

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/pkg/labels"
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

func TestExtractEventType(t *testing.T) {
	tests := []struct {
		name     string
		event    *eventrecorderpb.EventData
		expected string
	}{
		{
			name:     "startup",
			event:    startupEvent(),
			expected: "alertmanager_startup_event",
		},
		{
			name: "shutdown",
			event: &eventrecorderpb.EventData{
				EventType: &eventrecorderpb.EventData_AlertmanagerShutdownEvent{},
			},
			expected: "alertmanager_shutdown_event",
		},
		{
			name: "alert_created",
			event: &eventrecorderpb.EventData{
				EventType: &eventrecorderpb.EventData_AlertCreated{},
			},
			expected: "alert_created",
		},
		{
			name:     "unknown",
			event:    &eventrecorderpb.EventData{},
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
	require.Equal(t, eventrecorderpb.Matcher_TYPE_REGEXP, proto.Type)
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
	require.Equal(t, eventrecorderpb.Matcher_TYPE_EQUAL, protos[0].Type)
	require.Equal(t, eventrecorderpb.Matcher_TYPE_NOT_EQUAL, protos[1].Type)
}

func TestEventRecorderConfigEqual(t *testing.T) {
	a := Config{
		Outputs: []Output{
			{Type: OutputFile, Path: "/tmp/events.jsonl"},
		},
	}
	b := Config{
		Outputs: []Output{
			{Type: OutputFile, Path: "/tmp/events.jsonl"},
		},
	}
	require.True(t, configEqual(a, b))

	b.Outputs[0].Path = "/tmp/other.jsonl"
	require.False(t, configEqual(a, b))

	// Different number of outputs.
	c := Config{}
	require.False(t, configEqual(a, c))
}

func TestEventRecorderConfigEqual_KafkaBrokerOrder(t *testing.T) {
	a := Config{Outputs: []Output{{
		Type:    OutputKafka,
		Brokers: []string{"b:9092", "a:9092"},
		Topic:   "t",
		Format:  KafkaFormatJSON,
	}}}
	b := Config{Outputs: []Output{{
		Type:    OutputKafka,
		Brokers: []string{"a:9092", "b:9092"},
		Topic:   "t",
		Format:  KafkaFormatJSON,
	}}}
	require.True(t, configEqual(a, b), "broker order must not affect equality")

	b.Outputs[0].Topic = "other"
	require.False(t, configEqual(a, b), "differing topics must compare unequal")

	b.Outputs[0].Topic = "t"
	b.Outputs[0].Format = KafkaFormatProtobuf
	require.False(t, configEqual(a, b), "differing formats must compare unequal")
}

func TestOutput_UnmarshalYAML_Kafka(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, o Output)
	}{
		{
			name: "valid minimal kafka",
			yaml: `
type: kafka
brokers: [a:9092, b:9092]
topic: amgr-events
`,
			check: func(t *testing.T, o Output) {
				require.Equal(t, OutputKafka, o.Type)
				require.Equal(t, "amgr-events", o.Topic)
				// Format defaults to "json" when omitted.
				require.Equal(t, "json", o.Format)
			},
		},
		{
			name: "valid full kafka",
			yaml: `
type: kafka
brokers: [a:9092]
topic: t
client_id: amgr
format: protobuf
acks: all
compression: zstd
buffer_size: 4096
`,
			check: func(t *testing.T, o Output) {
				require.Equal(t, KafkaFormatProtobuf, o.Format)
				require.Equal(t, KafkaAcksAll, o.Acks)
				require.Equal(t, KafkaCompressionZstd, o.Compression)
				require.Equal(t, 4096, o.BufferSize)
				require.Equal(t, "amgr", o.ClientID)
			},
		},
		{
			name:    "missing brokers",
			yaml:    "type: kafka\ntopic: t\n",
			wantErr: true,
		},
		{
			name:    "missing topic",
			yaml:    "type: kafka\nbrokers: [a:9092]\n",
			wantErr: true,
		},
		{
			name:    "empty broker entry",
			yaml:    "type: kafka\nbrokers: ['']\ntopic: t\n",
			wantErr: true,
		},
		{
			name:    "bad format",
			yaml:    "type: kafka\nbrokers: [a:9092]\ntopic: t\nformat: yaml\n",
			wantErr: true,
		},
		{
			name:    "bad acks",
			yaml:    "type: kafka\nbrokers: [a:9092]\ntopic: t\nacks: majority\n",
			wantErr: true,
		},
		{
			name:    "bad compression",
			yaml:    "type: kafka\nbrokers: [a:9092]\ntopic: t\ncompression: deflate\n",
			wantErr: true,
		},
		{
			name:    "unknown type",
			yaml:    "type: nats\n",
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
