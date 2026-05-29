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
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/kafka"
)

// --- helpers.

func testOutputDrops() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_event_output_drops_total",
	}, []string{"output"})
}

func testKafkaProduceErrors() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_event_kafka_produce_errors_total",
	}, []string{"output", "error_type"})
}

// startFakeCluster boots an in-process Kafka broker with the given
// topic pre-seeded so consumers can attach without auto-create races.
func startFakeCluster(t *testing.T, topic string, partitions int32) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(
		kfake.NumBrokers(1),
		kfake.SeedTopics(partitions, topic),
		kfake.AllowAutoTopicCreation(),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

// readRecords starts a consumer on the given topic and returns up to n
// records seen within timeout.
func readRecords(t *testing.T, brokers []string, topic string, n int, timeout time.Duration) []*kgo.Record {
	t.Helper()
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)
	defer cl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var out []*kgo.Record
	for len(out) < n {
		fetches := cl.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// Stop on context-cancel; surface other errors.
			for _, fe := range errs {
				if errors.Is(fe.Err, context.DeadlineExceeded) || errors.Is(fe.Err, context.Canceled) {
					return out
				}
				t.Fatalf("fetch error on %s/%d: %v", fe.Topic, fe.Partition, fe.Err)
			}
		}
		fetches.EachRecord(func(r *kgo.Record) {
			out = append(out, r)
		})
		if ctx.Err() != nil {
			return out
		}
	}
	return out
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

func sampleEvent() *eventrecorderpb.Event {
	return &eventrecorderpb.Event{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Instance:  "test-host",
		Data: &eventrecorderpb.EventData{
			EventType: &eventrecorderpb.EventData_AlertmanagerStartupEvent{
				AlertmanagerStartupEvent: &eventrecorderpb.AlertmanagerStartupEvent{
					Version:      "v-test",
					BuildContext: "test-build",
				},
			},
		},
	}
}

// --- tests.

func TestKafkaOutput_SendEvent_JSON(t *testing.T) {
	const topic = "amgr-events"
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: brokers,
			Topic:   topic,
			Format:  kafka.FormatJSON,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)

	// Marshal via the same path marshalAndSend would use.
	payload, err := protojson.Marshal(sampleEvent())
	require.NoError(t, err)
	payload = append(payload, '\n')

	require.NoError(t, ko.SendEvent(payload))
	require.NoError(t, ko.Close())

	records := readRecords(t, brokers, topic, 1, 5*time.Second)
	require.Len(t, records, 1)
	require.JSONEq(t, string(payload), string(records[0].Value))
	require.Equal(t, "test-host", string(records[0].Key))
}

func TestKafkaOutput_SendProto_Protobuf(t *testing.T) {
	const topic = "amgr-events-proto"
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: brokers,
			Topic:   topic,
			Format:  kafka.FormatProtobuf,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)
	require.True(t, ko.WantsProto())

	ev := sampleEvent()
	size, err := ko.SendProto(ev)
	require.NoError(t, err)
	require.Positive(t, size)

	require.NoError(t, ko.Close())

	records := readRecords(t, brokers, topic, 1, 5*time.Second)
	require.Len(t, records, 1)

	var got eventrecorderpb.Event
	require.NoError(t, proto.Unmarshal(records[0].Value, &got))
	require.Equal(t, ev.Instance, got.Instance)
	require.Equal(t, ev.Timestamp.AsTime().Unix(), got.Timestamp.AsTime().Unix())
	require.Equal(t, "v-test", got.GetData().GetAlertmanagerStartupEvent().GetVersion())
	require.Equal(t, "test-host", string(records[0].Key))
}

func TestKafkaOutput_KeyIsInstance(t *testing.T) {
	const topic = "amgr-events-key"
	// Single partition guarantees ordering by send order.
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: brokers,
			Topic:   topic,
			Format:  kafka.FormatJSON,
		},
		"instance-A",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)

	const n = 5
	for range n {
		require.NoError(t, ko.SendEvent([]byte(`{"i":1}`)))
	}
	require.NoError(t, ko.Close())

	records := readRecords(t, brokers, topic, n, 5*time.Second)
	require.Len(t, records, n)
	for _, r := range records {
		require.Equal(t, "instance-A", string(r.Key))
	}
}

func TestKafkaOutput_DropsOnFullBuffer(t *testing.T) {
	const topic = "amgr-events-drops"
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:       OutputKafka,
			Brokers:    brokers,
			Topic:      topic,
			Format:     kafka.FormatJSON,
			BufferSize: 1,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)

	// Stop the dispatcher so the channel is no longer drained, then
	// push records directly until the buffered-channel default branch
	// fires.  enqueue is gated by ko.closed (not ko.done), so sends
	// continue to land on the buffer while the output is still "open".
	close(ko.done)
	ko.wg.Wait()

	// First send fills the size-1 buffer; subsequent sends drop.
	require.NoError(t, ko.SendEvent([]byte("a")))
	for range 10 {
		require.NoError(t, ko.SendEvent([]byte("b")))
	}

	require.GreaterOrEqual(t, counterValue(t, ko.drops), float64(10))

	// Manually close the franz-go client; we already shut the
	// dispatcher down by closing ko.done above.
	ko.client.Close()
}

