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
// webhook, etc.).
//
// RecordEvent never blocks the caller: events are serialized and
// placed on a bounded in-memory queue.  A background goroutine
// drains the queue and sends to destinations.  If the queue is full,
// events are dropped and a metric is incremented.
package eventrecorder

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/eventrecorder/eventrecorderpb"
	"github.com/prometheus/alertmanager/pkg/labels"
	silencepb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

const (
	OutputFile    string = "file"
	OutputWebhook string = "webhook"
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

// NopRecorder returns a Recorder that silently discards all events.
// Use this in tests or when the event recorder is not configured.
func NopRecorder() Recorder {
	return Recorder{core: &sharedRecorder{}}
}

// metrics holds Prometheus metrics for the event recorder.
type metrics struct {
	eventsRecorded            *prometheus.CounterVec
	eventRecorderBytesWritten *prometheus.CounterVec
	eventsDropped             *prometheus.CounterVec
	eventSerializeErrors      *prometheus.CounterVec
	webhookDrops              *prometheus.CounterVec
}

func newMetrics(r prometheus.Registerer) *metrics {
	eventsRecorded := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_recorded_total",
		Help: "The total number of events recorded by the event recorder.",
	}, []string{"event_type", "output", "result"})

	eventRecorderBytesWritten := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_recorder_bytes_written_total",
		Help: "The total number of bytes written to the event recorder.",
	}, []string{"event_type", "output"})

	eventsDropped := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_events_dropped_total",
		Help: "The total number of events dropped due to a full queue.",
	}, []string{"event_type"})

	eventSerializeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_serialize_errors_total",
		Help: "The total number of events that failed to serialize.",
	}, []string{"event_type"})

	webhookDrops := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_event_webhook_drops_total",
		Help: "The total number of events dropped by a webhook output due to a full queue.",
	}, []string{"output"})

	if r != nil {
		r.MustRegister(eventsRecorded, eventRecorderBytesWritten, eventsDropped, eventSerializeErrors, webhookDrops)
	}

	return &metrics{
		eventsRecorded:            eventsRecorded,
		eventRecorderBytesWritten: eventRecorderBytesWritten,
		eventsDropped:             eventsDropped,
		eventSerializeErrors:      eventSerializeErrors,
		webhookDrops:              webhookDrops,
	}
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
	initialOutputs := buildOutputs(cfg.Outputs, core.metrics, logger)

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
func buildOutputs(cfgOutputs []Output, m *metrics, logger *slog.Logger) []Destination {
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
			wo, err := NewWebhookOutput(out, m.webhookDrops, logger)
			if err != nil {
				logger.Error("Failed to create webhook event recorder output", "url", out.URL, "err", err)
				continue
			}
			outputs = append(outputs, wo)
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
			if !eventRecorderConfigEqual(update.cfg, currentCfg) {
				newOutputs := buildOutputs(update.cfg.Outputs, c.metrics, c.logger)
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

// marshalAndSend serializes a queued event and fans it out to all outputs.
func (c *sharedRecorder) marshalAndSend(req writeRequest, outputs []Destination) {
	data, err := protojson.Marshal(req.event)
	if err != nil {
		c.metrics.eventSerializeErrors.WithLabelValues(req.eventType).Inc()
		c.logger.Error("Failed to marshal event", "event_type", req.eventType, "err", err)
		return
	}
	data = append(data, '\n')
	sendToOutputs(data, req.eventType, outputs, c.metrics, c.logger)
}

// sendToOutputs sends a pre-serialized event to all outputs.
func sendToOutputs(data []byte, eventType string, outputs []Destination, m *metrics, logger *slog.Logger) {
	for _, out := range outputs {
		name := out.Name()
		if writeErr := out.SendEvent(data); writeErr != nil {
			m.eventsRecorded.WithLabelValues(eventType, name, "error").Inc()
			logger.Error("Failed to write event", "event_type", eventType, "output", name, "err", writeErr)
		} else {
			m.eventsRecorded.WithLabelValues(eventType, name, "success").Inc()
			m.eventRecorderBytesWritten.WithLabelValues(eventType, name).Add(float64(len(data)))
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

// extractEventType returns the proto oneof field name for the event
// type (e.g. "alert_created", "notification").  It uses a type switch
// on the generated oneof wrapper types, avoiding proto reflection.
func extractEventType(event *eventrecorderpb.EventData) string {
	switch event.EventType.(type) {
	case *eventrecorderpb.EventData_AlertmanagerStartupEvent:
		return "alertmanager_startup_event"
	case *eventrecorderpb.EventData_AlertmanagerShutdownEvent:
		return "alertmanager_shutdown_event"
	case *eventrecorderpb.EventData_AlertCreated:
		return "alert_created"
	case *eventrecorderpb.EventData_AlertResolved:
		return "alert_resolved"
	case *eventrecorderpb.EventData_AlertGrouped:
		return "alert_grouped"
	case *eventrecorderpb.EventData_Notification:
		return "notification"
	case *eventrecorderpb.EventData_SilenceCreated:
		return "silence_created"
	case *eventrecorderpb.EventData_SilenceUpdated:
		return "silence_updated"
	case *eventrecorderpb.EventData_SilenceMutedAlert:
		return "silence_muted_alert"
	case *eventrecorderpb.EventData_InhibitionMutedAlert:
		return "inhibition_muted_alert"
	default:
		return "unknown"
	}
}

// LabelSetAsProto converts a model.LabelSet to an eventrecorderpb.LabelSet.
// Labels are sorted by name for deterministic output.
func LabelSetAsProto(ls model.LabelSet) *eventrecorderpb.LabelSet {
	names := make([]model.LabelName, 0, len(ls))
	for k := range ls {
		names = append(names, k)
	}
	slices.SortFunc(names, func(a, b model.LabelName) int {
		return strings.Compare(string(a), string(b))
	})
	pairs := make([]*eventrecorderpb.LabelPair, 0, len(ls))
	for _, k := range names {
		pairs = append(pairs, &eventrecorderpb.LabelPair{Key: string(k), Value: string(ls[k])})
	}
	return &eventrecorderpb.LabelSet{Labels: pairs}
}

// AlertAsProto converts a types.Alert to an eventrecorderpb.Alert.
func AlertAsProto(alert *types.Alert) *eventrecorderpb.Alert {
	return &eventrecorderpb.Alert{
		Fingerprint: uint64(alert.Fingerprint()),
		Name:        alert.Name(),
		Labels:      LabelSetAsProto(alert.Labels),
		Annotations: LabelSetAsProto(alert.Annotations),
		StartsAt:    timestamppb.New(alert.StartsAt),
		EndsAt:      timestamppb.New(alert.EndsAt),
		Resolved:    alert.Resolved(),
	}
}

// MatcherAsProto converts a single *labels.Matcher to its protobuf
// representation.
func MatcherAsProto(m *labels.Matcher) *eventrecorderpb.Matcher {
	var matcherType eventrecorderpb.Matcher_Type
	switch m.Type {
	case labels.MatchEqual:
		matcherType = eventrecorderpb.Matcher_TYPE_EQUAL
	case labels.MatchNotEqual:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_EQUAL
	case labels.MatchRegexp:
		matcherType = eventrecorderpb.Matcher_TYPE_REGEXP
	case labels.MatchNotRegexp:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_REGEXP
	default:
		matcherType = eventrecorderpb.Matcher_TYPE_UNSPECIFIED
	}
	return &eventrecorderpb.Matcher{
		Type:     matcherType,
		Name:     m.Name,
		Pattern:  m.Value,
		Rendered: m.String(),
	}
}

// MatchersAsProto converts a slice of matchers to their protobuf
// representations.
func MatchersAsProto(matchers labels.Matchers) []*eventrecorderpb.Matcher {
	result := make([]*eventrecorderpb.Matcher, len(matchers))
	for i, m := range matchers {
		result[i] = MatcherAsProto(m)
	}
	return result
}

// SilenceMatcherAsProto converts a silencepb.Matcher to an
// eventrecorderpb.Matcher.
func SilenceMatcherAsProto(m *silencepb.Matcher) *eventrecorderpb.Matcher {
	var matcherType eventrecorderpb.Matcher_Type
	switch m.Type {
	case silencepb.Matcher_EQUAL:
		matcherType = eventrecorderpb.Matcher_TYPE_EQUAL
	case silencepb.Matcher_REGEXP:
		matcherType = eventrecorderpb.Matcher_TYPE_REGEXP
	case silencepb.Matcher_NOT_EQUAL:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_EQUAL
	case silencepb.Matcher_NOT_REGEXP:
		matcherType = eventrecorderpb.Matcher_TYPE_NOT_REGEXP
	default:
		matcherType = eventrecorderpb.Matcher_TYPE_UNSPECIFIED
	}

	var rendered string
	var matchType labels.MatchType
	switch m.Type {
	case silencepb.Matcher_EQUAL:
		matchType = labels.MatchEqual
	case silencepb.Matcher_NOT_EQUAL:
		matchType = labels.MatchNotEqual
	case silencepb.Matcher_REGEXP:
		matchType = labels.MatchRegexp
	case silencepb.Matcher_NOT_REGEXP:
		matchType = labels.MatchNotRegexp
	default:
		matchType = labels.MatchEqual
	}
	if lm, err := labels.NewMatcher(matchType, m.Name, m.Pattern); err == nil {
		rendered = lm.String()
	}

	return &eventrecorderpb.Matcher{
		Type:     matcherType,
		Name:     m.Name,
		Pattern:  m.Pattern,
		Rendered: rendered,
	}
}

// SilenceAsProto converts a silencepb.Silence to an
// eventrecorderpb.Silence.
func SilenceAsProto(sil *silencepb.Silence) *eventrecorderpb.Silence {
	matcherSets := make([]*eventrecorderpb.MatcherSet, len(sil.MatcherSets))
	for i, ms := range sil.MatcherSets {
		matcherSet := &eventrecorderpb.MatcherSet{
			Matchers: make([]*eventrecorderpb.Matcher, len(ms.Matchers)),
		}
		for j, m := range ms.Matchers {
			matcherSet.Matchers[j] = SilenceMatcherAsProto(m)
		}
		matcherSets[i] = matcherSet
	}

	var matchers []*eventrecorderpb.Matcher
	if len(matcherSets) > 0 {
		matchers = matcherSets[0].Matchers
	}

	return &eventrecorderpb.Silence{
		Id:          sil.Id,
		Matchers:    matchers,
		MatcherSets: matcherSets,
		StartsAt:    sil.StartsAt,
		EndsAt:      sil.EndsAt,
		UpdatedAt:   sil.UpdatedAt,
		CreatedBy:   sil.CreatedBy,
		Comment:     sil.Comment,
	}
}

// InhibitRuleAsProto converts inhibit rule fields to an
// eventrecorderpb.InhibitRule.  It accepts the individual fields rather
// than the InhibitRule struct to avoid an import cycle.
func InhibitRuleAsProto(sourceMatchers, targetMatchers labels.Matchers, equal map[model.LabelName]struct{}) *eventrecorderpb.InhibitRule {
	equalLabels := make([]string, 0, len(equal))
	for label := range equal {
		equalLabels = append(equalLabels, string(label))
	}
	slices.Sort(equalLabels)
	return &eventrecorderpb.InhibitRule{
		SourceMatchers: MatchersAsProto(sourceMatchers),
		TargetMatchers: MatchersAsProto(targetMatchers),
		EqualLabels:    equalLabels,
	}
}

// NewAlertCreatedEvent constructs an AlertCreated event.
func NewAlertCreatedEvent(alert *types.Alert) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_AlertCreated{
			AlertCreated: &eventrecorderpb.AlertCreatedEvent{
				Alert: AlertAsProto(alert),
			},
		},
	}
}

// NewSilenceMutedAlertEvent constructs a SilenceMutedAlert event.
func NewSilenceMutedAlertEvent(silence *eventrecorderpb.Silence, fp model.Fingerprint, lset model.LabelSet) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceMutedAlert{
			SilenceMutedAlert: &eventrecorderpb.SilenceMutedAlertEvent{
				Silence: silence,
				MutedAlert: &eventrecorderpb.MutedAlert{
					Fingerprint: uint64(fp),
					Labels:      LabelSetAsProto(lset),
				},
			},
		},
	}
}

// NewSilenceCreatedEvent constructs a SilenceCreated event.
func NewSilenceCreatedEvent(silence *eventrecorderpb.Silence) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceCreated{
			SilenceCreated: &eventrecorderpb.SilenceCreatedEvent{
				Silence: silence,
			},
		},
	}
}

