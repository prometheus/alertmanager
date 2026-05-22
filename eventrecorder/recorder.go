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

// Package eventrecorder provides a structured event recorder for
// significant Alertmanager events.  Events are serialized as JSON and
// fanned out to one or more configured destinations (JSONL file,
// webhook, kafka).
//
// RecordEvent never blocks the caller: events are serialized and
// placed on a bounded in-memory queue.  A background goroutine
// drains the queue and sends to destinations.  If the queue is full,
// events are dropped and a metric is incremented.
//
// Package layout:
//
//   - eventrecorder.go    Recorder core: types, write loop, fan-out.
//   - metrics.go          Prometheus metric definitions.
//   - events.go           Pure proto-conversion helpers and event constructors.
//   - config.go           Output struct + per-type validation/equality dispatch.
//   - file.go             File output and its config validators.
//   - webhook.go          Webhook output and its config validators.
//   - kafka.go            Kafka output and its config validators.
package eventrecorder

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
)

// Output type identifiers used in the YAML configuration.
const (
	OutputFile    = "file"
	OutputWebhook = "webhook"
	OutputKafka   = "kafka"
)

const (
	// Maximum number of events buffered before new events are dropped.
	// At ~500 bytes per event this caps memory usage at roughly 4 MB.
	eventQueueSize = 8192
)

type recordingEnabledContextKey struct{}

// WithEventRecording returns a context that enables event recording.
// By default, event recording is disabled; callers must opt in by
// decorating their context with this function.
func WithEventRecording(ctx context.Context) context.Context {
	return context.WithValue(ctx, recordingEnabledContextKey{}, true)
}

// EventRecordingEnabled reports whether event recording has been
// enabled in the given context via WithEventRecording.
func EventRecordingEnabled(ctx context.Context) bool {
	v, _ := ctx.Value(recordingEnabledContextKey{}).(bool)
	return v
}

// Recorder is a concrete, non-nil-able handle to an event recorder.
// Because it is a struct (not an interface), passing nil where a
// Recorder is expected is a compile-time error.
//
// The zero value (Recorder{}) is safe to use and silently discards all
// events, but prefer NopRecorder() for clarity.
type Recorder struct {
	core *sharedRecorder
}

// writeRequest is a single event queued for background serialization
// and writing.  It carries the proto message so that the expensive
// protojson.Marshal call happens in the write-loop goroutine, not on
// the caller's hot path.
type writeRequest struct {
	event     *eventrecorderpb.Event
	eventType string
}

// sharedRecorder holds the mutable state shared by all copies of a
// Recorder value.  Mutable state (outputs, currentCfg) is owned
// exclusively by the writeLoop goroutine and updated via the
// cfgUpdate channel, eliminating the need for a mutex.
type sharedRecorder struct {
	instance string
	logger   *slog.Logger
	metrics  *metrics
	peer     atomic.Pointer[cluster.Peer]

	// Async write queue.  nil for NopRecorder, non-nil for active.
	events    chan writeRequest
	cfgUpdate chan cfgUpdateMsg
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// cfgUpdateMsg is sent to writeLoop to hot-reload the configuration.
// The sender blocks until the writeLoop acknowledges by closing done.
type cfgUpdateMsg struct {
	cfg  Config
	done chan struct{}
}

// Destination is a single event destination.  Implementations receive
// pre-serialized JSON bytes and are responsible for delivering them.
type Destination interface {
	// Name returns a stable identifier for this destination, suitable
	// for use as a Prometheus label value (e.g. "file:/var/log/events.jsonl"
	// or "webhook:https://example.com/hook").
	Name() string
	SendEvent(data []byte) error
	io.Closer
}

// ProtoDestination is an optional extension of Destination for outputs
// that want to consume events as protobuf messages directly, skipping
// the JSON serialization step.  Outputs that implement this interface
// and return true from WantsProto receive SendProto calls instead of
// SendEvent; SendEvent must remain implemented for fallback callers.
type ProtoDestination interface {
	Destination
	// WantsProto returns true if this destination prefers SendProto
	// over SendEvent.  Implementations may return false to fall back
	// to the JSON path (useful when format is configurable per output).
	WantsProto() bool
	// SendProto delivers the event in its native proto form.  The
	// implementation is responsible for any encoding (e.g. proto.Marshal).
	// The returned size is the number of payload bytes ultimately
	// written to the wire, used for the bytes-written metric.
	SendProto(event *eventrecorderpb.Event) (size int, err error)
}

// NopRecorder returns a Recorder that silently discards all events.
// Use this in tests or when the event recorder is not configured.
func NopRecorder() Recorder {
	return Recorder{core: &sharedRecorder{}}
}

// NewRecorderFromConfig builds a Recorder from the given configuration.
// A background goroutine is started to drain the event queue; call
// Close to stop it.
func NewRecorderFromConfig(cfg Config, instance string, logger *slog.Logger, r prometheus.Registerer) Recorder {
	core := &sharedRecorder{
		instance:  instance,
		logger:    logger,
		metrics:   newMetrics(r),
		events:    make(chan writeRequest, eventQueueSize),
		cfgUpdate: make(chan cfgUpdateMsg),
		done:      make(chan struct{}),
	}
	initialOutputs := buildOutputs(cfg.Outputs, instance, core.metrics, logger)

	if r != nil {
		r.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "alertmanager_event_recorder_queue_length",
			Help: "Current number of events waiting in the event recorder write queue.",
		}, func() float64 {
			return float64(len(core.events))
		}))
	}

	core.wg.Add(1)
	go core.writeLoop(initialOutputs, cfg)

	return Recorder{core: core}
}

