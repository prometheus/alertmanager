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

// Package eventlog provides an audit log of significant Alertmanager
// events.  Events are serialized as JSON and fanned out to one or more
// configured outputs (JSONL file, webhook, etc.).
//
// RecordEvent never blocks the caller: events are serialized and
// placed on a bounded in-memory queue.  A background goroutine
// drains the queue and writes to outputs.  If the queue is full,
// events are dropped and a metric is incremented.
package eventlog

import (
	"io"
	"log/slog"
	"reflect"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventlog/eventlogpb"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/types"
)

const (
	// Maximum number of events buffered before new events are dropped.
	// At ~500 bytes per event this caps memory usage at roughly 4 MB.
	eventQueueSize = 8192
)

// Recorder is a concrete, non-nil-able handle to an event recorder.
// Because it is a struct (not an interface), passing nil where a
// Recorder is expected is a compile-time error.
//
// The zero value (Recorder{}) is safe to use and silently discards all
// events, but prefer NopRecorder() for clarity.
type Recorder struct {
	core *recorderCore
}

// writeRequest is a single event queued for background writing.
type writeRequest struct {
	data      []byte
	eventType string
}

// recorderCore holds the mutable state shared by all copies of a
// Recorder value.  Access is protected by mu.
type recorderCore struct {
	mu         sync.RWMutex
	outputs    []Output
	currentCfg config.EventLogConfig
	instance   string
	peer       *cluster.Peer
	logger     *slog.Logger
	metrics    *metrics

	// Async write queue.  nil for NopRecorder, non-nil for active.
	events chan writeRequest
	done   chan struct{}
	wg     sync.WaitGroup
}

// Output is a single event log destination.  Implementations receive
// pre-serialized JSON bytes and are responsible for delivering them.
type Output interface {
	WriteEvent(data []byte) error
	io.Closer
}

// NopRecorder returns a Recorder that silently discards all events.
// Use this in tests or when the event log is not configured.
func NopRecorder() Recorder {
	return Recorder{core: &recorderCore{}}
}

// metrics holds Prometheus metrics for the event log.
type metrics struct {
	eventsRecorded       *prometheus.CounterVec
	eventLogBytesWritten *prometheus.CounterVec
	eventsDropped        *prometheus.CounterVec
	eventSerializeErrors *prometheus.CounterVec
}

func newMetrics(r prometheus.Registerer) *metrics {
	eventsRecorded := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_recorded_total",
		Help: "The total number of events recorded by the event recorder.",
	}, []string{"event_type", "output", "result"})

	eventLogBytesWritten := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_log_bytes_written_total",
		Help: "The total number of bytes written to the event log.",
	}, []string{"event_type", "output"})

	eventsDropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_dropped_total",
		Help: "The total number of events dropped due to a full queue.",
	}, []string{"event_type"})

	eventSerializeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_serialize_errors_total",
		Help: "The total number of events that failed to serialize.",
	}, []string{"event_type"})

	if r != nil {
		r.MustRegister(eventsRecorded, eventLogBytesWritten, eventsDropped, eventSerializeErrors)
	}

	return &metrics{
		eventsRecorded:       eventsRecorded,
		eventLogBytesWritten: eventLogBytesWritten,
		eventsDropped:        eventsDropped,
		eventSerializeErrors: eventSerializeErrors,
	}
}

// NewRecorderFromConfig builds a Recorder from the given configuration.
// A background goroutine is started to drain the event queue; call
// Close to stop it.
func NewRecorderFromConfig(cfg config.EventLogConfig, instance string, logger *slog.Logger, r prometheus.Registerer) Recorder {
	core := &recorderCore{
		instance:   instance,
		logger:     logger,
		metrics:    newMetrics(r),
		currentCfg: cfg,
		events:     make(chan writeRequest, eventQueueSize),
		done:       make(chan struct{}),
	}
	core.outputs = buildOutputs(cfg.Outputs, logger)

	if r != nil {
		r.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "alertmanager_event_log_queue_length",
			Help: "Current number of events waiting in the event log write queue.",
		}, func() float64 {
			return float64(len(core.events))
		}))
	}

	core.wg.Add(1)
	go core.writeLoop()

	return Recorder{core: core}
}

