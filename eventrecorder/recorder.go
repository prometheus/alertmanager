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
// fanned out to one or more configured destinations.
//
// RecordEvent never blocks the caller: events are serialized and
// placed on a bounded in-memory queue.  A background goroutine
// drains the queue and sends to destinations.  If the queue is full,
// events are dropped and a metric is incremented.
package eventrecorder

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
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

// Destination is a single event destination.  Each implementation
// owns its own serialization: it receives the structured event and is
// responsible for encoding it (e.g. JSON or protobuf) and delivering it.
//
// Owning serialization per destination — rather than handing every
// destination a pre-encoded JSON blob — avoids the footgun of, say, a
// protobuf-configured Kafka output silently shipping a JSON payload.
type Destination interface {
	// Name returns a stable identifier for this destination, suitable
	// for use as a Prometheus label value (e.g. "file:/var/log/events.jsonl"
	// or "webhook:https://example.com/hook").
	Name() string
	// SendEvent encodes and delivers the event.  It returns the number
	// of payload bytes written (for the bytes-written metric) and any
	// delivery error.  A serialization failure should be returned
	// wrapped in *serializeError so the recorder can attribute it to
	// the serialize-errors metric.
	SendEvent(event *eventrecorderpb.Event) (size int, err error)
	io.Closer
}

// serializeError marks a failure to encode an event (as opposed to a
// delivery failure) so marshalAndSend can attribute it to the
// serialize-errors metric.  Destinations wrap encoding failures in it.
type serializeError struct{ err error }

func (e *serializeError) Error() string { return e.err.Error() }
func (e *serializeError) Unwrap() error { return e.err }

// NopRecorder returns a Recorder that silently discards all events.
// Use this in tests or when the event recorder is not configured.
func NopRecorder() Recorder {
	return Recorder{core: &sharedRecorder{}}
}

// NewRecorderFromConfig builds a Recorder from the given configuration.
// A background goroutine is started to drain the event queue; call
// Close to stop it.
func NewRecorderFromConfig(cfg Config, instance string, logger *slog.Logger, r prometheus.Registerer) Recorder {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	core := &sharedRecorder{
		instance:  instance,
		logger:    logger,
		metrics:   newMetrics(r),
		events:    make(chan writeRequest, eventQueueSize),
		cfgUpdate: make(chan cfgUpdateMsg),
		done:      make(chan struct{}),
	}
	initialOutputs := buildOutputs(cfg, instance, core.metrics, logger)

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
func buildOutputs(cfg Config, instance string, m *metrics, logger *slog.Logger) []Destination {
	var outputs []Destination
	for _, fc := range cfg.FileOutputs {
		fo, err := NewFileOutput(fc.Path, logger)
		if err != nil {
			logger.Error("Failed to create file event recorder output", "path", fc.Path, "err", err)
			continue
		}
		outputs = append(outputs, fo)
	}
	for _, wc := range cfg.WebhookOutputs {
		wo, err := NewWebhookOutput(wc, m.outputDrops, logger)
		if err != nil {
			logger.Error("Failed to create webhook event recorder output", "url", sanitizeSecretURL(wc.URL), "err", err)
			continue
		}
		outputs = append(outputs, wo)
	}
	for _, kc := range cfg.KafkaOutputs {
		ko, err := NewKafkaOutput(kc, instance, m.outputDrops, m.kafkaProduceErrors, logger)
		if err != nil {
			logger.Error("Failed to create kafka event recorder output", "brokers", kc.Brokers, "topic", kc.Topic, "err", err)
			continue
		}
		outputs = append(outputs, ko)
	}
	for range cfg.StdoutOutputs {
		outputs = append(outputs, &StdoutOutput{})
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
				newOutputs := buildOutputs(update.cfg, c.instance, c.metrics, c.logger)
				if len(newOutputs) != update.cfg.totalOutputs() {
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

// marshalAndSend fans the queued event out to all outputs.  Each
// destination owns its own serialization, so the recorder hands every
// output the structured event and records per-output result and
// bytes-written metrics from the returned size/error.
func (c *sharedRecorder) marshalAndSend(req writeRequest, outputs []Destination) {
	for _, out := range outputs {
		name := out.Name()
		size, err := out.SendEvent(req.event)
		if err != nil {
			var se *serializeError
			if errors.As(err, &se) {
				c.metrics.eventSerializeErrors.WithLabelValues(req.eventType).Inc()
			}
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "error").Inc()
			c.logger.Error("Failed to write event", "event_type", req.eventType, "output", name, "err", err)
			continue
		}
		c.metrics.eventsRecorded.WithLabelValues(req.eventType, name, "success").Inc()
		c.metrics.eventRecorderBytesWritten.WithLabelValues(req.eventType, name).Add(float64(size))
	}
}

// RecordEvent wraps the event and places it on a bounded queue for
// background serialization and delivery.  If the queue is full the
// event is dropped (never blocks the caller).  Recording only occurs
// when the context has been decorated with WithEventRecording.
//
// The event is supplied as a builder function rather than a value so
// that callers on hot read paths do not pay to construct an event
// (protobuf conversions, fingerprint slices, etc.) that would only be
// discarded when recording is disabled.  The builder is invoked only
// after the recording gates pass, and exactly once.
//
// The expensive protojson.Marshal call is deferred to the write-loop
// goroutine so that the caller's hot path only pays for the proto
// wrapping and a channel send.
func (r Recorder) RecordEvent(ctx context.Context, build func() *eventrecorderpb.EventData) {
	if r.core == nil || r.core.events == nil {
		return
	}
	if !EventRecordingEnabled(ctx) {
		return
	}

	event := build()
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