// NewSilenceUpdatedEvent constructs a SilenceUpdated event.
func NewSilenceUpdatedEvent(silence *eventrecorderpb.Silence) *eventrecorderpb.EventData {
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_SilenceUpdated{
			SilenceUpdated: &eventrecorderpb.SilenceUpdatedEvent{
				Silence: silence,
			},
		},
	}
}

// NewInhibitionMutedAlertEvent constructs an InhibitionMutedAlert event.
func NewInhibitionMutedAlertEvent(rules []*eventrecorderpb.InhibitRule, fp model.Fingerprint, lset model.LabelSet, inhibitingFPs []model.Fingerprint) *eventrecorderpb.EventData {
	fps := make([]uint64, len(inhibitingFPs))
	for i, f := range inhibitingFPs {
		fps[i] = uint64(f)
	}
	return &eventrecorderpb.EventData{
		EventType: &eventrecorderpb.EventData_InhibitionMutedAlert{
			InhibitionMutedAlert: &eventrecorderpb.InhibitionMutedAlertEvent{
				InhibitRules: rules,
				MutedAlert: &eventrecorderpb.MutedAlert{
					Fingerprint: uint64(fp),
					Labels:      LabelSetAsProto(lset),
				},
				InhibitingFingerprints: fps,
			},
		},
	}
}