// buildOutputs creates Output implementations from the given config.
func buildOutputs(cfgOutputs []config.EventLogOutput, logger *slog.Logger) []Output {
	var outputs []Output
	for _, out := range cfgOutputs {
		switch out.Type {
		case "file":
			fo, err := NewFileOutput(out.Path, logger)
			if err != nil {
				logger.Error("Failed to create file event log output", "path", out.Path, "err", err)
				continue
			}
			outputs = append(outputs, fo)
		case "webhook":
			wo, err := NewWebhookOutput(out, logger)
			if err != nil {
				logger.Error("Failed to create webhook event log output", "url", out.URL, "err", err)
				continue
			}
			outputs = append(outputs, wo)
		default:
			logger.Error("Unknown event log output type", "type", out.Type)
		}
	}
	return outputs
}

// writeLoop drains the event queue and writes to outputs.  It runs
// until the done channel is closed, then drains any remaining events.
func (c *recorderCore) writeLoop() {
	defer c.wg.Done()
	for {
		select {
		case req := <-c.events:
			c.writeToOutputs(req)
		case <-c.done:
			for {
				select {
				case req := <-c.events:
					c.writeToOutputs(req)
				default:
					return
				}
			}
		}
	}
}

// writeToOutputs sends a pre-serialized event to all current outputs.
func (c *recorderCore) writeToOutputs(req writeRequest) {
	c.mu.RLock()
	outputs := c.outputs
	c.mu.RUnlock()

	if len(outputs) == 0 {
		return
	}

	for i, out := range outputs {
		idx := strconv.Itoa(i)
		if writeErr := out.WriteEvent(req.data); writeErr != nil {
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, idx, "error").Inc()
			c.logger.Error("Failed to write event", "event_type", req.eventType, "output", i, "err", writeErr)
		} else {
			c.metrics.eventsRecorded.WithLabelValues(req.eventType, idx, "success").Inc()
			c.metrics.eventLogBytesWritten.WithLabelValues(req.eventType, idx).Add(float64(len(req.data)))
		}
	}
}