func TestKafkaOutput_SendAfterClose(t *testing.T) {
	const topic = "amgr-events-after-close"
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: brokers,
			Topic:   topic,
			Format:  kafka.FormatJSON,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)
	require.NoError(t, ko.Close())

	// SendEvent must report closure rather than silently dropping
	// onto a buffer that no dispatcher is draining.
	err = ko.SendEvent([]byte(`{"after":"close"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")

	// SendProto should behave the same way.
	_, err = ko.SendProto(sampleEvent())
	require.Error(t, err)
	require.Contains(t, err.Error(), "closed")
}

func TestKafkaOutput_CloseFlushesQueue(t *testing.T) {
	const topic = "amgr-events-flush"
	cluster := startFakeCluster(t, topic, 1)
	brokers := cluster.ListenAddrs()

	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: brokers,
			Topic:   topic,
			Format:  kafka.FormatJSON,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	require.NoError(t, err)

	const n = 10
	for range n {
		require.NoError(t, ko.SendEvent([]byte(`{"flush":true}`)))
	}
	require.NoError(t, ko.Close())

	records := readRecords(t, brokers, topic, n, 5*time.Second)
	require.Len(t, records, n)
}

func TestKafkaOutput_ContinuesOnInitialPingFailure(t *testing.T) {
	// Use a closed TCP port (likely-unreachable broker).  Construction
	// must succeed and Name() must be well-formed.  Importantly, the
	// constructor must NOT block on the ping timeout — that runs in
	// the background.
	start := time.Now()
	ko, err := NewKafkaOutput(
		Output{
			Type:    OutputKafka,
			Brokers: []string{"127.0.0.1:1"},
			Topic:   "no-broker",
			Format:  kafka.FormatJSON,
		},
		"test-host",
		testOutputDrops(),
		testKafkaProduceErrors(),
		slog.Default(),
	)
	constructDur := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, ko)
	require.Equal(t, "kafka:127.0.0.1:1/no-broker", ko.Name())

	// Construction must return well before the ping timeout (5s).
	// 1s is generous for CI but still 5x faster than the timeout.
	require.Less(t, constructDur, time.Second,
		"NewKafkaOutput must not block on broker reachability; took %s", constructDur)

	// Closing must also be fast even though the background ping is
	// still in flight: Close cancels the ping context.  Shrink the
	// flush budget so the test stays fast.
	ko.flushBudget = 200 * time.Millisecond
	closeStart := time.Now()
	require.NoError(t, ko.Close())
	require.Less(t, time.Since(closeStart), 2*time.Second,
		"Close must abort the in-flight ping")
}

func TestKafkaOutput_RejectsBadConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Output
	}{
		{
			name: "no brokers",
			cfg:  Output{Type: OutputKafka, Topic: "t", Format: kafka.FormatJSON},
		},
		{
			name: "empty broker entry",
			cfg: Output{
				Type:    OutputKafka,
				Brokers: []string{"127.0.0.1:9092", ""},
				Topic:   "t",
				Format:  kafka.FormatJSON,
			},
		},
		{
			name: "no topic",
			cfg:  Output{Type: OutputKafka, Brokers: []string{"127.0.0.1:9092"}, Format: kafka.FormatJSON},
		},
		{
			name: "bad format",
			cfg:  Output{Type: OutputKafka, Brokers: []string{"127.0.0.1:9092"}, Topic: "t", Format: "yaml"},
		},
		{
			name: "bad acks",
			cfg: Output{
				Type: OutputKafka, Brokers: []string{"127.0.0.1:9092"}, Topic: "t",
				Format: kafka.FormatJSON, Acks: "majority",
			},
		},
		{
			name: "bad compression",
			cfg: Output{
				Type: OutputKafka, Brokers: []string{"127.0.0.1:9092"}, Topic: "t",
				Format: kafka.FormatJSON, Compression: "deflate",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKafkaOutput(tc.cfg, "h", testOutputDrops(), testKafkaProduceErrors(), slog.Default())
			require.Error(t, err)
		})
	}
}

func TestKafkaOutput_NameIsStable(t *testing.T) {
	// The Name() format ("kafka:<sorted-brokers>/<topic>") is composed
	// here in eventrecorder; broker-list sorting is handled by the
	// shared kafka package (and tested there).  This test pins the
	// composition formula so reordering brokers in YAML doesn't change
	// the Prometheus label value.
	const topic = "topic"
	a := "kafka:" + kafka.BrokerList([]string{"b:9092", "a:9092"}) + "/" + topic
	b := "kafka:" + kafka.BrokerList([]string{"a:9092", "b:9092"}) + "/" + topic
	require.Equal(t, a, b)
	require.Equal(t, "kafka:a:9092,b:9092/topic", a)
}

// --- config tests.

func TestEventRecorderConfigEqual_KafkaBrokerOrder(t *testing.T) {
	a := Config{Outputs: []Output{{
		Type:    OutputKafka,
		Brokers: []string{"b:9092", "a:9092"},
		Topic:   "t",
		Format:  kafka.FormatJSON,
	}}}
	b := Config{Outputs: []Output{{
		Type:    OutputKafka,
		Brokers: []string{"a:9092", "b:9092"},
		Topic:   "t",
		Format:  kafka.FormatJSON,
	}}}
	require.True(t, configEqual(a, b), "broker order must not affect equality")

	b.Outputs[0].Topic = "other"
	require.False(t, configEqual(a, b), "differing topics must compare unequal")

	b.Outputs[0].Topic = "t"
	b.Outputs[0].Format = kafka.FormatProtobuf
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
				require.Equal(t, kafka.FormatProtobuf, o.Format)
				require.Equal(t, kafka.AcksAll, o.Acks)
				require.Equal(t, kafka.CompressionZstd, o.Compression)
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