// buildOutputs creates Destination implementations from the given config.
func buildOutputs(cfgOutputs []Output, instance string, m *metrics, logger *slog.Logger) []Destination {
	var outputs []Destination
	for _, out := range cfgOutputs {
		switch out.Type {
		case OutputFile:
			fo, err := NewFileOutput(out.Path, logger)
			if err != nil {
				logger.Error("Failed to create file event recorder output", "path", out.Path, "err", err)
				continue
			}
			outputs = append(outputs, fo)
		case OutputWebhook:
			wo, err := NewWebhookOutput(out, m.outputDrops, logger)
			if err != nil {
				logger.Error("Failed to create webhook event recorder output", "url", out.URL, "err", err)
				continue
			}
			outputs = append(outputs, wo)
		case OutputKafka:
			ko, err := NewKafkaOutput(out, instance, m.outputDrops, m.kafkaProduceErrors, logger)
			if err != nil {
				logger.Error("Failed to create kafka event recorder output", "brokers", out.Brokers, "topic", out.Topic, "err", err)
				continue
			}
			outputs = append(outputs, ko)
		default:
			logger.Error("Unknown event recorder output type", "type", out.Type)
		}
	}
	return outputs
}

// writeLoop drains the event queue, serializes events, and writes to
// outputs.  It owns the outputs and currentCfg exclusively — all
// mutations arrive via the cfgUpdate channel, so no mutex is needed.
//
// The protojson.Marshal runs here (not in the caller goroutine) so that
// the serialization cost is off the alert-processing hot path.
//
// It runs until the done channel is closed, then drains remaining
// events and closes all outputs before returning.
func (c *sharedRecorder) writeLoop(outputs []Destination, currentCfg Config) {
	defer c.wg.Done()
	defer func() {
		for _, out := range outputs {
			if err := out.Close(); err != nil && c.logger != nil {
				c.logger.Error("Failed to close event recorder output", "err", err)
			}
		}
	}()

	for {
		select {
		case req := <-c.events:
			c.marshalAndSend(req, outputs)
		case update := <-c.cfgUpdate:
			if !configEqual(update.cfg, currentCfg) {
				newOutputs := buildOutputs(update.cfg.Outputs, c.instance, c.metrics, c.logger)
				if len(newOutputs) != len(update.cfg.Outputs) {
					// Some outputs failed to initialize.  Keep the existing
					// (known-good) set rather than risking partial coverage.
					c.logger.Error("Failed to reload event recorder outputs; keeping existing outputs")
					for _, out := range newOutputs {
						if err := out.Close(); err != nil {
							c.logger.Error("Failed to close partially-built event recorder output", "err", err)
						}
					}
					close(update.done)
					continue
				}
				oldOutputs := outputs
				outputs = newOutputs
				currentCfg = update.cfg
				for _, out := range oldOutputs {
					if err := out.Close(); err != nil {
						c.logger.Error("Failed to close old event recorder output", "err", err)
					}
				}
				c.logger.Info("Event recorder configuration reloaded", "outputs", len(outputs))
			}
			close(update.done)
		case <-c.done:
			// Drain remaining events and any pending config updates.
			for {
				select {
				case req := <-c.events:
					c.marshalAndSend(req, outputs)
				case update := <-c.cfgUpdate:
					close(update.done)
				default:
					return
				}
			}
		}
	}
}