// RecordEvent serializes the event and places it on a bounded queue
// for background delivery.  If the queue is full the event is dropped
// (never blocks the caller).
func (r Recorder) RecordEvent(event *eventlogpb.EventData) {
	if r.core == nil || r.core.events == nil {
		return
	}

	eventType := extractEventType(event)

	wrappedEvent := &eventlogpb.Event{
		Timestamp: timestamppb.Now(),
		Instance:  r.core.instance,
		Data:      event,
	}

	r.core.mu.RLock()
	peer := r.core.peer
	r.core.mu.RUnlock()

	if peer != nil {
		wrappedEvent.ClusterPosition = uint32(peer.Position())
	}

	data, err := protojson.Marshal(wrappedEvent)
	if err != nil {
		r.core.metrics.eventSerializeErrors.WithLabelValues(eventType).Inc()
		r.core.logger.Error("Failed to marshal event", "event_type", eventType, "err", err)
		return
	}

	data = append(data, '\n')

	select {
	case r.core.events <- writeRequest{data: data, eventType: eventType}:
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
	r.core.mu.Lock()
	r.core.peer = peer
	r.core.mu.Unlock()
}

// ApplyConfig hot-reloads the event log configuration.  Old outputs
// are closed and new ones are created from the given config.
func (r Recorder) ApplyConfig(cfg config.EventLogConfig) {
	if r.core == nil || r.core.logger == nil {
		return
	}

	// Skip if the config hasn't changed to avoid closing and
	// recreating identical outputs.
	r.core.mu.Lock()
	if reflect.DeepEqual(cfg, r.core.currentCfg) {
		r.core.mu.Unlock()
		return
	}

	newOutputs := buildOutputs(cfg.Outputs, r.core.logger)
	oldOutputs := r.core.outputs
	r.core.outputs = newOutputs
	r.core.currentCfg = cfg
	r.core.mu.Unlock()

	for _, out := range oldOutputs {
		if err := out.Close(); err != nil {
			r.core.logger.Error("Failed to close old event log output", "err", err)
		}
	}

	r.core.logger.Info("Event log configuration reloaded", "outputs", len(newOutputs))
}

// Close signals the background goroutine to drain remaining events
// and stop, then closes all outputs.
func (r Recorder) Close() error {
	if r.core == nil {
		return nil
	}

	if r.core.done != nil {
		close(r.core.done)
		r.core.wg.Wait()
	}

	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	for _, out := range r.core.outputs {
		if err := out.Close(); err != nil && r.core.logger != nil {
			r.core.logger.Error("Failed to close event log output", "err", err)
		}
	}
	r.core.outputs = nil
	return nil
}

// extractEventType returns a human-readable name for the event type.
func extractEventType(event *eventlogpb.EventData) string {
	switch event.EventType.(type) {
	case *eventlogpb.EventData_AlertmanagerStartupEvent:
		return "startup"
	case *eventlogpb.EventData_AlertmanagerShutdownEvent:
		return "shutdown"
	case *eventlogpb.EventData_AlertCreated:
		return "alert_created"
	case *eventlogpb.EventData_AlertResolved:
		return "alert_resolved"
	case *eventlogpb.EventData_AlertGrouped:
		return "alert_grouped"
	case *eventlogpb.EventData_Notification:
		return "notification"
	case *eventlogpb.EventData_SilenceCreated:
		return "silence_created"
	case *eventlogpb.EventData_SilenceUpdated:
		return "silence_updated"
	case *eventlogpb.EventData_SilenceMutedAlert:
		return "silence_muted_alert"
	case *eventlogpb.EventData_InhibitionMutedAlert:
		return "inhibition_muted_alert"
	default:
		return "unknown"
	}
}

// LabelSetAsProto converts a model.LabelSet to an eventlogpb.LabelSet.
func LabelSetAsProto(ls model.LabelSet) *eventlogpb.LabelSet {
	pairs := make([]*eventlogpb.LabelPair, 0, len(ls))
	for k, v := range ls {
		pairs = append(pairs, &eventlogpb.LabelPair{Key: string(k), Value: string(v)})
	}
	return &eventlogpb.LabelSet{Labels: pairs}
}

// AlertAsProto converts a types.Alert to an eventlogpb.Alert.
func AlertAsProto(alert *types.Alert) *eventlogpb.Alert {
	return &eventlogpb.Alert{
		Fingerprint: uint64(alert.Fingerprint()),
		Name:        alert.Name(),
		Labels:      LabelSetAsProto(alert.Labels),
		Annotations: LabelSetAsProto(alert.Annotations),
		StartsAt:    timestamppb.New(alert.StartsAt),
		EndsAt:      timestamppb.New(alert.EndsAt),
		Resolved:    alert.Resolved(),
	}
}

func MatchersAsProto(matchers labels.Matchers) []*eventlogpb.Matcher {
	result := make([]*eventlogpb.Matcher, len(matchers))
	for i, m := range matchers {
		var matcherType eventlogpb.Matcher_Type
		switch m.Type {
		case labels.MatchEqual:
			matcherType = eventlogpb.Matcher_TYPE_EQUAL
		case labels.MatchNotEqual:
			matcherType = eventlogpb.Matcher_TYPE_NOT_EQUAL
		case labels.MatchRegexp:
			matcherType = eventlogpb.Matcher_TYPE_REGEXP
		case labels.MatchNotRegexp:
			matcherType = eventlogpb.Matcher_TYPE_NOT_REGEXP
		default:
			matcherType = eventlogpb.Matcher_TYPE_UNSPECIFIED
		}

		result[i] = &eventlogpb.Matcher{
			Type:     matcherType,
			Name:     m.Name,
			Pattern:  m.Value,
			Rendered: m.String(),
		}
	}
	return result
}