// marshalAndSend fans the queued event out to all outputs.  JSON
// serialization is performed lazily — it only happens if at least one
// destination wants the JSON representation.  Destinations that
// implement ProtoDestination and return true from WantsProto receive
// SendProto and skip the JSON path entirely.
func (c *sharedRecorder) marshalAndSend(req writeRequest, outputs []Destination) {
	var (
		jsonData       []byte
		jsonMarshalErr error
		jsonMarshalled bool
	)

	for _, out := range outputs {
		name := out.Name()

		if pd, ok := out.(ProtoDestination); ok && pd.WantsProto() {
			size, writeErr := pd.SendProto(req.event)
			if writeErr != nil {
				c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "error").Inc()
				c.logger.Error("Failed to write event (proto)", "event_type", req.eventType, "output", name, "err", writeErr)
				continue
			}
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "success").Inc()
			c.metrics.eventRecorderBytesWritten.WithLabelValues(req.eventType, name).Add(float64(size))
			continue
		}

		if !jsonMarshalled {
			jsonData, jsonMarshalErr = protojson.Marshal(req.event)
			if jsonMarshalErr == nil {
				jsonData = append(jsonData, '\n')
			}
			jsonMarshalled = true
		}
		if jsonMarshalErr != nil {
			c.metrics.eventSerializeErrors.WithLabelValues(req.eventType).Inc()
			c.logger.Error("Failed to marshal event", "event_type", req.eventType, "err", jsonMarshalErr)
			// Don't try further JSON destinations with bad data.
			return
		}

		if writeErr := out.SendEvent(jsonData); writeErr != nil {
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "error").Inc()
			c.logger.Error("Failed to write event", "event_type", req.eventType, "output", name, "err", writeErr)
		} else {
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "success").Inc()
			c.metrics.eventRecorderBytesWritten.WithLabelValues(req.eventType, name).Add(float64(len(jsonData)))
		}
	}
}

// RecordEvent wraps the event and places it on a bounded queue for
// background serialization and delivery.  If the queue is full the
// event is dropped (never blocks the caller).  Recording only occurs
// when the context has been decorated with WithEventRecording.
//
// The expensive protojson.Marshal call is deferred to the write-loop
// goroutine so that the caller's hot path only pays for the proto
// wrapping and a channel send.
func (r Recorder) RecordEvent(ctx context.Context, event *eventrecorderpb.EventData) {
	if r.core == nil || r.core.events == nil {
		return
	}
	if !EventRecordingEnabled(ctx) {
		return
	}

	eventType := extractEventType(event)

	wrappedEvent := &eventrecorderpb.Event{
		Timestamp: timestamppb.Now(),
		Instance:  r.core.instance,
		Data:      event,
	}

	if peer := r.core.peer.Load(); peer != nil {
		wrappedEvent.ClusterPosition = uint32(peer.Position())
	}

	select {
	case r.core.events <- writeRequest{event: wrappedEvent, eventType: eventType}:
	default:
		// Queue full; drop event to avoid blocking alertmanager.
		r.core.metrics.eventsDropped.WithLabelValues(eventType).Inc()
	}
}

// SetClusterPeer sets the cluster peer for HA position tracking.
func (r Recorder) SetClusterPeer(peer *cluster.Peer) {
	if r.core == nil {
		return
	}
	r.core.peer.Store(peer)
}

// ApplyConfig hot-reloads the event recorder configuration.  The update is
// sent to the writeLoop goroutine, which owns the outputs; this method
// blocks until the writeLoop has acknowledged the update.
func (r Recorder) ApplyConfig(cfg Config) {
	if r.core == nil || r.core.cfgUpdate == nil {
		return
	}
	ack := make(chan struct{})
	select {
	case r.core.cfgUpdate <- cfgUpdateMsg{cfg: cfg, done: ack}:
		<-ack
	case <-r.core.done:
		// Shutting down; ignore config update.
	}
}

// Close signals the background goroutine to drain remaining events
// and stop.  The writeLoop closes all outputs before returning.
func (r Recorder) Close() error {
	if r.core == nil || r.core.done == nil {
		return nil
	}
	r.core.closeOnce.Do(func() {
		close(r.core.done)
	})
	r.core.wg.Wait()
	return nil
}
